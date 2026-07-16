package billing

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
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
	ListCatalog(context.Context, ListCatalogInput) (CatalogPage, error)
	ListPrices(context.Context, ListPricesInput) (PricePage, error)
}

type BillableCatalogService interface {
	List(context.Context) ([]BillableItemCatalogCard, error)
	PublishPrice(context.Context, string, int64, int64) (PriceInfo, error)
}

type Handler struct {
	quoter  QuoteService
	admin   AdminService
	catalog BillableCatalogService
}

func NewHandler(quoter QuoteService, admin AdminService, catalog BillableCatalogService) *Handler {
	return &Handler{quoter: quoter, admin: admin, catalog: catalog}
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
	UnitCredits *int64 `json:"unit_credits"`
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
	action, err := adminaction.New(actorID, request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	item, err := h.admin.CreateSKU(c.Request.Context(), CreateSKUInput{
		SKU: request.SKU, DisplayName: request.DisplayName, Description: request.Description,
		Enabled: *request.Enabled, Action: action,
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
	action, err := adminaction.New(actorID, request.Reason)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "reason 无效")
		return
	}
	item, err := h.admin.UpdateSKU(c.Request.Context(), UpdateSKUInput{
		SKU: c.Param("sku"), DisplayName: request.DisplayName, Description: request.Description,
		Enabled: request.Enabled, Action: action,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.SuccessDTO(c, item)
}

func (h *Handler) PublishPrice(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	var request createPriceRequest
	if err := decodeStrictJSON(c, &request); err != nil || request.UnitCredits == nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if h == nil || h.catalog == nil {
		writeError(c, ErrServiceUnavailable)
		return
	}
	price, err := h.catalog.PublishPrice(c.Request.Context(), c.Param("sku"), *request.UnitCredits, admin.UserID())
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
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	if h == nil || h.admin == nil {
		writeError(c, ErrServiceUnavailable)
		return 0, false
	}
	return identityPrincipal.UserID(), true
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
