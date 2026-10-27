package v1

// User notifications HTTP endpoints (#958, ADR-0040).

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/gin-gonic/gin"
)

type v1NotificationDTO struct {
	ID            string                      `json:"id"`
	Type          string                      `json:"type"`
	Title         string                      `json:"title"`
	Body          string                      `json:"body"`
	ActionPayload notifications.ActionPayload `json:"action_payload"`
	Status        string                      `json:"status"`
	ReadAt        *string                     `json:"read_at,omitempty"`
	CreatedAt     string                      `json:"created_at"`
	UpdatedAt     string                      `json:"updated_at"`
}

type v1NotificationListDTO struct {
	Items  []v1NotificationDTO `json:"items"`
	Total  int64               `json:"total"`
	Limit  int32               `json:"limit"`
	Offset int32               `json:"offset"`
}

type v1UnreadCountDTO struct {
	Count int64 `json:"count"`
}

type v1MarkAllReadDTO struct {
	Updated int64 `json:"updated"`
}

func (h *contractV1Handler) toNotificationDTO(n *notifications.Notification) v1NotificationDTO {
	if n == nil {
		return v1NotificationDTO{}
	}
	dto := v1NotificationDTO{
		ID:            n.PublicID,
		Type:          string(n.Type),
		Title:         n.Title,
		Body:          n.Body,
		ActionPayload: n.ActionPayload,
		Status:        string(n.Status),
		CreatedAt:     v1FormatTime(n.CreatedAt),
		UpdatedAt:     v1FormatTime(n.UpdatedAt),
	}
	if n.ActionPayload == nil {
		dto.ActionPayload = make(notifications.ActionPayload)
	}
	if n.ReadAt != nil {
		formatted := v1FormatTime(*n.ReadAt)
		dto.ReadAt = &formatted
	}
	return dto
}

func (h *contractV1Handler) toNotificationDTOs(items []*notifications.Notification) []v1NotificationDTO {
	dtos := make([]v1NotificationDTO, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		dtos = append(dtos, h.toNotificationDTO(item))
	}
	return dtos
}

// listUserNotifications lists notifications for the authenticated user (GET /api/v1/user/notifications).
func (h *contractV1Handler) listUserNotifications(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.Notifications == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	if status != "" {
		if status != string(notifications.StatusUnread) &&
			status != string(notifications.StatusRead) &&
			status != string(notifications.StatusArchived) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Invalid notification status filter")
			c.Abort()
			return
		}
	}

	category := strings.TrimSpace(c.Query("category"))
	if category != "" && category != "all" && category != "invites" && category != "billing" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, "Invalid notification category filter")
		c.Abort()
		return
	}

	// Read-tolerant pagination: limit/page_size above 100 are clamped to 100
	// (no 400); invalid values fall back to the default 20. page_size is an
	// accepted alias for limit. See openapi listUserNotifications docs.
	limit := int32(20)
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 32); err == nil && parsed > 0 {
			limit = int32(parsed)
			if limit > 100 {
				limit = 100
			}
		}
	} else if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.ParseInt(ps, 10, 32); err == nil && parsed > 0 {
			limit = int32(parsed)
			if limit > 100 {
				limit = 100
			}
		}
	}

	offset := int32(0)
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 32); err == nil && parsed >= 0 {
			offset = int32(parsed)
		}
	}

	items, total, err := h.deps.Notifications.ListNotifications(c.Request.Context(), principal.UserID(), notifications.ListFilter{Status: status, Category: category}, limit, offset)
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	contract.RenderSuccess(c, v1NotificationListDTO{
		Items:  h.toNotificationDTOs(items),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// getUserNotificationsUnreadCount returns the unread notification count for the authenticated user (GET /api/v1/user/notifications/unread-count).
func (h *contractV1Handler) getUserNotificationsUnreadCount(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.Notifications == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	count, err := h.deps.Notifications.CountUnread(c.Request.Context(), principal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	contract.RenderSuccess(c, v1UnreadCountDTO{Count: count})
}

// markUserNotificationRead marks a specific notification as read (POST /api/v1/user/notifications/:id/read).
func (h *contractV1Handler) markUserNotificationRead(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.Notifications == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" || !strings.HasPrefix(id, contract.PrefixNotification) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Notification not found")
		c.Abort()
		return
	}

	item, err := h.deps.Notifications.MarkRead(c.Request.Context(), principal.UserID(), id)
	if err != nil {
		if errors.Is(err, notifications.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Notification not found")
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	contract.RenderSuccess(c, h.toNotificationDTO(item))
}

// markAllUserNotificationsRead marks all unread notifications as read (POST /api/v1/user/notifications/read-all).
func (h *contractV1Handler) markAllUserNotificationsRead(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.Notifications == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	updated, err := h.deps.Notifications.MarkAllRead(c.Request.Context(), principal.UserID())
	if err != nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	contract.RenderSuccess(c, v1MarkAllReadDTO{Updated: int64(len(updated))})
}

// archiveUserNotification archives a notification (POST /api/v1/user/notifications/:id/archive).
func (h *contractV1Handler) archiveUserNotification(c *gin.Context) {
	principal, ok := v1RequestPrincipal(c)
	if !ok {
		return
	}
	if h.deps.Notifications == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" || !strings.HasPrefix(id, contract.PrefixNotification) {
		contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Notification not found")
		c.Abort()
		return
	}

	item, err := h.deps.Notifications.Archive(c.Request.Context(), principal.UserID(), id)
	if err != nil {
		if errors.Is(err, notifications.ErrNotFound) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeNotFound, "Notification not found")
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable)
		c.Abort()
		return
	}

	contract.RenderSuccess(c, h.toNotificationDTO(item))
}
