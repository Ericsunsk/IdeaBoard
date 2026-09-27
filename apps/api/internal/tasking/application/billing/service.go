package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	taskbilling "ideaboard/api/internal/tasking/ports/billing"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrUnsupportedProduct reports a feature slug with no billable SKU
	// mapping. It stays local: unlike billing.ErrSKUNotFound (a catalog
	// lookup miss for a known product), it fires before any SKU exists,
	// when the feature itself is not a billable task product.
	ErrUnsupportedProduct = errors.New("task product is not billable")
	// Canonical pricing/credit aliases (T4 convergence): these sentinels
	// are the shared billing/credits identities, so handler errors.Is
	// branches keep matching with zero handler edits.
	ErrPricingUnavailable  = billing.ErrPricingUnavailable
	ErrInsufficientCredits = domain.ErrInsufficientCredits
	ErrWalletRestricted    = domain.ErrWalletRestricted
)

type Quoter interface {
	QuoteAtResolution(context.Context, string, billing.OutputResolution, int64) (billing.Quote, error)
	QuoteAtResolutionInTx(context.Context, pgx.Tx, string, billing.OutputResolution, int64) (billing.Quote, error)
}

type CreditService interface {
	ports.WalletReader
	InTx(pgx.Tx) ports.Commands
}

type Service struct {
	quoter  Quoter
	credits CreditService
}

type Request struct {
	Feature          string
	OutputResolution billing.OutputResolution
	Quantity         int64
}

// HoldCommand bundles one task wallet hold with the validated billing request
// it settles. It replaces the unnamed (taskID, userID, request) parameter
// group previously threaded across the submission seam.
type HoldCommand struct {
	TaskID  string
	UserID  int64
	Billing Request
}

type Readiness struct {
	billing.Quote
	AvailableCredits int64 `json:"available_credits"`
	ReservedCredits  int64 `json:"reserved_credits"`
	Sufficient       bool  `json:"sufficient"`
}

type Snapshot struct {
	TaskID            string
	UserID            int64
	Feature           string
	SKU               billing.SKU
	DisplayName       string
	PriceVersionID    int64
	PriceVersion      int64
	OutputResolution  billing.OutputResolution
	UnitCredits       int64
	Quantity          int64
	TotalCredits      int64
	QuotedAt          time.Time
	HoldTransactionID string
}

func NewService(quoter Quoter, creditService CreditService) *Service {
	return &Service{quoter: quoter, credits: creditService}
}

func AIImageRequest(feature, rawResolution string, quantity int64) (Request, error) {
	canonicalFeature, ok := workflow.ParseAIImageFeatureSlug(feature)
	if !ok {
		return Request{}, ErrUnsupportedProduct
	}
	if quantity <= 0 {
		return Request{}, billing.ErrInvalidQuantity
	}
	resolution, err := billing.NormalizeOutputResolution(rawResolution)
	if err != nil {
		return Request{}, err
	}
	return Request{Feature: canonicalFeature, OutputResolution: resolution, Quantity: quantity}, nil
}

func ResolveSKU(request Request) (billing.SKU, error) {
	feature, ok := workflow.ParseAIImageFeatureSlug(request.Feature)
	if !ok {
		return "", ErrUnsupportedProduct
	}
	return billing.SKU("ai-image:" + feature), nil
}

func RequestFingerprint(request Request) (string, error) {
	sku, err := validateRequest(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(string(sku) + "\x00" + string(request.OutputResolution) + "\x00" + strconv.FormatInt(request.Quantity, 10)))
	return hex.EncodeToString(digest[:]), nil
}

func validateRequest(request Request) (billing.SKU, error) {
	sku, err := ResolveSKU(request)
	if err != nil {
		return "", err
	}
	if request.Quantity <= 0 {
		return "", billing.ErrInvalidQuantity
	}
	if _, err := billing.NormalizeOutputResolution(string(request.OutputResolution)); err != nil {
		return "", err
	}
	return sku, nil
}

