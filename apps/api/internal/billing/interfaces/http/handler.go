package httpapi

import (
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const maxBillingRequestBytes = 64 << 10

type Handler struct {
	quoter  QuoteService
	catalog BillableCatalogService
}

func NewHandler(quoter QuoteService, catalog BillableCatalogService) *Handler {
	return &Handler{quoter: quoter, catalog: catalog}
}

type quoteRequest struct {
	SKU              string `json:"sku"`
	OutputResolution string `json:"output_resolution"`
	Quantity         int64  `json:"quantity"`
}

type createPriceRequest struct {
	Prices map[string]int64 `json:"prices"`
}

type setAvailabilityRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h *Handler) Quote(c *gin.Context) {
	if _, ok := principal.Require(c); !ok {
		return
	}
	if h == nil || h.quoter == nil {
		writeError(c, billing.ErrServiceUnavailable)
		return
	}
	var request quoteRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeBillingInputError(c)
		return
	}
	resolution, err := billing.NormalizeOutputResolution(request.OutputResolution)
	if err != nil {
		writeError(c, err)
		return
	}
	quote, err := h.quoter.QuoteAtResolution(c.Request.Context(), request.SKU, resolution, request.Quantity)
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, quote)
}

func (h *Handler) PublishPrice(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	var request createPriceRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		writeBillingInputError(c)
		return
	}
	prices := make(map[billing.OutputResolution]int64, len(request.Prices))
	for rawResolution, unitCredits := range request.Prices {
		resolution, err := billing.NormalizeOutputResolution(rawResolution)
		if err != nil {
			writeError(c, err)
			return
		}
		prices[resolution] = unitCredits
	}
	if len(prices) == 0 {
		writeBillingInputError(c)
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, billing.ErrServiceUnavailable)
		return
	}
	published, err := h.catalog.PublishPrices(c.Request.Context(), c.Param("sku"), prices, admin.UserID())
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"prices": published})
}

func (h *Handler) SetAvailability(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	var request setAvailabilityRequest
	if err := decodeStrictJSON(c, &request); err != nil || request.Enabled == nil {
		writeBillingInputError(c)
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, billing.ErrServiceUnavailable)
		return
	}
	availability, err := h.catalog.SetAvailability(c.Request.Context(), c.Param("sku"), *request.Enabled, admin.UserID())
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, availability)
}

func (h *Handler) ListBillableCatalog(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, billing.ErrServiceUnavailable)
		return
	}
	items, err := h.catalog.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"items": items})
}

func (h *Handler) DeleteArchivedItem(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, billing.ErrServiceUnavailable)
		return
	}
	result, err := h.catalog.DeleteArchived(c.Request.Context(), c.Param("sku"), admin.UserID())
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, result)
}

func decodeStrictJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxBillingRequestBytes, destination)
}
