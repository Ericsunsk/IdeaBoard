// Package application owns the user-facing read model that composes Tasking and
// Credits facts without owning either context's persistence or write paths.
package application

import (
	"context"
	"fmt"
	"math"
	"time"

	billingrecordsports "ideaboard/api/internal/billingrecords/ports"
	usagedomain "ideaboard/api/internal/usage/domain"
	usageports "ideaboard/api/internal/usage/ports"

	"golang.org/x/sync/singleflight"
)

const ShanghaiTimezone = "Asia/Shanghai"

// MaxSafeInteger is the JavaScript safe-integer guard shared with
// billing-records: billingrecords/ports owns the value, this name only
// re-exports it so existing callers keep compiling.
const MaxSafeInteger int64 = billingrecordsports.MaxSafeInteger

// Shared usage loads must outlive an individual HTTP waiter, but still need
// a hard upper bound so a stalled database cannot leave singleflight work
// running forever after every caller has disconnected.
const sharedUsageLoadTimeout = 30 * time.Second

type CaptureUsageReader interface {
	GetCaptureUsage(context.Context, int64, *time.Location) (int64, int64, time.Time, error)
	GetDailyCaptureUsage(context.Context, int64, *time.Location, time.Time, time.Time) (map[string]int64, error)
}

type LogicalTaskUsageReader interface {
	GetTaskUsage(context.Context, int64, *time.Location) (int64, int64, error)
}

type LifetimeSummary struct {
	TotalCredits        int64   `json:"total_credits"`
	PeakDailyCredits    int64   `json:"peak_daily_credits"`
	AverageDailyCredits float64 `json:"average_daily_credits"`
	TaskCount           int64   `json:"task_count"`
	ActiveDays          int64   `json:"active_days"`
}

type DailyUsage struct {
	Date    string `json:"date"`
	Credits int64  `json:"credits"`
}

type Usage struct {
	Timezone string          `json:"timezone"`
	Lifetime LifetimeSummary `json:"lifetime"`
	Trend    []DailyUsage    `json:"trend"`
}

type Service struct {
	captures CaptureUsageReader
	tasks    LogicalTaskUsageReader
	now      func() time.Time
	cache    *usageCache
	// inflight collapses concurrent loads for the same key (e.g. every
	// dashboard client hitting a just-expired TTL) into one DB read.
	// Callers must keep Service behind its pointer: Group must not be copied.
	inflight singleflight.Group
}

type ServiceOptions struct {
	Now           func() time.Time
	CacheTTL      time.Duration
	CacheCapacity int
}

func NewService(
	captures CaptureUsageReader,
	tasks LogicalTaskUsageReader,
	options ServiceOptions,
) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		captures: captures,
		tasks:    tasks,
		now:      now,
		cache:    newUsageCache(options.CacheCapacity, options.CacheTTL, now),
	}
}

func (s *Service) GetCurrent(ctx context.Context, userID int64, usageRange usagedomain.Range) (Usage, error) {
	if s == nil || s.captures == nil || s.tasks == nil {
		return Usage{}, usageports.ErrReadersNotConfigured
	}
	if userID <= 0 {
		return Usage{}, usageports.ErrInvalidUserID
	}
	if !usageRange.Valid() {
		return Usage{}, fmt.Errorf("usage service: %w: %q", usagedomain.ErrInvalidRange, usageRange)
	}
	key := cacheKey{userID: userID, usageRange: usageRange}
	if cached, ok := s.cache.Get(key); ok {
		return cached, nil
	}
	loaded, err := s.loadSingleflight(ctx, key, func(sharedCtx context.Context) (Usage, error) {
		return s.loadCurrent(sharedCtx, userID, usageRange)
	})
	if err != nil {
		return Usage{}, err
	}
	return loaded, nil
}

// loadSingleflight shares one loadCurrent call across concurrent GetCurrent
// calls for the same key and rechecks the cache inside, so a waiter that
// arrives after a fresh Set never triggers a redundant load.
func (s *Service) loadSingleflight(
	ctx context.Context,
	key cacheKey,
	load func(context.Context) (Usage, error),
) (Usage, error) {
	waiter := s.inflight.DoChan(key.String(), func() (any, error) {
		if cached, ok := s.cache.Get(key); ok {
			return cached, nil
		}
		sharedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sharedUsageLoadTimeout)
		defer cancel()
		result, err := load(sharedCtx)
		if err != nil {
			return Usage{}, err
		}
		s.cache.Set(key, result)
		return result, nil
	})
	select {
	case <-ctx.Done():
		return Usage{}, ctx.Err()
	case res := <-waiter:
		if res.Err != nil {
			return Usage{}, res.Err
		}
		result, ok := res.Val.(Usage)
		if !ok {
			return Usage{}, usageports.ErrCacheValueType
		}
		return result, nil
	}
}

