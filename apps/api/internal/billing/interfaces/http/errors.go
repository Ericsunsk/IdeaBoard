package http

import (
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

// billingDomainErrorTable is the single source of truth for mapping billing
// failures to the machine-readable envelope. Rows are ordered from the most
// specific outcome to the most generic; the first match wins.
var billingDomainErrorTable = []errorrender.Mapping{
	{
		Targets: []error{billing.ErrInvalidSKU, billing.ErrInvalidQuantity, billing.ErrInvalidCatalogEntry, billing.ErrInvalidOutputResolution},
		Code:    codeBillingInvalidInput,
		Message: errorrender.MsgMatches,
	},
	{
		Targets: []error{billing.ErrSKUNotFound},
		Code:    codeBillingSKUNotFound,
		Message: errorrender.MsgMatches2,
	},
	{
		Targets: []error{billing.ErrPricingUnavailable},
		Code:    codeBillingPricingUnavailable,
		Message: errorrender.MsgMatches3,
	},
	{
		Targets: []error{billing.ErrItemNotArchived},
		Code:    codeBillingItemNotArchived,
		Message: errorrender.MsgMatches4,
	},
	{
		Targets: []error{billing.ErrArchivedItemInUse},
		Code:    codeBillingArchivedItemInUse,
		Message: errorrender.MsgMatches5,
	},
	{
		Targets: []error{billing.ErrQuoteTotalExceeded},
		Code:    codeBillingQuoteTotalExceeded,
		Message: errorrender.MsgMatches6,
	},
	{
		Targets: []error{billing.ErrServiceUnavailable},
		Code:    codeBillingServiceUnavailable,
		Message: errorrender.MsgMatches7,
	},
}

func writeError(c *gin.Context, err error) {
	errorrender.RenderMapped(c, err, billingDomainErrorTable, errorrender.MappedOptions{
		DefaultStatus:  http.StatusInternalServerError,
		DefaultCode:    codeBillingOperationFailed,
		DefaultMessage: errorrender.MsgBillingOperationFailed,
		UnmatchedLog:   "billing operation failed",
		UnmatchedAttrs: errorrender.UnmatchedErrorTypeAttr,
		// 5xx matches (service unavailable) stay attributable like the
		// unmatched default; 4xx matches render without logging.
		MatchedLogAtOrAbove: 500,
		MatchedLog:          "billing operation failed",
		MatchedAttrs:        errorrender.MatchErrorTypeAttrWithErr,
	})
}

// writeBillingInputError renders request-shape failures (malformed JSON,
// unknown fields, missing required fields) through the machine envelope so
// callers branch on a stable code instead of copy.
func writeBillingInputError(c *gin.Context) {
	errorrender.Render(c, http.StatusBadRequest, codeBillingInvalidInput, errorrender.MsgMatches)
}