func (s *Service) Readiness(ctx context.Context, userID int64, request Request) (Readiness, error) {
	if s == nil || s.quoter == nil || s.credits == nil || userID <= 0 {
		return Readiness{}, fmt.Errorf("task billing service unavailable")
	}
	sku, err := validateRequest(request)
	if err != nil {
		return Readiness{}, err
	}
	quote, err := s.quoter.QuoteAtResolution(ctx, string(sku), request.OutputResolution, request.Quantity)
	if err != nil {
		return Readiness{}, fmt.Errorf("%w: %w", ErrPricingUnavailable, err)
	}
	wallet, err := s.credits.GetWallet(ctx, userID)
	if err != nil {
		return Readiness{}, err
	}
	if len(wallet.ActiveRestrictions) > 0 {
		return Readiness{}, ErrWalletRestricted
	}
	return Readiness{
		Quote: quote, AvailableCredits: wallet.Available, ReservedCredits: wallet.Reserved,
		Sufficient: wallet.Available >= quote.TotalCredits,
	}, nil
}

func (s *Service) HoldAndPersist(ctx context.Context, tx pgx.Tx, command HoldCommand) (Snapshot, error) {
	taskID, userID, request := command.TaskID, command.UserID, command.Billing
	if s == nil || s.quoter == nil || s.credits == nil || tx == nil || strings.TrimSpace(taskID) == "" || userID <= 0 {
		return Snapshot{}, fmt.Errorf("task billing service unavailable")
	}
	sku, err := validateRequest(request)
	if err != nil {
		return Snapshot{}, err
	}
	quote, err := s.quoter.QuoteAtResolutionInTx(ctx, tx, string(sku), request.OutputResolution, request.Quantity)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %w", ErrPricingUnavailable, err)
	}

	snapshot := Snapshot{
		TaskID: taskID, UserID: userID, Feature: request.Feature,
		SKU: quote.SKU, DisplayName: quote.DisplayName, PriceVersionID: quote.PriceVersionID, PriceVersion: quote.PriceVersion,
		OutputResolution: quote.OutputResolution,
		UnitCredits:      quote.UnitCredits, Quantity: quote.Quantity, TotalCredits: quote.TotalCredits, QuotedAt: quote.QuotedAt,
	}
	state := taskbilling.SettlementStateFree
	settledAt := any(quote.QuotedAt)
	var holdTransactionID any
	if quote.TotalCredits > 0 {
		amount, amountErr := domain.NewCreditAmount(quote.TotalCredits)
		if amountErr != nil {
			return Snapshot{}, amountErr
		}
		hold, holdErr := s.credits.InTx(tx).Hold(ctx, ports.HoldCommand{
			UserID:   userID,
			Amount:   amount,
			Metadata: taskCommandMetadata("task-hold:"+taskID, domain.ActorUser, fmt.Sprint(userID), "task_submission", "AI 生图任务 Idea 预授权", taskID),
		})
		if errors.Is(holdErr, domain.ErrInsufficientCredits) {
			return Snapshot{}, ErrInsufficientCredits
		}
		if errors.Is(holdErr, domain.ErrWalletRestricted) {
			return Snapshot{}, ErrWalletRestricted
		}
		if holdErr != nil {
			return Snapshot{}, holdErr
		}
		snapshot.HoldTransactionID = hold.TransactionID
		holdTransactionID = hold.TransactionID
		state = taskbilling.SettlementStateHeld
		settledAt = nil
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, display_name, price_version_id, price_version,
  output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		snapshot.TaskID, snapshot.UserID, snapshot.Feature, snapshot.SKU,
		snapshot.DisplayName, snapshot.PriceVersionID, snapshot.PriceVersion, snapshot.OutputResolution, snapshot.UnitCredits, snapshot.Quantity,
		snapshot.TotalCredits, snapshot.QuotedAt, holdTransactionID,
	); err != nil {
		return Snapshot{}, fmt.Errorf("persist task billing snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO task_billing_settlements (task_id, state, settled_at)
VALUES ($1, $2, $3)`, snapshot.TaskID, state, settledAt); err != nil {
		return Snapshot{}, fmt.Errorf("persist task billing settlement: %w", err)
	}
	return snapshot, nil
}

func taskCommandMetadata(key string, actorType domain.ActorType, actorID, source, reason, taskID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             domain.Actor{Type: actorType, ID: actorID},
		Source:            source,
		Reason:            reason,
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}
