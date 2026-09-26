package runtime

// Workspace library production billing adapters (#828 slice B).
//
// The v1 library surface (contract_v1_library.go) settles materialization
// jobs through two narrow seams: V1BillingQuoter (server-side price for
// the library:materialize SKU; no client price input crosses it) and
// V1CreditSettler (Hold/Capture/Release with caller idempotency keys).
// This file adapts the production billing catalog and credits Commands
// to those seams so http_wiring.go can take the materialization billing
// path live (replacing the nil-seam 503 fail-closed).
//
// Pattern (follows the #804 workspace task adapters in
// workspace_task_adapters.go and the vision reserve/capture/release
// shape in searchcatalog/application/vision_billing.go):
//   - Quote goes through BillableItemCatalog.QuoteAtResolution with the
//     default variant, quantity 1. Any error stays an error: the handler
//     already maps every quoter failure to billing_quote_unavailable,
//     and unpriced/unregistered SKUs fail closed as pricing-unavailable
//     inside the catalog (registry gate).
//   - Hold/Capture/Release go through credits.Service (the sole ledger
//     writer); this seam performs no ledger SQL of its own. Metadata
//     carries the caller idempotency key, the user actor, the
//     library_materialize source, and a BusinessReference keyed by that
//     same idempotency key, so the ledger holds the
//     task_id -> materialization traceability link without schema
//     changes (the trigger crafts keys embedding both ids).
//   - Wallet rejections (insufficient/restricted) map to the exported
//     httpapi.ErrV1BillingRestricted identity so the handler renders the
//     #804 403 billing_wallet_restricted; everything else surfaces raw
//     for the 503 branch.
//   - Zero/negative amounts are free markers, not ledger commands:
//     Hold returns ("", nil) and Capture/Release are no-ops, so a free
//     SKU completes its job with no ledger writes (no orphans by
//     construction). This mirrors the workspaceTaskHolder convention
//     (amount<=0 converges without touching Credits).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
)

// libraryMaterializeQuoter adapts the billing catalog to the v1 library
// quoter seam. SKU is server-derived (library:materialize); the
// workspace id scopes the route, not the price.
type libraryMaterializeQuoter struct {
	catalog *billingapplication.BillableItemCatalog
}

var _ httpapi.V1BillingQuoter = (*libraryMaterializeQuoter)(nil)

func NewLibraryMaterializeQuoter(catalog *billingapplication.BillableItemCatalog) *libraryMaterializeQuoter {
	return &libraryMaterializeQuoter{catalog: catalog}
}

// Quote implements httpapi.V1BillingQuoter.
func (q *libraryMaterializeQuoter) Quote(ctx context.Context, workspacePublicID, sku string) (int64, error) {
	if q == nil || q.catalog == nil {
		return 0, fmt.Errorf("library materialize quoter is not configured: %w", httpapi.ErrV1BillingQuoteUnavailable)
	}
	trimmed := strings.TrimSpace(sku)
	if trimmed == "" {
		return 0, fmt.Errorf("library materialize quoter: SKU required: %w", httpapi.ErrV1BillingQuoteUnavailable)
	}
	quote, err := q.catalog.QuoteAtResolution(ctx, trimmed, billing.OutputResolutionDefault, 1)
	if err != nil {
		return 0, fmt.Errorf("library materialize quoter: %w", err)
	}
	_ = workspacePublicID
	return quote.TotalCredits, nil
}

// libraryMaterializeSettler adapts credits.Commands to the v1 library
// settler seam. Only Credits owns ledger writes.
type libraryMaterializeSettler struct {
	credits *credits.Service
}

var _ httpapi.V1CreditSettler = (*libraryMaterializeSettler)(nil)

func NewLibraryMaterializeSettler(creditService *credits.Service) *libraryMaterializeSettler {
	return &libraryMaterializeSettler{credits: creditService}
}

func librarySettleMetadata(idempotencyKey string, userID int64, reason string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		Actor:          domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:         "library_materialize",
		Reason:         reason,
		BusinessReference: domain.BusinessReference{
			Type: "library_materialization",
			ID:   strings.TrimSpace(idempotencyKey),
		},
	}
}

