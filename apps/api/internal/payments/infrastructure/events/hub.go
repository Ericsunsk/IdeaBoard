package paymentevents

import (
	"encoding/json"
	"log/slog"
	"sync"

	platformevents "ideaboard/api/internal/events"
	paymentports "ideaboard/api/internal/payments/ports"
)

// Hub adapts the Payments event port to the platform user-event transport.
// It only publishes post-commit hints; PostgreSQL remains the payment fact
// source.
type Hub struct {
	mu      sync.RWMutex
	userHub *platformevents.UserEventHub
}

func NewHub() *Hub { return &Hub{userHub: platformevents.NewUserEventHub()} }

func (h *Hub) SetUserEventHub(userHub *platformevents.UserEventHub) {
	if h == nil || userHub == nil {
		return
	}
	h.mu.Lock()
	h.userHub = userHub
	h.mu.Unlock()
}

func (h *Hub) currentUserEventHub() *platformevents.UserEventHub {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.userHub
}

func (h *Hub) Publish(userID int64, event paymentports.PaymentEvent) {
	userHub := h.currentUserEventHub()
	if userHub == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Error("marshal payment user event failed", "user_id", userID, "status", event.Status, "err", err)
		return
	}
	userHub.Publish(platformevents.UserEvent{UserID: userID, Type: platformevents.EventTypePaymentUpdated, Data: payload})
}

// Subscribe exposes the underlying user-scoped stream for composition and
// black-box tests without leaking the hub's synchronization internals.
func (h *Hub) Subscribe(userID int64) (<-chan platformevents.UserEvent, func(), bool) {
	userHub := h.currentUserEventHub()
	if userHub == nil {
		return nil, func() {}, false
	}
	return userHub.Subscribe(userID)
}

var _ paymentports.PaymentEventPublisher = (*Hub)(nil)
