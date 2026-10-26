package adapters

// Workspace task billing bridge (#821, Phase 4-04 of #809).
//
// This quoter adapts the billing catalog to workspacetasks.Quoter. SKU is
// server-derived (ai-image:<canonical service>); no client price input
// crosses this seam. Quoter failures keep the ErrQuoteUnavailable routing
// identity while preserving the underlying catalog cause (multi-%w):
// pricing failures (ErrPricingUnavailable/ErrSKUNotFound) render 422
// billing_pricing_unavailable via the first branch of renderTaskError,
// transient failures keep 503 billing_quote_unavailable.
//
// QuoteInTx mints the same snapshot through the settlement transaction so
// the price read shares the repeatable-read snapshot with the Hold that
// follows (workspacetasks.TxQuoter).

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

// WorkspaceTaskQuoter adapts the billing catalog to workspacetasks.Quoter.
type WorkspaceTaskQuoter struct {
	Catalog *billingapplication.BillableItemCatalog
}

var _ workspacetasks.Quoter = (*WorkspaceTaskQuoter)(nil)
var _ workspacetasks.TxQuoter = (*WorkspaceTaskQuoter)(nil)

func (q *WorkspaceTaskQuoter) Quote(ctx context.Context, service, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.Catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(service))
	if !ok {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: unsupported service %q: %w", service, workspacetasks.ErrQuoteUnavailable)
	}
	if quantity <= 0 {
		quantity = 1
	}
	outputResolution, err := billing.NormalizeOutputResolution(resolution)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: invalid output resolution: %w", err)
	}
	sku := "ai-image:" + canonical
	quote, err := q.Catalog.Quote(ctx, nil, sku, outputResolution, quantity)
	if err != nil {
		// Preserve the underlying billing cause alongside the
		// quote-unavailable identity (multi-%w): pricing failures
		// (ErrPricingUnavailable/ErrSKUNotFound) render 422 via the
		// first branch of renderTaskError, while transient failures
		// keep the 503 billing_quote_unavailable branch.
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w: %w", err, workspacetasks.ErrQuoteUnavailable)
	}
	return workspacetasks.BillingSnapshot{
		SKU:              string(quote.SKU),
		DisplayName:      quote.DisplayName,
		PriceVersionID:   quote.PriceVersionID,
		PriceVersion:     quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits,
		Quantity:         quote.Quantity,
		TotalCredits:     quote.TotalCredits,
		QuotedAt:         quote.QuotedAt,
		SettlementState:  "quoted",
	}, nil
}

// QuoteInTx mints the same snapshot through the settlement transaction so
// the price read shares the repeatable-read snapshot with the Hold that
// follows. It satisfies workspacetasks.TxQuoter.
func (q *WorkspaceTaskQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if q == nil || q.Catalog == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter is not configured: %w", workspacetasks.ErrQuoteUnavailable)
	}
	if tx == nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: settlement transaction is nil: %w", workspacetasks.ErrQuoteUnavailable)
	}
	canonical, ok := workflow.ParseAIImageFeatureSlug(strings.TrimSpace(service))
	if !ok {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: unsupported service %q: %w", service, workspacetasks.ErrQuoteUnavailable)
	}
	if quantity <= 0 {
		quantity = 1
	}
	outputResolution, err := billing.NormalizeOutputResolution(resolution)
	if err != nil {
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: invalid output resolution: %w", err)
	}
	sku := "ai-image:" + canonical
	quote, err := q.Catalog.Quote(ctx, tx, sku, outputResolution, quantity)
	if err != nil {
		// Same multi-%w preservation as Quote: the price read shares
		// the repeatable-read snapshot with the Hold that follows, and
		// the pricing cause must survive to renderTaskError.
		return workspacetasks.BillingSnapshot{}, fmt.Errorf("workspace task quoter: %w: %w", err, workspacetasks.ErrQuoteUnavailable)
	}
	return workspacetasks.BillingSnapshot{
		SKU:              string(quote.SKU),
		DisplayName:      quote.DisplayName,
		PriceVersionID:   quote.PriceVersionID,
		PriceVersion:     quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits,
		Quantity:         quote.Quantity,
		TotalCredits:     quote.TotalCredits,
		QuotedAt:         quote.QuotedAt,
		SettlementState:  "quoted",
	}, nil
}
