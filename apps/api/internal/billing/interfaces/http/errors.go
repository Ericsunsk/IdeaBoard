package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// Billing error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display and never carry SKU plaintext
// beyond the already-constrained canonical form, quantities, or provider
// payloads.
//
// Classification lives here at the HTTP edge so billing domain sentinels
// stay dependency-free; every billing failure below renders through
// response.MachineError, never through the numeric response.Error envelope.
// 5xx outcomes are logged server-side (InternalError semantics) so they stay
// attributable; 4xx outcomes render without logging.
const (
	codeBillingInvalidInput       = "billing_invalid_input"
	codeBillingSKUNotFound        = "billing_sku_not_found"
	codeBillingPricingUnavailable = "billing_pricing_unavailable"
	codeBillingItemNotArchived    = "billing_item_not_archived"
	codeBillingArchivedItemInUse  = "billing_archived_item_in_use"
	codeBillingQuoteTotalExceeded = "billing_quote_total_exceeded"
	codeBillingServiceUnavailable = "billing_service_unavailable"
	codeBillingOperationFailed    = "billing_operation_failed"
)

type billingErrorMapping struct {
	targets []error
	status  int
	code    string
	message string
}

func (m billingErrorMapping) Matches(err error) bool {
	return errorrender.TargetsMatch(m.targets, err)
}

// billingDomainErrorTable is the single source of truth for mapping billing
// failures to the machine-readable envelope. Rows are ordered from the most
// specific outcome to the most generic; the first match wins.
var billingDomainErrorTable = []billingErrorMapping{
	{
		targets: []error{billing.ErrInvalidSKU, billing.ErrInvalidQuantity, billing.ErrInvalidCatalogEntry, billing.ErrInvalidOutputResolution},
		status:  http.StatusBadRequest,
		code:    codeBillingInvalidInput,
		message: "参数错误",
	},
	{
		targets: []error{billing.ErrSKUNotFound},
		status:  http.StatusNotFound,
		code:    codeBillingSKUNotFound,
		message: "计费功能不存在",
	},
	{
		targets: []error{billing.ErrPricingUnavailable},
		status:  http.StatusUnprocessableEntity,
		code:    codeBillingPricingUnavailable,
		message: "计费功能当前不可计价",
	},
	{
		targets: []error{billing.ErrItemNotArchived},
		status:  http.StatusConflict,
		code:    codeBillingItemNotArchived,
		message: "仅已归档功能可以删除",
	},
	{
		targets: []error{billing.ErrArchivedItemInUse},
		status:  http.StatusConflict,
		code:    codeBillingArchivedItemInUse,
		message: "功能已有历史计费记录，无法删除",
	},
	{
		targets: []error{billing.ErrQuoteTotalExceeded},
		status:  http.StatusUnprocessableEntity,
		code:    codeBillingQuoteTotalExceeded,
		message: "报价超过允许上限",
	},
	{
		targets: []error{billing.ErrServiceUnavailable},
		status:  http.StatusServiceUnavailable,
		code:    codeBillingServiceUnavailable,
		message: "计费服务暂不可用",
	},
}

func writeError(c *gin.Context, err error) {
	if row, ok := errorrender.FirstMatch(billingDomainErrorTable, err); ok {
		if row.status >= 500 {
			slog.Error("billing operation failed",
				"status", row.status,
				"code", row.code,
				"error_type", errorTypeOf(err),
				"err", err)
		}
		errorrender.Render(c, row.status, row.code, row.message)
		return
	}
	errorrender.RenderDefault(c, "billing operation failed",
		http.StatusInternalServerError, codeBillingOperationFailed, "计费操作失败", err,
		"error_type", errorTypeOf(err))
}

// writeBillingInputError renders request-shape failures (malformed JSON,
// unknown fields, missing required fields) through the machine envelope so
// callers branch on a stable code instead of copy.
func writeBillingInputError(c *gin.Context) {
	errorrender.Render(c, http.StatusBadRequest, codeBillingInvalidInput, "参数错误")
}

func errorTypeOf(err error) string {
	return fmt.Sprintf("%T", err)
}
