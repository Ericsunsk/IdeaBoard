package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/idempotency"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	visionSearchSKU    = "ai-search:vision"
	visionPendingAfter = 15 * time.Minute
)

// visionBillingKeyMaxLength follows the platform idempotency bound so vision
// and tasking reject the same oversized keys with idempotency_key_required.
const visionBillingKeyMaxLength = idempotency.MaxKeyLength

var (
	ErrVisionIdempotencyKeyRequired = errors.New("vision search idempotency key is required")
	ErrVisionIdempotencyConflict    = errors.New("vision search idempotency key conflicts with a different request")
	ErrVisionPricingUnavailable     = errors.New("vision search pricing is unavailable")
	ErrVisionSettlement             = errors.New("vision search credit settlement is uncertain")
	// ErrVisionWorkspaceNotFound reports a hidden workspace or a caller
	// without membership. Handlers render it as workspace_not_found (never
	// a billing status), per anti-enumeration: outsiders learn nothing
	// about the payer.
	ErrVisionWorkspaceNotFound = errors.New("vision search workspace is not visible")
)

type VisionBillingResult struct {
	searchdomain.VisionExtraction
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed,omitempty"`
}

// VisionOperationStatus is the closed vision billing operation vocabulary
// carried by VisionBillingResult.Status and the ai_search_operations
// status column. The wire/DB words are unchanged
// ("pending","succeeded","failed","reconciliation_pending"); the type
// only documents the allowed words at compile time. The DTO field stays
// string with conversion at each construction site, and DB-scanned words
// convert at each comparison site.
type VisionOperationStatus string

const (
	VisionOperationPending               VisionOperationStatus = "pending"
	VisionOperationSucceeded             VisionOperationStatus = "succeeded"
	VisionOperationFailed                VisionOperationStatus = "failed"
	VisionOperationReconciliationPending VisionOperationStatus = "reconciliation_pending"
)

// visionFailureCodes are the closed settlement-failure code words flowing
// through VisionOperationFailure.Code into httpStatusForVisionCode. The
// wire words are unchanged; the constants only kill the raw literals.
const (
	visionFailureCodeSKUNotFound        = "sku_not_found"
	visionFailureCodePricingUnavailable = "pricing_unavailable"
	visionFailureCodeWalletRestricted   = "wallet_restricted"
	visionFailureCodeUnpricedZero       = "unpriced_zero"
)

// visionBillingDeferredReason preserves the catalog miss distinction (do
// NOT merge the sentinels): pricing_unavailable vs sku_not_found stay
// distinct reasons in the deferred metric.
func visionBillingDeferredReason(err error) (string, bool) {
	if errors.Is(err, billing.ErrSKUNotFound) {
		return visionFailureCodeSKUNotFound, true
	}
	if errors.Is(err, billing.ErrPricingUnavailable) {
		return visionFailureCodePricingUnavailable, true
	}
	return "", false
}

// visionBillingDeferredObserve emits the billing_deferred metric + backfill
// note via the existing slog observability pattern (closed labels only:
// sku, reason; no user IDs in metric fields). It is a package-level hook
// so tests can stub metric emission without scraping Prometheus;
// production uses slog.
var visionBillingDeferredObserve = func(sku, reason, operationID string) {
	slog.Warn("vision search billing deferred: catalog lacks price; proceeding free with backfill note",
		"sku", sku, "reason", reason, "operation_id", operationID,
		"backfill", "ai_search_operations row omitted; needs backfill for unpriced vision SKU")
}

type VisionOperationFailure struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *VisionOperationFailure) Error() string { return e.Message }

type VisionBillingPending struct {
	OperationID string
}

func (e *VisionBillingPending) Error() string {
	return "vision search operation is pending reconciliation"
}

type VisionBillingService struct {
	pool     *pgxpool.Pool
	catalog  *billingapplication.BillableItemCatalog
	credits  *credits.Service
	vision   *VisionSearchService
	accounts VisionBillingAccountResolver
}

func NewVisionBillingService(pool *pgxpool.Pool, catalog *billingapplication.BillableItemCatalog, creditService *credits.Service, vision *VisionSearchService) *VisionBillingService {
	return &VisionBillingService{pool: pool, catalog: catalog, credits: creditService, vision: vision, accounts: visionBillingAccountResolver{}}
}

