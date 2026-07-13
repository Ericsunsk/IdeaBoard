package billing

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const maxBillingRequestBytes = 64 << 10

type QuoteService interface {
	Quote(context.Context, string, int64) (Quote, error)
}

type AdminService interface {
	CreateSKU(context.Context, CreateSKUInput) (CatalogItem, error)
	UpdateSKU(context.Context, UpdateSKUInput) (CatalogItem, error)
	CreatePrice(context.Context, CreatePriceInput) (PriceInfo, error)
	SetPriceEnabled(context.Context, SetPriceEnabledInput) (PriceInfo, error)
	ListCatalog(context.Context, ListCatalogInput) (CatalogPage, error)
	ListPrices(context.Context, ListPricesInput) (PricePage, error)
}

type Handler struct {
	quoter QuoteService
	admin  AdminService
}

func NewHandler(quoter QuoteService, admin AdminService) *Handler {
	return &Handler{quoter: quoter, admin: admin}
}

type quoteRequest struct {
	SKU      string `json:"sku"`
	Quantity int64  `json:"quantity"`
}

type createSKURequest struct {
	SKU         string `json:"sku"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	Reason      string `json:"reason"`
}

type updateSKURequest struct {
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
	Reason      string  `json:"reason"`
}

type createPriceRequest struct {
	UnitCredits int64  `json:"unit_credits"`
	Enabled     *bool  `json:"enabled"`
	Reason      string `json:"reason"`
}

type setPriceEnabledRequest struct {
	Enabled *bool  `json:"enabled"`
	Reason  string `json:"reason"`
}

func (h *Handler) Quote(c *gin.Context) {
	if h == nil || h.quoter == nil {
		writeError(c, ErrServiceUnavailable)
		return
	}
	var request quoteRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	quote, err := h.quoter.Quote(c.Request.Context(), request.SKU, request.Quantity)
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, quote)
}

func (h *Handler) CreateSKU(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request createSKURequest
	if err := decodeStrictJSON(c, &request); err != nil || request.Enabled == nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	reason, err := normalizeAdminReason(request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	item, err := h.admin.CreateSKU(c.Request.Context(), CreateSKUInput{
		SKU: request.SKU, DisplayName: request.DisplayName, Description: request.Description,
		Enabled: *request.Enabled, ActorID: actorID, Reason: reason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, item)
}

func (h *Handler) UpdateSKU(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request updateSKURequest
	if err := decodeStrictJSON(c, &request); err != nil || (request.DisplayName == nil && request.Description == nil && request.Enabled == nil) {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	reason, err := normalizeAdminReason(request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	item, err := h.admin.UpdateSKU(c.Request.Context(), UpdateSKUInput{
		SKU: c.Param("sku"), DisplayName: request.DisplayName, Description: request.Description,
		Enabled: request.Enabled, ActorID: actorID, Reason: reason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, item)
}

func (h *Handler) CreatePrice(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request createPriceRequest
	if err := decodeStrictJSON(c, &request); err != nil || request.Enabled == nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	reason, err := normalizeAdminReason(request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	price, err := h.admin.CreatePrice(c.Request.Context(), CreatePriceInput{
		SKU: c.Param("sku"), UnitCredits: request.UnitCredits, Enabled: *request.Enabled, ActorID: actorID, Reason: reason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, price)
}

func (h *Handler) SetPriceEnabled(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	version, ok := parsePositiveInt64(c.Param("version"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "价格版本无效")
		return
	}
	var request setPriceEnabledRequest
	if err := decodeStrictJSON(c, &request); err != nil || request.Enabled == nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	reason, err := normalizeAdminReason(request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	price, err := h.admin.SetPriceEnabled(c.Request.Context(), SetPriceEnabledInput{
		SKU: c.Param("sku"), Version: version, Enabled: *request.Enabled, ActorID: actorID, Reason: reason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, price)
}

func (h *Handler) ListCatalog(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	limit, ok := parseOptionalInt(c.Query("limit"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "limit 无效")
		return
	}
	page, err := h.admin.ListCatalog(c.Request.Context(), ListCatalogInput{AfterSKU: c.Query("after_sku"), Limit: limit})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) ListPrices(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	limit, ok := parseOptionalInt(c.Query("limit"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "limit 无效")
		return
	}
	beforeVersion, ok := parseOptionalPositiveInt64(c.Query("before_version"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "before_version 无效")
		return
	}
	page, err := h.admin.ListPrices(c.Request.Context(), ListPricesInput{
		SKU: c.Param("sku"), BeforeVersion: beforeVersion, Limit: limit,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) requireAdmin(c *gin.Context) (int64, bool) {
	if h == nil || h.admin == nil {
		writeError(c, ErrServiceUnavailable)
		return 0, false
	}
	actorID := c.GetInt64("user_id")
	if actorID <= 0 {
		response.Error(c, http.StatusUnauthorized, "未登录")
		return 0, false
	}
	return actorID, true
}

func decodeStrictJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxBillingRequestBytes, destination)
}

func parseOptionalInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(raw)
	return parsed, err == nil && parsed > 0 && parsed <= MaxPageLimit
}

func parseOptionalPositiveInt64(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	return parsePositiveInt64(raw)
}

func parsePositiveInt64(raw string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return parsed, err == nil && parsed > 0
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidSKU), errors.Is(err, ErrInvalidQuantity), errors.Is(err, ErrInvalidCatalogEntry):
		response.Error(c, http.StatusBadRequest, "参数错误")
	case errors.Is(err, ErrSKUAlreadyExists):
		response.Error(c, http.StatusConflict, "SKU 已存在且不可复用")
	case errors.Is(err, ErrSKUNotFound):
		response.Error(c, http.StatusNotFound, "SKU 不存在")
	case errors.Is(err, ErrPriceNotFound):
		response.Error(c, http.StatusNotFound, "价格版本不存在")
	case errors.Is(err, ErrPricingUnavailable):
		response.Error(c, http.StatusUnprocessableEntity, "SKU 当前不可计价")
	case errors.Is(err, ErrQuoteTotalExceeded):
		response.Error(c, http.StatusUnprocessableEntity, "Idea报价超过允许上限")
	case errors.Is(err, ErrServiceUnavailable):
		response.InternalError(c, http.StatusServiceUnavailable, "计费服务暂不可用", err)
	default:
		response.InternalError(c, http.StatusInternalServerError, "计费操作失败", err)
	}
}
