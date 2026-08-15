package billing

import (
	"context"
	"errors"
	"net/http"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const maxBillingRequestBytes = 64 << 10

type QuoteService interface {
	QuoteAtResolution(context.Context, string, OutputResolution, int64) (Quote, error)
}

type BillableCatalogService interface {
	List(context.Context) ([]BillableItemCatalogCard, error)
	PublishPrices(context.Context, string, map[OutputResolution]int64, int64) (map[OutputResolution]PriceInfo, error)
	SetAvailability(context.Context, string, bool, int64) (AvailabilityInfo, error)
	DeleteArchived(context.Context, string, int64) (ArchivedItemDeleteResult, error)
}

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
		writeError(c, ErrServiceUnavailable)
		return
	}
	var request quoteRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	resolution, err := NormalizeOutputResolution(request.OutputResolution)
	if request.OutputResolution == "" {
		resolution = OutputResolution1K
		err = nil
	}
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
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	prices := make(map[OutputResolution]int64, len(request.Prices))
	for rawResolution, unitCredits := range request.Prices {
		resolution, err := NormalizeOutputResolution(rawResolution)
		if err != nil {
			writeError(c, err)
			return
		}
		prices[resolution] = unitCredits
	}
	if _, hasOneK := prices[OutputResolution1K]; !hasOneK {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, ErrServiceUnavailable)
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
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, ErrServiceUnavailable)
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
		writeError(c, ErrServiceUnavailable)
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
		writeError(c, ErrServiceUnavailable)
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

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidSKU), errors.Is(err, ErrInvalidQuantity), errors.Is(err, ErrInvalidCatalogEntry), errors.Is(err, ErrInvalidOutputResolution):
		response.Error(c, http.StatusBadRequest, "参数错误")
	case errors.Is(err, ErrSKUNotFound):
		response.Error(c, http.StatusNotFound, "SKU 不存在")
	case errors.Is(err, ErrPricingUnavailable):
		response.Error(c, http.StatusUnprocessableEntity, "SKU 当前不可计价")
	case errors.Is(err, ErrItemNotArchived):
		response.Error(c, http.StatusConflict, "仅已归档功能可以删除")
	case errors.Is(err, ErrArchivedItemInUse):
		response.Error(c, http.StatusConflict, "功能已有历史计费记录，无法删除")
	case errors.Is(err, ErrQuoteTotalExceeded):
		response.Error(c, http.StatusUnprocessableEntity, "Idea报价超过允许上限")
	case errors.Is(err, ErrServiceUnavailable):
		response.InternalError(c, http.StatusServiceUnavailable, "计费服务暂不可用", err)
	default:
		response.InternalError(c, http.StatusInternalServerError, "计费操作失败", err)
	}
}
