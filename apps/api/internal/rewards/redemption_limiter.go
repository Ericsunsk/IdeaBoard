package rewards

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRedemptionAttemptLimit  = 10
	defaultRedemptionAttemptWindow = 10 * time.Minute
	maxTrackedRedemptionUsers      = 10_000
	redemptionLimiterCleanupEvery  = time.Minute
)

type redemptionAttemptLimiter struct {
	mu          sync.Mutex
	entries     map[int64]*redemptionAttemptEntry
	order       *list.List
	now         func() time.Time
	lastCleanup time.Time
}

type redemptionAttemptEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
	element  *list.Element
}

func newRedemptionAttemptLimiter(now func() time.Time) *redemptionAttemptLimiter {
	if now == nil {
		now = time.Now
	}
	return &redemptionAttemptLimiter{
		entries: make(map[int64]*redemptionAttemptEntry),
		order:   list.New(),
		now:     now,
	}
}

func (l *redemptionAttemptLimiter) Allow(userID int64) bool {
	if l == nil || userID <= 0 {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.cleanupIdleUsers(now)
	entry, ok := l.entries[userID]
	if !ok {
		if len(l.entries) >= maxTrackedRedemptionUsers {
			l.removeOldestUser()
		}
		entry = &redemptionAttemptEntry{
			limiter: rate.NewLimiter(
				rate.Every(defaultRedemptionAttemptWindow/defaultRedemptionAttemptLimit),
				defaultRedemptionAttemptLimit,
			),
		}
		entry.element = l.order.PushBack(userID)
		l.entries[userID] = entry
	} else {
		l.order.MoveToBack(entry.element)
	}
	entry.lastSeen = now
	return entry.limiter.AllowN(now, 1)
}

func (l *redemptionAttemptLimiter) cleanupIdleUsers(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Before(l.lastCleanup.Add(redemptionLimiterCleanupEvery)) {
		return
	}
	l.lastCleanup = now
	for {
		oldest := l.order.Front()
		if oldest == nil {
			return
		}
		entry := l.entries[oldest.Value.(int64)]
		if now.Before(entry.lastSeen.Add(defaultRedemptionAttemptWindow)) {
			return
		}
		l.removeOldestUser()
	}
}

func (l *redemptionAttemptLimiter) removeOldestUser() {
	oldest := l.order.Front()
	if oldest == nil {
		return
	}
	delete(l.entries, oldest.Value.(int64))
	l.order.Remove(oldest)
}
