package accounts

import (
	"sync"
	"time"
)

// TaskEvent is emitted when a task's status changes.
// It carries minimal data; consumers should call GetUserTask for the full safe view.
type TaskEvent struct {
	TaskID string
	Status string
}

// taskSubscription represents a single subscriber to a task's events.
type taskSubscription struct {
	ch     chan TaskEvent
	done   chan struct{}
	userID uint64
}

// TaskEventHub manages per-task subscriptions for SSE streaming.
// It is safe for concurrent use.
type TaskEventHub struct {
	mu             sync.RWMutex
	subs           map[string][]*taskSubscription // taskID -> subscribers
	globalLimit    int
	perUserLimit   int
	total          int64
	userConnCounts map[uint64]int
}

const (
	defaultSSEGlobalLimit   = 1000
	defaultSSEPerUserLimit  = 10
	defaultSSEMaxConnPeriod = 30 * time.Minute
)

// NewTaskEventHub creates a new hub.
func NewTaskEventHub() *TaskEventHub {
	return NewTaskEventHubWithLimits(defaultSSEGlobalLimit, defaultSSEPerUserLimit)
}

func NewTaskEventHubWithLimits(globalLimit, perUserLimit int) *TaskEventHub {
	if globalLimit <= 0 {
		globalLimit = defaultSSEGlobalLimit
	}
	if perUserLimit <= 0 {
		perUserLimit = defaultSSEPerUserLimit
	}
	return &TaskEventHub{
		subs:           make(map[string][]*taskSubscription),
		globalLimit:    globalLimit,
		perUserLimit:   perUserLimit,
		userConnCounts: make(map[uint64]int),
	}
}

// Subscribe returns a channel that receives TaskEvents for the given taskID,
// and an unsubscribe function. Unsubscribe removes the subscriber from the hub
// but does not close the channel.
func (h *TaskEventHub) Subscribe(taskID string, userID uint64) (<-chan TaskEvent, func(), bool) {
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
			return // already unsubscribed
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
func (h *TaskEventHub) Publish(taskID string, event TaskEvent) {
	h.mu.RLock()
	subs := h.subs[taskID]
	// Copy slice to avoid holding lock during sends
	copied := make([]*taskSubscription, len(subs))
	copy(copied, subs)
	h.mu.RUnlock()

	for _, sub := range copied {
		select {
		case <-sub.done:
			// subscriber already cancelled
		default:
			select {
			case sub.ch <- event:
			default:
				select {
				case <-sub.ch:
				default:
				}
				select {
				case sub.ch <- event:
				default:
				}
			}
		}
	}
}

// SubscriberCount returns the number of active subscribers for a taskID.
// Useful for testing and diagnostics.
func (h *TaskEventHub) SubscriberCount(taskID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[taskID])
}

func (h *TaskEventHub) TotalSubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return int(h.total)
}
