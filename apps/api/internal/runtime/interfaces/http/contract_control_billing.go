package httpapi

// Control billing family (issue #808): billing accounts and the
// Registry-backed pricing catalog behind control:billing.
//
// The billing account is implicit per workspace (no public account id in
// URLs); the catalog delegates to the billing BillableCatalogService
// seam. Legacy /api/admin/billing/* routes are untouched.

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

type controlBillingAccount struct {
	WorkspaceID string    `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (d *ContractControlDeps) listControlBillingAccounts(c *gin.Context, _ controlContext) {
	if d == nil || d.Tenancy == nil || d.BillingAccounts == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return
	}
	workspaceID := strings.TrimSpace(c.Query("workspace_id"))
	if !contract.IsWorkspaceID(workspaceID) {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest3)
		return
	}
	workspace, err := d.Tenancy.GetWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		writeControlTenancyError(c, err, msgListControlBillingAccounts)
		return
	}
	account, err := d.BillingAccounts.EnsureBillingAccountForWorkspace(c.Request.Context(), workspace.InternalID, workspace.PublicID)
	if err != nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable2)
		return
	}
	items := []controlBillingAccount{{
		WorkspaceID: account.WorkspacePublicID,
		CreatedAt:   account.CreatedAt,
		UpdatedAt:   account.UpdatedAt,
	}}
	contract.RenderPaginated(c, items, contract.NewPageMeta("", false))
}

func writeControlBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrInvalidSKU),
		errors.Is(err, billing.ErrInvalidQuantity),
		errors.Is(err, billing.ErrInvalidCatalogEntry),
		errors.Is(err, billing.ErrInvalidOutputResolution):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
	case errors.Is(err, billing.ErrSKUNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound)
	case errors.Is(err, billing.ErrPricingUnavailable),
		errors.Is(err, billing.ErrQuoteTotalExceeded):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, msgValidationFailed)
	case errors.Is(err, billing.ErrItemNotArchived),
		errors.Is(err, billing.ErrArchivedItemInUse):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, msgConflict)
	case errors.Is(err, billing.ErrServiceUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable3)
	}
}

func (d *ContractControlDeps) listControlBillingCatalog(c *gin.Context, _ controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return
	}
	items, err := d.Catalog.List(c.Request.Context())
	if err != nil {
		writeControlBillingError(c, err)
		return
	}
	var payload any = items
	if items == nil {
		payload = []any{}
	}
	contract.RenderSuccess(c, struct {
		Items any `json:"items"`
	}{Items: payload})
}

type controlPublishPricesRequest struct {
	Prices map[string]int64 `json:"prices"`
	// Reason rides the operator-note convention (header wins; the guard
	// extracts and audits it centrally). Accepted here only so strict
	// decoding does not 400 the documented shape.
	Reason *string `json:"reason,omitempty"`
}

func (d *ContractControlDeps) publishControlCatalogPrices(c *gin.Context, ctx controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return
	}
	sku := strings.TrimSpace(c.Param("sku"))
	if sku == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return
	}
	var req controlPublishPricesRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	prices := make(map[billing.OutputResolution]int64, len(req.Prices))
	for rawResolution, unitCredits := range req.Prices {
		resolution, err := billing.NormalizeOutputResolution(rawResolution)
		if err != nil {
			writeControlBillingError(c, err)
			return
		}
		prices[resolution] = unitCredits
	}
	if len(prices) == 0 {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	published, err := d.Catalog.PublishPrices(c.Request.Context(), sku, prices, ctx.Operator.UserID())
	if err != nil {
		writeControlBillingError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Prices any `json:"prices"`
	}{Prices: published})
}

func (d *ContractControlDeps) setControlCatalogAvailability(c *gin.Context, ctx controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return
	}
	sku := strings.TrimSpace(c.Param("sku"))
	if sku == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return
	}
	var req struct {
		Enabled *bool   `json:"enabled"`
		Reason  *string `json:"reason,omitempty"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil || req.Enabled == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	availability, err := d.Catalog.SetAvailability(c.Request.Context(), sku, *req.Enabled, ctx.Operator.UserID())
	if err != nil {
		writeControlBillingError(c, err)
		return
	}
	contract.RenderSuccess(c, availability)
}

func (d *ContractControlDeps) deleteControlCatalogItem(c *gin.Context, ctx controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return
	}
	sku := strings.TrimSpace(c.Param("sku"))
	if sku == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest5)
		return
	}
	result, err := d.Catalog.DeleteArchived(c.Request.Context(), sku, ctx.Operator.UserID())
	if err != nil {
		writeControlBillingError(c, err)
		return
	}
	contract.RenderSuccess(c, result)
}
