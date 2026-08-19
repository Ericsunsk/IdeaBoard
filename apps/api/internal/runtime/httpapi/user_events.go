package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"

	"github.com/gin-gonic/gin"
)

const userEventsHeartbeatInterval = 15 * time.Second

type userEventStream interface {
	Subscribe(int64) (<-chan events.UserEvent, func(), bool)
}

// UserEventStreamHandler is the single SSE delivery seam for user-scoped
// invalidations. Event payloads are intentionally small; clients reload
// authoritative views from the API.
type UserEventStreamHandler struct {
	stream userEventStream
}

func NewUserEventStreamHandler(stream userEventStream) *UserEventStreamHandler {
	return &UserEventStreamHandler{stream: stream}
}

func (h *UserEventStreamHandler) StreamUserEvents(c *gin.Context) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.stream == nil {
		response.Error(c, http.StatusServiceUnavailable, "用户事件服务暂不可用")
		return
	}
	userEvents, unsubscribe, accepted := h.stream.Subscribe(identityPrincipal.UserID())
	if !accepted || userEvents == nil {
		response.Error(c, http.StatusTooManyRequests, "SSE 连接数已达上限，请稍后重试")
		return
	}
	defer unsubscribe()

	if err := sse.Prepare(c); err != nil {
		slog.Warn("clear user events SSE write deadline failed", "err", err)
	}
	c.Header("Cache-Control", "no-store")
	heartbeat := time.NewTicker(userEventsHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-userEvents:
			if !open {
				return
			}
			if err := sse.WriteEvent(c, event.Type, json.RawMessage(event.Data)); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := c.Writer.Write([]byte(":keepalive\n\n")); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
