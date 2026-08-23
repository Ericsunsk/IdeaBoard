package adminbranding

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

type Handler struct{ store Store }

func NewHandler(store Store) *Handler { return &Handler{store: store} }

type listResponse struct {
	Overrides []Override `json:"overrides"`
}

type updateRequest struct {
	LogoKey string `json:"logo_key"`
}

func (h *Handler) List(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	store, ok := h.requireStore(c)
	if !ok {
		return
	}
	overrides, err := store.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, "读取品牌 Logo 配置失败", err)
		return
	}
	response.SuccessDTO(c, listResponse{Overrides: overrides})
}

func (h *Handler) Upsert(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	store, ok := h.requireStore(c)
	if !ok {
		return
	}
	scope, targetKey, ok := parseTarget(c)
	if !ok {
		return
	}
	var request updateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	logoKey := strings.TrimSpace(request.LogoKey)
	if err := Validate(scope, targetKey, logoKey); h.respondStoreError(c, err) {
		return
	}
	override, err := store.Upsert(c.Request.Context(), scope, targetKey, logoKey)
	if h.respondStoreError(c, err) {
		return
	}
	response.SuccessDTO(c, override)
}

func (h *Handler) Delete(c *gin.Context) {
	if _, ok := principal.RequireAdmin(c); !ok {
		return
	}
	store, ok := h.requireStore(c)
	if !ok {
		return
	}
	scope, targetKey, ok := parseTarget(c)
	if !ok {
		return
	}
	if _, err := store.Delete(c.Request.Context(), scope, targetKey); h.respondStoreError(c, err) {
		return
	}
	response.Success(c, nil)
}

func (h *Handler) requireStore(c *gin.Context) (Store, bool) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusInternalServerError, "品牌 Logo 配置服务未初始化", errors.New("admin brand logo override store is unavailable"))
		return nil, false
	}
	return h.store, true
}

func parseTarget(c *gin.Context) (Scope, string, bool) {
	scope := Scope(strings.TrimSpace(c.Param("scope")))
	targetKey := strings.TrimSpace(c.Param("target_key"))
	if err := ValidateTarget(scope, targetKey); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return scope, targetKey, true
}

func (h *Handler) respondStoreError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if validation.Is(err) {
		response.Error(c, http.StatusBadRequest, err.Error())
		return true
	}
	response.InternalError(c, http.StatusInternalServerError, "保存品牌 Logo 配置失败", err)
	return true
}
