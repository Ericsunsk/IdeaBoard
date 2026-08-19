package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/sse"
	"ideaboard/api/internal/tasking/taskservice"

	"github.com/gin-gonic/gin"
)

const userEventsHeartbeatInterval = 15 * time.Second

type userTaskEventStream interface {
	SubscribeUser(int64) (<-chan taskservice.TaskEvent, func(), bool)
}

type userPaymentEventStream interface {
	Subscribe(int64) (<-chan payments.PaymentEvent, func())
}

// UserEventStreamHandler multiplexes user-scoped task and payment invalidations.
// Event payloads are intentionally small; clients reload authoritative views from the API.
type UserEventStreamHandler struct {
	tasks    userTaskEventStream
	payments userPaymentEventStream
}

func NewUserEventStreamHandler(tasks userTaskEventStream, payments userPaymentEventStream) *UserEventStreamHandler {
	return &UserEventStreamHandler{tasks: tasks, payments: payments}
}

func (h *UserEventStreamHandler) StreamUserEvents(c *gin.Context) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	if h == nil || h.tasks == nil || h.payments == nil {
		response.Error(c, http.StatusServiceUnavailable, "用户事件服务暂不可用")
		return
	}
	taskEvents, unsubscribeTasks, accepted := h.tasks.SubscribeUser(identityPrincipal.UserID())
	if !accepted || taskEvents == nil {
		response.Error(c, http.StatusTooManyRequests, "SSE 连接数已达上限，请稍后重试")
		return
	}
	defer unsubscribeTasks()
	paymentEvents, unsubscribePayments := h.payments.Subscribe(identityPrincipal.UserID())
	if paymentEvents == nil {
		response.Error(c, http.StatusServiceUnavailable, "支付事件服务暂不可用")
		return
	}
	defer unsubscribePayments()

	c.Header("Cache-Control", "no-store")
	if err := sse.Prepare(c); err != nil {
		slog.Warn("clear user events SSE write deadline failed", "err", err)
	}
	heartbeat := time.NewTicker(userEventsHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-taskEvents:
			if !open {
				taskEvents = nil
				continue
			}
			if err := sse.WriteEvent(c, "task", event); err != nil {
				return
			}
		case event, open := <-paymentEvents:
			if !open {
				paymentEvents = nil
				continue
			}
			if err := sse.WriteEvent(c, "payment", event); err != nil {
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