// NewVisionBillingServiceWithResolver is the seam tests use to inject a
// fake settlement identity; production uses NewVisionBillingService.
func NewVisionBillingServiceWithResolver(pool *pgxpool.Pool, catalog *billingapplication.BillableItemCatalog, creditService *credits.Service, vision *VisionSearchService, resolver VisionBillingAccountResolver) *VisionBillingService {
	return &VisionBillingService{pool: pool, catalog: catalog, credits: creditService, vision: vision, accounts: resolver}
}

// Extract bills one vision call to the consuming Workspace's stable
// Billing Account (#918): the submitter stays the real initiator (Actor
// on every Command), the Workspace is the consumption location, and the
// resolved account is the only payer. There is no personal-wallet
// fallback on this path.
func (s *VisionBillingService) Extract(ctx context.Context, workspacePublicID string, userID int64, idempotencyKey string, input searchdomain.VisionInput) (VisionBillingResult, error) {
	if s == nil || s.pool == nil || s.catalog == nil || s.credits == nil || s.vision == nil {
		return VisionBillingResult{}, searchdomain.ErrVisionUnavailable
	}
	if userID <= 0 {
		return VisionBillingResult{}, searchdomain.ErrVisionInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" {
		return VisionBillingResult{}, ErrVisionWorkspaceNotFound
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > visionBillingKeyMaxLength {
		return VisionBillingResult{}, ErrVisionIdempotencyKeyRequired
	}
	if err := validateVisionInput(input); err != nil {
		return VisionBillingResult{}, err
	}
	// The request hash binds the resource scope (workspace + submitter +
	// normalized input bytes) so a key reused across workspaces, operators,
	// or bodies conflicts instead of replaying another scope's operation.
	requestHash := hashVisionRequest(workspacePublicID, userID, input)
	reserved, err := s.reserve(ctx, workspacePublicID, userID, idempotencyKey, requestHash)
	if err != nil {
		return VisionBillingResult{}, err
	}
	if reserved.result.Status != string(VisionOperationPending) {
		return reserved.result, reserved.failure
	}
	if !reserved.execute {
		return reserved.result, nil
	}

	extraction, providerErr := s.vision.Extract(ctx, input)
	if providerErr != nil {
		if reserved.deferred {
			// Deferred free path has no ledger row: skip release/
			// reconciliation and fail directly (no settlement uncertainty).
			code, message, status := visionFailureDetails(providerErr)
			return VisionBillingResult{}, &VisionOperationFailure{Code: code, Message: message, HTTPStatus: status}
		}
		if isVisionUncertain(providerErr) {
			_ = s.markReconciliationPending(context.WithoutCancel(ctx), reserved.result.OperationID)
			return VisionBillingResult{OperationID: reserved.result.OperationID, Status: string(VisionOperationReconciliationPending)}, &VisionBillingPending{OperationID: reserved.result.OperationID}
		}
		code, message, status := visionFailureDetails(providerErr)
		if err := s.release(context.WithoutCancel(ctx), reserved.result.OperationID, workspacePublicID, userID, code, message); err != nil {
			_ = s.markReconciliationPending(context.WithoutCancel(ctx), reserved.result.OperationID)
			return VisionBillingResult{OperationID: reserved.result.OperationID, Status: string(VisionOperationReconciliationPending)}, fmt.Errorf("%w: release: %w", ErrVisionSettlement, err)
		}
		return VisionBillingResult{}, &VisionOperationFailure{Code: code, Message: message, HTTPStatus: status}
	}

	if reserved.deferred {
		// Deferred free path skips ledger capture: amount 0 free-marker.
		return VisionBillingResult{VisionExtraction: extraction, OperationID: reserved.result.OperationID, Status: string(VisionOperationSucceeded)}, nil
	}
	if err := s.capture(context.WithoutCancel(ctx), reserved.result.OperationID, workspacePublicID, userID, extraction.Query); err != nil {
		_ = s.markReconciliationPending(context.WithoutCancel(ctx), reserved.result.OperationID)
		return VisionBillingResult{OperationID: reserved.result.OperationID, Status: string(VisionOperationReconciliationPending)}, fmt.Errorf("%w: capture: %w", ErrVisionSettlement, err)
	}
	return VisionBillingResult{VisionExtraction: extraction, OperationID: reserved.result.OperationID, Status: string(VisionOperationSucceeded)}, nil
}

type visionReservation struct {
	result   VisionBillingResult
	failure  error
	execute  bool
	deferred bool
}

func validateVisionInput(input searchdomain.VisionInput) error {
	if len(input.Images) < 1 || len(input.Images) > searchdomain.VisionMaxImages {
		return searchdomain.ErrVisionInput
	}
	if err := ValidateVisionImages(input.Images); err != nil {
		return err
	}
	if utf8RuneCount(strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")) > MaxSearchQueryRunes {
		return fmt.Errorf(errorrender.MsgNormalizeQuery, MaxSearchQueryRunes)
	}
	return nil
}

func hashVisionInput(input searchdomain.VisionInput) string {
	hash := sha256.New()
	hash.Write([]byte(strings.Join(strings.Fields(strings.TrimSpace(input.UserText)), " ")))
	for _, image := range input.Images {
		hash.Write([]byte{0})
		hash.Write([]byte(image.Filename))
		hash.Write([]byte{0})
		hash.Write([]byte(image.ContentType))
		hash.Write([]byte{0})
		hash.Write(image.Data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// hashVisionRequest binds the idempotency hash to the resource scope
// (consuming workspace + submitter) in addition to the normalized input
// bytes. The same client key reused across workspaces, operators, or
// bodies therefore hashes differently and conflicts instead of replaying
// another scope's operation.
func hashVisionRequest(workspacePublicID string, userID int64, input searchdomain.VisionInput) string {
	hash := sha256.New()
	hash.Write([]byte(strings.TrimSpace(workspacePublicID)))
	hash.Write([]byte{0})
	fmt.Fprintf(hash, "%d", userID)
	hash.Write([]byte{0})
	hash.Write([]byte(hashVisionInput(input)))
	return hex.EncodeToString(hash.Sum(nil))
}

func isVisionUncertain(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var providerErr *searchdomain.VisionProviderError
	return errors.As(err, &providerErr) && (providerErr.Kind == searchdomain.VisionProviderFailureNetwork || providerErr.Kind == searchdomain.VisionProviderFailureTimeout)
}

func visionFailureDetails(err error) (string, string, int) {
	var providerErr *searchdomain.VisionProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case searchdomain.VisionProviderFailureCredentials:
			return "vision_search_credentials", errorrender.MsgVisionFailureDetails, 502
		case searchdomain.VisionProviderFailureQuota:
			return "vision_search_quota", errorrender.MsgVisionFailureDetails2, 502
		case searchdomain.VisionProviderFailureResponse:
			return "vision_search_schema_error", errorrender.MsgVisionFailureDetails3, 502
		}
	}
	switch {
	case errors.Is(err, searchdomain.ErrVisionSchema):
		return "vision_search_schema_error", errorrender.MsgVisionFailureDetails3, 502
	case errors.Is(err, searchdomain.ErrVisionNotConfigured):
		return "vision_search_not_configured", errorrender.MsgVisionFailureDetails4, 503
	case errors.Is(err, searchdomain.ErrVisionInput), errors.Is(err, searchdomain.ErrVisionImageInvalid), errors.Is(err, searchdomain.ErrVisionImageUnavailable):
		return "vision_search_input_invalid", errorrender.MsgVisionFailureDetails5, 400
	default:
		return "vision_search_provider_error", errorrender.MsgVisionFailureDetails6, 502
	}
}

func httpStatusForVisionCode(code string) int {
	switch code {
	case visionFailureCodePricingUnavailable:
		return 503
	case visionFailureCodeWalletRestricted:
		return 409
	case string(idempotency.CodeConflict):
		return 409
	case string(idempotency.CodeRequired):
		return 400
	}
	if strings.HasSuffix(code, "not_configured") {
		return 503
	}
	if strings.HasSuffix(code, "input_invalid") {
		return 400
	}
	return 502
}
