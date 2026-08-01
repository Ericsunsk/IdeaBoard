package payments

import "sync"

type PaymentEvent struct {
	PublicToken string `json:"public_token,omitempty"`
	Status      string `json:"status"`
}

type paymentSubscription struct {
	channel chan PaymentEvent
	done    chan struct{}
}

type EventHub struct {
	mu          sync.RWMutex
	subscribers map[int64][]*paymentSubscription
}

func NewEventHub() *EventHub {
	return &EventHub{subscribers: make(map[int64][]*paymentSubscription)}
}

func (h *EventHub) Subscribe(userID int64) (<-chan PaymentEvent, func()) {
	if h == nil || userID <= 0 {
		return nil, func() {}
	}
	subscription := &paymentSubscription{channel: make(chan PaymentEvent, 4), done: make(chan struct{})}
	h.mu.Lock()
	h.subscribers[userID] = append(h.subscribers[userID], subscription)
	h.mu.Unlock()
	return subscription.channel, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		select {
		case <-subscription.done:
			return
		default:
			close(subscription.done)
		}
		entries := h.subscribers[userID]
		for index, entry := range entries {
			if entry == subscription {
				h.subscribers[userID] = append(entries[:index], entries[index+1:]...)
				break
			}
		}
		if len(h.subscribers[userID]) == 0 {
			delete(h.subscribers, userID)
		}
	}
}

func (h *EventHub) Publish(userID int64, event PaymentEvent) {
	if h == nil || userID <= 0 {
		return
	}
	h.mu.RLock()
	entries := append([]*paymentSubscription(nil), h.subscribers[userID]...)
	h.mu.RUnlock()
	for _, entry := range entries {
		select {
		case <-entry.done:
			continue
		default:
		}
		select {
		case entry.channel <- event:
		default:
			select {
			case <-entry.channel:
			default:
			}
			select {
			case entry.channel <- event:
			default:
			}
		}
	}
}
