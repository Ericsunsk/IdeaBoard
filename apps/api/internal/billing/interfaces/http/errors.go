package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/platform/contract"
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
// Billing error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display and never carry SKU plaintext
// beyond the already-constrained canonical form, quantities, or provider
// payloads.
//
// The values below alias the kernel registry
// (platform/contract.CodeBilling*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeBillingInvalidInput       = string(contract.CodeBillingInvalidInput)
	codeBillingSKUNotFound        = string(contract.CodeBillingSKUNotFound)
	codeBillingPricingUnavailable = string(contract.CodeBillingPricingUnavailable)
	codeBillingItemNotArchived    = string(contract.CodeBillingItemNotArchived)
	codeBillingArchivedItemInUse  = string(contract.CodeBillingArchivedItemInUse)
	codeBillingQuoteTotalExceeded = string(contract.CodeBillingQuoteTotalExceeded)
	codeBillingServiceUnavailable = string(contract.CodeBillingServiceUnavailable)
	codeBillingOperationFailed    = string(contract.CodeBillingOperationFailed)
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
		message: msgMatches,
	},
	{
		targets: []error{billing.ErrSKUNotFound},
		status:  http.StatusNotFound,
		code:    codeBillingSKUNotFound,
		message: msgMatches2,
	},
	{
		targets: []error{billing.ErrPricingUnavailable},
		status:  http.StatusUnprocessableEntity,
		code:    codeBillingPricingUnavailable,
		message: msgMatches3,
	},
	{
		targets: []error{billing.ErrItemNotArchived},
		status:  http.StatusConflict,
		code:    codeBillingItemNotArchived,
		message: msgMatches4,
	},
	{
		targets: []error{billing.ErrArchivedItemInUse},
		status:  http.StatusConflict,
		code:    codeBillingArchivedItemInUse,
		message: msgMatches5,
	},
	{
		targets: []error{billing.ErrQuoteTotalExceeded},
		status:  http.StatusUnprocessableEntity,
		code:    codeBillingQuoteTotalExceeded,
		message: msgMatches6,
	},
	{
		targets: []error{billing.ErrServiceUnavailable},
		status:  http.StatusServiceUnavailable,
		code:    codeBillingServiceUnavailable,
		message: msgMatches7,
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
		http.StatusInternalServerError, codeBillingOperationFailed, msgBillingOperationFailed, err,
		"error_type", errorTypeOf(err))
}

// writeBillingInputError renders request-shape failures (malformed JSON,
// unknown fields, missing required fields) through the machine envelope so
// callers branch on a stable code instead of copy.
func writeBillingInputError(c *gin.Context) {
	errorrender.Render(c, http.StatusBadRequest, codeBillingInvalidInput, msgMatches)
}

func errorTypeOf(err error) string {
	return fmt.Sprintf("%T", err)
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgMatches                = "参数错误"            // Matches @ errors.go
	msgMatches2               = "计费功能不存在"         // Matches @ errors.go
	msgMatches3               = "计费功能当前不可计价"      // Matches @ errors.go
	msgMatches4               = "仅已归档功能可以删除"      // Matches @ errors.go
	msgMatches5               = "功能已有历史计费记录，无法删除" // Matches @ errors.go
	msgMatches6               = "报价超过允许上限"        // Matches @ errors.go
	msgMatches7               = "计费服务暂不可用"        // Matches @ errors.go
	msgBillingOperationFailed = "计费操作失败"          // codeBillingOperationFailed
)
