package payments

import (
	"encoding/json"
	"sync"

	"ideaboard/api/internal/events"
)

type PaymentEvent struct {
	PublicToken string `json:"public_token,omitempty"`
	Status      string `json:"status"`
}

type EventHub struct {
	mu      sync.RWMutex
	userHub *events.UserEventHub
}

func NewEventHub() *EventHub {
	return &EventHub{userHub: events.NewUserEventHub()}
}

func (h *EventHub) SetUserEventHub(userHub *events.UserEventHub) {
	if h == nil || userHub == nil {
		return
	}
	h.mu.Lock()
	h.userHub = userHub
	h.mu.Unlock()
}

func (h *EventHub) currentUserEventHub() *events.UserEventHub {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.userHub
}

func (h *EventHub) Publish(userID int64, event PaymentEvent) {
	userHub := h.currentUserEventHub()
	if userHub == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	userHub.Publish(events.UserEvent{UserID: userID, Type: events.EventTypePaymentUpdated, Data: payload})
}
