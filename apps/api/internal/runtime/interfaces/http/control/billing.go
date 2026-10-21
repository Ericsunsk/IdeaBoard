package control

// Control billing accounts and Registry-backed pricing catalog.
// Accounts are implicit per workspace; catalog operations delegate to
// BillableCatalogService.

import (
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

func writeControlBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrInvalidSKU),
		errors.Is(err, billing.ErrInvalidQuantity),
		errors.Is(err, billing.ErrInvalidCatalogEntry),
		errors.Is(err, billing.ErrInvalidOutputResolution):
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
	case errors.Is(err, billing.ErrSKUNotFound):
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, runtimehttp.MsgControlNotFound)
	case errors.Is(err, billing.ErrPricingUnavailable),
		errors.Is(err, billing.ErrQuoteTotalExceeded):
		contract.RenderError(c, http.StatusUnprocessableEntity, contract.CodeValidationFailed, runtimehttp.MsgValidationFailed)
	case errors.Is(err, billing.ErrItemNotArchived),
		errors.Is(err, billing.ErrArchivedItemInUse):
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, runtimehttp.MsgConflict)
	case errors.Is(err, billing.ErrServiceUnavailable):
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable3)
	}
}

func (d *ContractControlDeps) listControlBillingCatalog(c *gin.Context, _ controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
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
}

func (d *ContractControlDeps) publishControlCatalogPrices(c *gin.Context, ctx controlContext) {
	if d == nil || d.Catalog == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	sku, ok := parseControlSKU(c)
	if !ok {
		return
	}
	var req controlPublishPricesRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	sku, ok := parseControlSKU(c)
	if !ok {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil || req.Enabled == nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
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
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		return
	}
	sku, ok := parseControlSKU(c)
	if !ok {
		return
	}
	result, err := d.Catalog.DeleteArchived(c.Request.Context(), sku, ctx.Operator.UserID())
	if err != nil {
		writeControlBillingError(c, err)
		return
	}
	contract.RenderSuccess(c, result)
}
