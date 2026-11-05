package billing

// Workspace usage read facet (issue #804 slice).
//
// This file owns the read-only usage surface: GetCurrent over settled
// captures + task snapshots, rendered through the shared error contract.
// It is the first read facet migrated out of billing.go; the remaining
// ledger/records/balance/checkout facets follow the same pattern
// (one facet per file, no behavior change).

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	usagedomain "ideaboard/api/internal/usage/domain"
	usageports "ideaboard/api/internal/usage/ports"

	"github.com/gin-gonic/gin"
)

// renderV1UsageError maps usage read outcomes onto the wire:
// invalid-input sentinels render 4xx (bad range reuses the 422
// validation_failed shape the edge uses for query parsing; bad user
// renders 400 bad_request); every other GetCurrent failure stays 503
// service_unavailable (transient or store-side).
func renderV1UsageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usagedomain.ErrInvalidRange):
		contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
			runtimehttp.MsgRenderV1UsageError, contract.FieldErrors{Fields: map[string]string{"range": "invalid"}})
	case errors.Is(err, usageports.ErrInvalidBillingAccountID):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest33)
	default:
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable40)
	}
	c.Abort()
}

func (h *contractV1BillingHandler) getUsage(c *gin.Context) {
	h.withWorkspace(c, v1WorkspaceOpts{}, func(wctx v1WorkspaceCtx) {
		workspace := wctx.Workspace
		if h.deps.Usage == nil {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable40)
			c.Abort()
			return
		}
		usageRange, err := usagedomain.ParseRange(strings.TrimSpace(c.Query("range")))
		if err != nil {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeValidationFailed), contract.CodeValidationFailed,
				runtimehttp.MsgRenderV1UsageError, contract.FieldErrors{Fields: map[string]string{"range": "invalid"}})
			c.Abort()
			return
		}
		account, ok := h.billingAccountForActor(c, workspace, wctx.Principal.UserID())
		if !ok {
			return
		}
		filter, _, ok := h.consumptionReadScope(c, wctx, account)
		if !ok {
			return
		}
		snapshot, err := h.deps.Usage.GetCurrent(c.Request.Context(), account.InternalID, filter, usageRange)
		if err != nil {
			renderV1UsageError(c, err)
			return
		}
		contract.RenderSuccess(c, snapshot)
	})
}
