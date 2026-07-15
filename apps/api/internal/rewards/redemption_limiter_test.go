package rewards

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedemptionAttemptLimiterEnforcesDefaultPolicyPerUser(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRedemptionAttemptLimiter(func() time.Time { return now })

	for attempt := 1; attempt <= 10; attempt++ {
		if !limiter.Allow(41) {
			t.Fatalf("attempt %d rejected before the default burst was exhausted", attempt)
		}
	}
	if limiter.Allow(41) {
		t.Fatal("eleventh attempt was allowed without token refill")
	}
	if !limiter.Allow(42) {
		t.Fatal("one user's attempts depleted another user's tokens")
	}

	now = now.Add(time.Minute)
	if !limiter.Allow(41) {
		t.Fatal("one token was not refilled after one minute")
	}
	if limiter.Allow(41) {
		t.Fatal("more than one token was refilled after one minute")
	}
}

func TestRedemptionAttemptLimiterCleansIdleUsersOnTraffic(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRedemptionAttemptLimiter(func() time.Time { return now })

	if !limiter.Allow(41) {
		t.Fatal("initial attempt was rejected")
	}
	now = now.Add(defaultRedemptionAttemptWindow)
	if !limiter.Allow(42) {
		t.Fatal("attempt triggering idle cleanup was rejected")
	}
	if got := limiter.trackedUserCount(); got != 1 {
		t.Fatalf("tracked users after idle cleanup = %d, want 1", got)
	}
}

func TestRedemptionAttemptLimiterSerializesConcurrentAttempts(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRedemptionAttemptLimiter(func() time.Time { return now })
	var accepted atomic.Int64
	var workers sync.WaitGroup

	for range 100 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if limiter.Allow(41) {
				accepted.Add(1)
			}
		}()
	}
	workers.Wait()

	if got := accepted.Load(); got != 10 {
		t.Fatalf("accepted concurrent attempts = %d, want 10", got)
	}
}

func TestRedemptionAttemptLimiterBoundsTrackedUsers(t *testing.T) {
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRedemptionAttemptLimiter(func() time.Time { return now })

	for userID := int64(1); userID <= maxTrackedRedemptionUsers+1; userID++ {
		if !limiter.Allow(userID) {
			t.Fatalf("initial attempt for user %d was rejected", userID)
		}
	}
	if got := limiter.trackedUserCount(); got != maxTrackedRedemptionUsers {
		t.Fatalf("tracked users = %d, want %d", got, maxTrackedRedemptionUsers)
	}
	for attempt := 1; attempt <= defaultRedemptionAttemptLimit; attempt++ {
		if !limiter.Allow(1) {
			t.Fatalf("evicted user's fresh attempt %d was rejected", attempt)
		}
	}
	if limiter.Allow(1) {
		t.Fatal("evicted user's fresh bucket exceeded the default burst")
	}
}
