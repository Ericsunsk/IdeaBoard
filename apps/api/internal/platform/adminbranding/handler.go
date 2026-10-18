package adminbranding

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// RequireAdminFunc reports whether the request carries admin authority,
// rendering the denial response otherwise. The composition root injects the
// identity-backed implementation so platform stays independent of identity.
type RequireAdminFunc func(c *gin.Context) bool

type Handler struct {
	store        Store
	requireAdmin RequireAdminFunc
}

func NewHandler(store Store, requireAdmin RequireAdminFunc) *Handler {
	return &Handler{store: store, requireAdmin: requireAdmin}
}

type listResponse struct {
	Overrides []Override `json:"overrides"`
}

type updateRequest struct {
	LogoKey string `json:"logo_key"`
}

func (h *Handler) List(c *gin.Context) {
	if !h.checkAdmin(c) {
		return
	}
	store, ok := h.requireStore(c)
	if !ok {
		return
	}
	overrides, err := store.List(c.Request.Context())
	if err != nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgPlatformAdminbrandingList, err)
		return
	}
	response.SuccessDTO(c, listResponse{Overrides: overrides})
}

func (h *Handler) Upsert(c *gin.Context) {
	if !h.checkAdmin(c) {
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
		response.Error(c, http.StatusBadRequest, errorrender.MsgUpsert)
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
	if !h.checkAdmin(c) {
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

// checkAdmin enforces admin authority through the injected port. A missing
// port denies fail-closed so platform never serves admin writes by default.
func (h *Handler) checkAdmin(c *gin.Context) bool {
	if h == nil || h.requireAdmin == nil {
		response.Error(c, http.StatusForbidden, errorrender.MsgCheckAdmin)
		return false
	}
	return h.requireAdmin(c)
}

func (h *Handler) requireStore(c *gin.Context) (Store, bool) {
	if h == nil || h.store == nil {
		response.InternalError(c, http.StatusInternalServerError, errorrender.MsgRequireStore, errors.New("admin brand logo override store is unavailable"))
		return nil, false
	}
	return h.store, true
}

func parseTarget(c *gin.Context) (Scope, string, bool) {
	scope := Scope(strings.TrimSpace(c.Param("scope")))
	targetKey := strings.TrimSpace(c.Param("target_key"))
	if err := ValidateTarget(scope, targetKey); err != nil {
		slog.Warn("admin brand logo target invalid", "err", err)
		response.Error(c, http.StatusBadRequest, errorrender.MsgParseTarget)
		return "", "", false
	}
	return scope, targetKey, true
}

func (h *Handler) respondStoreError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errorrender.IsValidationError(err) {
		slog.Warn("admin brand logo validation failed", "err", err)
		response.Error(c, http.StatusBadRequest, errorrender.MsgParseTarget)
		return true
	}
	response.InternalError(c, http.StatusInternalServerError, errorrender.MsgRespondStoreError, err)
	return true
}
