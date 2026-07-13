package usage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestServiceCacheExpiresEntriesAfterTTL(t *testing.T) {
	clock := newUsageTestClock()
	captures := newCountingCaptureReader()
	service := NewService(captures, &taskUsageReaderStub{}, ServiceOptions{
		Now:           clock.Now,
		CacheTTL:      30 * time.Second,
		CacheCapacity: 2,
	})

	getUsageForTest(t, service, 41, Range7Days)
	getUsageForTest(t, service, 41, Range7Days)
	if calls := captures.dailyCallCount(41, Range7Days); calls != 1 {
		t.Fatalf("fresh cache daily reads = %d, want 1", calls)
	}

	clock.Advance(29 * time.Second)
	getUsageForTest(t, service, 41, Range7Days)
	if calls := captures.dailyCallCount(41, Range7Days); calls != 1 {
		t.Fatalf("pre-expiry daily reads = %d, want 1", calls)
	}

	clock.Advance(2 * time.Second)
	getUsageForTest(t, service, 41, Range7Days)
	if calls := captures.dailyCallCount(41, Range7Days); calls != 2 {
		t.Fatalf("expired cache daily reads = %d, want 2", calls)
	}
}

func TestServiceCacheUsesLRUAndIsolatesUsersAndRanges(t *testing.T) {
	clock := newUsageTestClock()
	captures := newCountingCaptureReader()
	service := NewService(captures, &taskUsageReaderStub{}, ServiceOptions{
		Now:           clock.Now,
		CacheTTL:      time.Minute,
		CacheCapacity: 2,
	})

	sevenDayUser41 := getUsageForTest(t, service, 41, Range7Days)
	thirtyDayUser41 := getUsageForTest(t, service, 41, Range30Days)
	if sevenDayUser41.Trend[6].Credits == thirtyDayUser41.Trend[29].Credits {
		t.Fatal("range-specific fixtures unexpectedly match")
	}
	getUsageForTest(t, service, 41, Range7Days) // Touch 7d so 30d becomes least-recently used.
	sevenDayUser42 := getUsageForTest(t, service, 42, Range7Days)
	if sevenDayUser42.Trend[6].Credits == sevenDayUser41.Trend[6].Credits {
		t.Fatal("user-specific fixtures unexpectedly match")
	}
	getUsageForTest(t, service, 41, Range30Days)

	if calls := captures.dailyCallCount(41, Range7Days); calls != 1 {
		t.Fatalf("recent 7d entry daily reads = %d, want 1", calls)
	}
	if calls := captures.dailyCallCount(42, Range7Days); calls != 1 {
		t.Fatalf("isolated user daily reads = %d, want 1", calls)
	}
	if calls := captures.dailyCallCount(41, Range30Days); calls != 2 {
		t.Fatalf("evicted 30d entry daily reads = %d, want 2", calls)
	}
}

func TestServiceCacheReturnsIndependentTrendCopies(t *testing.T) {
	clock := newUsageTestClock()
	captures := newCountingCaptureReader()
	service := NewService(captures, &taskUsageReaderStub{}, ServiceOptions{
		Now:           clock.Now,
		CacheTTL:      time.Minute,
		CacheCapacity: 2,
	})

	first := getUsageForTest(t, service, 41, Range7Days)
	wantLast := first.Trend[6].Credits
	first.Trend[0].Credits = 999
	first.Trend[6].Credits = 999

	second := getUsageForTest(t, service, 41, Range7Days)
	if second.Trend[0].Credits != 0 || second.Trend[6].Credits != wantLast {
		t.Fatalf("cached trend was mutated through first response: %+v", second.Trend)
	}
	second.Trend[6].Credits = 777
	third := getUsageForTest(t, service, 41, Range7Days)
	if third.Trend[6].Credits != wantLast {
		t.Fatalf("cached trend was mutated through cache-hit response: got %d want %d", third.Trend[6].Credits, wantLast)
	}
}

func TestServiceCacheSupportsConcurrentReadersWithoutSharingTrendSlices(t *testing.T) {
	clock := newUsageTestClock()
	captures := newCountingCaptureReader()
	service := NewService(captures, &taskUsageReaderStub{}, ServiceOptions{
		Now:           clock.Now,
		CacheTTL:      time.Minute,
		CacheCapacity: 2,
	})
	want := getUsageForTest(t, service, 41, Range1Year).Trend[364].Credits

	const readers = 64
	var wait sync.WaitGroup
	errorsCh := make(chan error, readers)
	for index := 0; index < readers; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.GetCurrent(context.Background(), 41, Range1Year)
			if err != nil {
				errorsCh <- err
				return
			}
			result.Trend[index%len(result.Trend)].Credits = int64(index)
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent GetCurrent() error = %v", err)
		}
	}

	result := getUsageForTest(t, service, 41, Range1Year)
	if result.Trend[364].Credits != want {
		t.Fatalf("shared trend slice changed cached value: got %d want %d", result.Trend[364].Credits, want)
	}
	if calls := captures.dailyCallCount(41, Range1Year); calls != 1 {
		t.Fatalf("warm concurrent cache daily reads = %d, want 1", calls)
	}
}

func getUsageForTest(t *testing.T, service *Service, userID int64, usageRange Range) Usage {
	t.Helper()
	result, err := service.GetCurrent(context.Background(), userID, usageRange)
	if err != nil {
		t.Fatalf("GetCurrent(%d, %s) error = %v", userID, usageRange, err)
	}
	return result
}

type usageTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newUsageTestClock() *usageTestClock {
	return &usageTestClock{now: time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)}
}

func (c *usageTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *usageTestClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

type countingCaptureReader struct {
	mu         sync.Mutex
	dailyCalls map[string]int
}

func newCountingCaptureReader() *countingCaptureReader {
	return &countingCaptureReader{dailyCalls: make(map[string]int)}
}

func (r *countingCaptureReader) GetCaptureUsage(context.Context, int64, *time.Location) (int64, int64, time.Time, error) {
	return 0, 0, time.Time{}, nil
}

func (r *countingCaptureReader) GetDailyCaptureUsage(
	_ context.Context,
	userID int64,
	_ *time.Location,
	start time.Time,
	endExclusive time.Time,
) (map[string]int64, error) {
	days := int(endExclusive.Sub(start).Hours() / 24)
	usageRange, err := rangeFromDays(days)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.dailyCalls[usageCacheTestKey(userID, usageRange)]++
	r.mu.Unlock()
	lastDate := endExclusive.AddDate(0, 0, -1).Format(time.DateOnly)
	return map[string]int64{lastDate: userID*1_000 + int64(days)}, nil
}

func (r *countingCaptureReader) dailyCallCount(userID int64, usageRange Range) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dailyCalls[usageCacheTestKey(userID, usageRange)]
}

func rangeFromDays(days int) (Range, error) {
	for _, usageRange := range []Range{Range7Days, Range30Days, Range1Year} {
		if usageRange.Days() == days {
			return usageRange, nil
		}
	}
	return "", fmt.Errorf("unexpected usage days %d", days)
}

func usageCacheTestKey(userID int64, usageRange Range) string {
	return fmt.Sprintf("%d:%s", userID, usageRange)
}