func (s *Service) loadCurrent(ctx context.Context, userID int64, usageRange usagedomain.Range) (Usage, error) {
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		return Usage{}, fmt.Errorf("%w: %w", usageports.ErrLoadTimezone, err)
	}
	totalCredits, peakDailyCredits, firstCaptureDay, err := s.captures.GetCaptureUsage(ctx, userID, location)
	if err != nil {
		return Usage{}, fmt.Errorf("%w: %w", usageports.ErrReadSettledCaptures, err)
	}
	if err := validateSafeInteger("total credits", totalCredits); err != nil {
		return Usage{}, err
	}
	if err := validateSafeInteger("peak daily credits", peakDailyCredits); err != nil {
		return Usage{}, err
	}
	taskCount, activeDays, err := s.tasks.GetTaskUsage(ctx, userID, location)
	if err != nil {
		return Usage{}, fmt.Errorf("%w: %w", usageports.ErrReadLogicalTasks, err)
	}
	if err := validateSafeInteger("task count", taskCount); err != nil {
		return Usage{}, err
	}
	if err := validateSafeInteger("active days", activeDays); err != nil {
		return Usage{}, err
	}

	today := dateInLocation(s.now(), location)
	result := Usage{
		Timezone: ShanghaiTimezone,
		Lifetime: LifetimeSummary{
			TotalCredits:     totalCredits,
			PeakDailyCredits: peakDailyCredits,
			TaskCount:        taskCount,
			ActiveDays:       activeDays,
		},
	}
	if !firstCaptureDay.IsZero() {
		firstDay := dateInLocation(firstCaptureDay, location)
		days := int64(today.Sub(firstDay).Hours()/24) + 1
		if days < 1 {
			days = 1
		}
		result.Lifetime.AverageDailyCredits = averageCredits(totalCredits, days)
	}

	trendDays := usageRange.Days()
	trendStart := today.AddDate(0, 0, -(trendDays - 1))
	dailyCredits, err := s.captures.GetDailyCaptureUsage(
		ctx,
		userID,
		location,
		trendStart,
		today.AddDate(0, 0, 1),
	)
	if err != nil {
		return Usage{}, fmt.Errorf("%w: %w", usageports.ErrReadDailyCaptures, err)
	}
	result.Trend = make([]DailyUsage, 0, trendDays)
	for dayIndex := 0; dayIndex < trendDays; dayIndex++ {
		date := trendStart.AddDate(0, 0, dayIndex).Format(time.DateOnly)
		credits := dailyCredits[date]
		if err := validateSafeInteger("daily credits", credits); err != nil {
			return Usage{}, err
		}
		result.Trend = append(result.Trend, DailyUsage{Date: date, Credits: credits})
	}
	if err := ValidateUsage(result, usageRange); err != nil {
		return Usage{}, err
	}
	return result, nil
}

func dateInLocation(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func averageCredits(totalCredits, days int64) float64 {
	whole := totalCredits / days
	remainder := totalCredits % days
	tenths := (remainder*10 + days/2) / days
	if tenths == 10 {
		whole++
		tenths = 0
	}
	return float64(whole) + float64(tenths)/10
}

func ValidateUsage(value Usage, usageRange usagedomain.Range) error {
	if !usageRange.Valid() {
		return fmt.Errorf("usage response: %w: %q", usagedomain.ErrInvalidRange, usageRange)
	}
	for name, integer := range map[string]int64{
		"total credits":      value.Lifetime.TotalCredits,
		"peak daily credits": value.Lifetime.PeakDailyCredits,
		"task count":         value.Lifetime.TaskCount,
		"active days":        value.Lifetime.ActiveDays,
	} {
		if err := validateSafeInteger(name, integer); err != nil {
			return err
		}
	}
	average := value.Lifetime.AverageDailyCredits
	if math.IsNaN(average) || math.IsInf(average, 0) || average < 0 || average > float64(MaxSafeInteger) {
		return fmt.Errorf("%w: average daily credits", usageports.ErrUnsafeNumber)
	}
	for _, day := range value.Trend {
		if err := validateSafeInteger("daily credits", day.Credits); err != nil {
			return err
		}
	}
	if value.Timezone != ShanghaiTimezone {
		return fmt.Errorf("%w: %q", usageports.ErrUnsupportedTimezone, value.Timezone)
	}
	if len(value.Trend) != usageRange.Days() {
		return fmt.Errorf("%w: got %d, want %d", usageports.ErrTrendBucketCount, len(value.Trend), usageRange.Days())
	}
	return nil
}

func validateSafeInteger(name string, value int64) error {
	if value < 0 || value > MaxSafeInteger {
		return fmt.Errorf("%w: %s", usageports.ErrUnsafeNumber, name)
	}
	return nil
}
