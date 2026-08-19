package taskservice

import (
	"sync"
)

// TaskEvent is emitted when a task's status changes.
// It carries minimal data; consumers should call GetUserTask for the full safe view.
type TaskEvent struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type taskSubscription struct {
	ch     chan TaskEvent
	done   chan struct{}
	userID int64
}

type userSubscription struct {
	ch     chan TaskEvent
	done   chan struct{}
	userID int64
}

// EventHub manages per-task subscriptions for SSE streaming.
// It is safe for concurrent use.
type EventHub struct {
	mu             sync.RWMutex
	subs           map[string][]*taskSubscription
	userSubs       map[int64][]*userSubscription
	globalLimit    int
	perUserLimit   int
	total          int64
	userConnCounts map[int64]int
}

const (
	defaultSSEGlobalLimit  = 1000
	defaultSSEPerUserLimit = 10
)

func NewEventHub() *EventHub {
	return newEventHubWithLimits(defaultSSEGlobalLimit, defaultSSEPerUserLimit)
}

func newEventHubWithLimits(globalLimit, perUserLimit int) *EventHub {
	if globalLimit <= 0 {
		globalLimit = defaultSSEGlobalLimit
	}
	if perUserLimit <= 0 {
		perUserLimit = defaultSSEPerUserLimit
	}
	return &EventHub{
		subs:           make(map[string][]*taskSubscription),
		userSubs:       make(map[int64][]*userSubscription),
		globalLimit:    globalLimit,
		perUserLimit:   perUserLimit,
		userConnCounts: make(map[int64]int),
	}
}

// SubscribeUser returns task status events for all tasks owned by userID.
// The stream is an invalidation signal; clients should load the task view again.
func (h *EventHub) SubscribeUser(userID int64) (<-chan TaskEvent, func(), bool) {
	if h == nil {
		return nil, nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if userID <= 0 || int(h.total) >= h.globalLimit || h.userConnCounts[userID] >= h.perUserLimit {
		return nil, nil, false
	}
	sub := &userSubscription{ch: make(chan TaskEvent, 8), done: make(chan struct{}), userID: userID}
	h.userSubs[userID] = append(h.userSubs[userID], sub)
	h.total++
	h.userConnCounts[userID]++
	return sub.ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		select {
		case <-sub.done:
			return
		default:
		}
		close(sub.done)
		entries := h.userSubs[userID]
		for i, entry := range entries {
			if entry == sub {
				h.userSubs[userID] = append(entries[:i], entries[i+1:]...)
				h.total--
				h.userConnCounts[userID]--
				break
			}
		}
		if len(h.userSubs[userID]) == 0 {
			delete(h.userSubs, userID)
		}
	}, true
}

// Subscribe returns a channel that receives TaskEvents for the given taskID,
// and an unsubscribe function. Unsubscribe removes the subscriber from the hub
// but does not close the channel.
func (h *EventHub) Subscribe(taskID string, userID int64) (<-chan TaskEvent, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if int(h.total) >= h.globalLimit {
		return nil, nil, false
	}
	if userID != 0 && h.userConnCounts[userID] >= h.perUserLimit {
		return nil, nil, false
	}

	sub := &taskSubscription{
		ch:     make(chan TaskEvent, 8),
		done:   make(chan struct{}),
		userID: userID,
	}
	h.subs[taskID] = append(h.subs[taskID], sub)
	h.total++
	if userID != 0 {
		h.userConnCounts[userID]++
	}

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		select {
		case <-sub.done:
			return
		default:
		}
		close(sub.done)

		subs := h.subs[taskID]
		for i, s := range subs {
			if s == sub {
				h.subs[taskID] = append(subs[:i], subs[i+1:]...)
				h.total--
				if sub.userID != 0 {
					nextCount := h.userConnCounts[sub.userID] - 1
					if nextCount <= 0 {
						delete(h.userConnCounts, sub.userID)
					} else {
						h.userConnCounts[sub.userID] = nextCount
					}
				}
				break
			}
		}
		if len(h.subs[taskID]) == 0 {
			delete(h.subs, taskID)
		}
	}

	return sub.ch, unsubscribe, true
}

// Publish sends an event to all subscribers of the given taskID.
// It is non-blocking; for slow consumers older buffered events may be dropped
// to keep delivery moving and preserve the latest status when possible.
func (h *EventHub) Publish(taskID string, event TaskEvent) {
	h.publish(taskID, 0, event)
}

// PublishForUser publishes to task-specific subscribers and the owner's user stream.
func (h *EventHub) PublishForUser(userID int64, taskID string, event TaskEvent) {
	h.publish(taskID, userID, event)
}

func (h *EventHub) publish(taskID string, userID int64, event TaskEvent) {
	h.mu.RLock()
	taskSubs := append([]*taskSubscription(nil), h.subs[taskID]...)
	userSubs := append([]*userSubscription(nil), h.userSubs[userID]...)
	h.mu.RUnlock()

	for _, sub := range taskSubs {
		publishTaskEvent(sub.done, sub.ch, event)
	}
	for _, sub := range userSubs {
		publishTaskEvent(sub.done, sub.ch, event)
	}
}

func publishTaskEvent(done <-chan struct{}, ch chan TaskEvent, event TaskEvent) {
	select {
	case <-done:
	default:
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}
