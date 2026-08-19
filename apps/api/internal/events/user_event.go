package events

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	EventTypeTaskUpdated            = "task.updated"
	EventTypePaymentUpdated         = "payment.updated"
	EventTypeMediaDerivativeUpdated = "media.derivative.updated"
	defaultUserEventGlobalLimit     = 1000
	defaultUserEventPerUserLimit    = 10
)

type UserEvent struct {
	UserID     int64           `json:"user_id"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type UserEventHandler func(context.Context, UserEvent) error

type userEventSubscription struct {
	channel chan UserEvent
	done    chan struct{}
}

// UserEventHub is the single in-process user-scoped event fanout. It owns
// connection limits and drop-oldest delivery so every event type follows the
// same lifecycle regardless of its producer.
type UserEventHub struct {
	mu             sync.RWMutex
	subscribers    map[int64][]*userEventSubscription
	globalLimit    int
	perUserLimit   int
	total          int
	userConnCounts map[int64]int
	remote         func(UserEvent)
}

func NewUserEventHub() *UserEventHub {
	return NewUserEventHubWithLimits(defaultUserEventGlobalLimit, defaultUserEventPerUserLimit)
}

func NewUserEventHubWithLimits(globalLimit, perUserLimit int) *UserEventHub {
	if globalLimit <= 0 {
		globalLimit = defaultUserEventGlobalLimit
	}
	if perUserLimit <= 0 {
		perUserLimit = defaultUserEventPerUserLimit
	}
	return &UserEventHub{
		subscribers:    make(map[int64][]*userEventSubscription),
		globalLimit:    globalLimit,
		perUserLimit:   perUserLimit,
		userConnCounts: make(map[int64]int),
	}
}

func (h *UserEventHub) Subscribe(userID int64) (<-chan UserEvent, func(), bool) {
	if h == nil || userID <= 0 {
		return nil, nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= h.globalLimit || h.userConnCounts[userID] >= h.perUserLimit {
		return nil, nil, false
	}
	sub := &userEventSubscription{channel: make(chan UserEvent, 16), done: make(chan struct{})}
	h.subscribers[userID] = append(h.subscribers[userID], sub)
	h.total++
	h.userConnCounts[userID]++
	return sub.channel, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		select {
		case <-sub.done:
			return
		default:
		}
		close(sub.done)
		entries := h.subscribers[userID]
		for index, entry := range entries {
			if entry != sub {
				continue
			}
			h.subscribers[userID] = append(entries[:index], entries[index+1:]...)
			h.total--
			h.userConnCounts[userID]--
			break
		}
		if len(h.subscribers[userID]) == 0 {
			delete(h.subscribers, userID)
			delete(h.userConnCounts, userID)
		}
	}, true
}

func (h *UserEventHub) SetRemotePublisher(publisher func(UserEvent)) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.remote = publisher
	h.mu.Unlock()
}

func (h *UserEventHub) SubscriberCount(userID int64) int {
	if h == nil || userID <= 0 {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[userID])
}

func (h *UserEventHub) Publish(event UserEvent) {
	h.publish(event, true)
}

func (h *UserEventHub) PublishRemote(event UserEvent) {
	h.publish(event, false)
}

func (h *UserEventHub) publish(event UserEvent, forward bool) {
	if h == nil || event.UserID <= 0 || strings.TrimSpace(event.Type) == "" {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	h.mu.RLock()
	entries := append([]*userEventSubscription(nil), h.subscribers[event.UserID]...)
	remote := h.remote
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
	if forward && remote != nil {
		remote(event)
	}
}

func NewUserEvent(userID int64, eventType string, data any) (UserEvent, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return UserEvent{}, err
	}
	return UserEvent{UserID: userID, Type: strings.TrimSpace(eventType), Data: payload, OccurredAt: time.Now().UTC()}, nil
}