// mapLibrarySettleError converts wallet rejections to the #804 restricted
// identity; every other failure keeps its credits shape for the 503 path.
func mapLibrarySettleError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInsufficientCredits) || errors.Is(err, domain.ErrWalletRestricted) {
		return fmt.Errorf("library wallet restricted (%v): %w", err, httpapi.ErrV1BillingRestricted)
	}
	return err
}

// Hold implements httpapi.V1CreditSettler. A non-positive amount is a
// free marker: no ledger command, empty hold id, nil error.
func (s *libraryMaterializeSettler) Hold(ctx context.Context, userID int64, amountCredits int64, idempotencyKey, reason string) (string, error) {
	if amountCredits <= 0 {
		return "", nil
	}
	if userID <= 0 {
		return "", fmt.Errorf("library materialize settler: user required: %w", httpapi.ErrV1BillingRestricted)
	}
	if s == nil || s.credits == nil {
		return "", fmt.Errorf("library materialize settler is not configured: %w", httpapi.ErrV1BillingQuoteUnavailable)
	}
	amount, err := domain.NewCreditAmount(amountCredits)
	if err != nil {
		return "", fmt.Errorf("library materialize settler: %w", err)
	}
	result, err := s.credits.Hold(ctx, ports.HoldCommand{
		UserID: userID, Amount: amount,
		Metadata: librarySettleMetadata(idempotencyKey, userID, reason),
	})
	if err != nil {
		return "", mapLibrarySettleError(err)
	}
	return result.TransactionID, nil
}

// Capture implements httpapi.V1CreditSettler. Empty hold ids and
// non-positive amounts (free jobs) converge without ledger writes.
func (s *libraryMaterializeSettler) Capture(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	if strings.TrimSpace(holdID) == "" || amountCredits <= 0 {
		return nil
	}
	if userID <= 0 {
		return fmt.Errorf("library materialize settler: user required: %w", httpapi.ErrV1BillingRestricted)
	}
	if s == nil || s.credits == nil {
		return fmt.Errorf("library materialize settler is not configured: %w", httpapi.ErrV1BillingQuoteUnavailable)
	}
	if userID <= 0 {
		return fmt.Errorf("library materialize settler: user required: %w", httpapi.ErrV1BillingRestricted)
	}
	amount, err := domain.NewCreditAmount(amountCredits)
	if err != nil {
		return fmt.Errorf("library materialize settler: %w", err)
	}
	_, err = s.credits.Capture(ctx, ports.CaptureCommand{
		UserID: userID, HoldTransactionID: strings.TrimSpace(holdID), Amount: amount,
		Metadata: librarySettleMetadata(idempotencyKey, userID, "library materialize capture"),
	})
	if err != nil {
		return mapLibrarySettleError(err)
	}
	return nil
}

// Release implements httpapi.V1CreditSettler. Empty hold ids and
// non-positive amounts (free jobs, or holds that never happened)
// converge without ledger writes, so compensation never orphans.
func (s *libraryMaterializeSettler) Release(ctx context.Context, userID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	if strings.TrimSpace(holdID) == "" || amountCredits <= 0 {
		return nil
	}
	if userID <= 0 {
		return fmt.Errorf("library materialize settler: user required: %w", httpapi.ErrV1BillingRestricted)
	}
	if s == nil || s.credits == nil {
		return fmt.Errorf("library materialize settler is not configured: %w", httpapi.ErrV1BillingQuoteUnavailable)
	}
	if userID <= 0 {
		return fmt.Errorf("library materialize settler: user required: %w", httpapi.ErrV1BillingRestricted)
	}
	amount, err := domain.NewCreditAmount(amountCredits)
	if err != nil {
		return fmt.Errorf("library materialize settler: %w", err)
	}
	_, err = s.credits.Release(ctx, ports.ReleaseCommand{
		UserID: userID, HoldTransactionID: strings.TrimSpace(holdID), Amount: amount,
		Metadata: librarySettleMetadata(idempotencyKey, userID, "library materialize release"),
	})
	if err != nil {
		return mapLibrarySettleError(err)
	}
	return nil
}
