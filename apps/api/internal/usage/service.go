// Package usage owns the user-facing read model that composes Tasking and
// Credits facts without owning either context's persistence or write paths.
package usage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

const ShanghaiTimezone = "Asia/Shanghai"
const MaxSafeInteger int64 = 9_007_199_254_740_991
const DefaultTrendDays = 30

var ErrUnsafeNumber = errors.New("usage value exceeds JavaScript safe integer range")

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
}

func NewService(
	captures CaptureUsageReader,
	tasks LogicalTaskUsageReader,
	now func() time.Time,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{captures: captures, tasks: tasks, now: now}
}

func (s *Service) GetCurrent(ctx context.Context, userID int64) (Usage, error) {
	if s == nil || s.captures == nil || s.tasks == nil {
		return Usage{}, fmt.Errorf("usage service: readers not configured")
	}
	if userID <= 0 {
		return Usage{}, fmt.Errorf("usage service: invalid user id")
	}
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		return Usage{}, fmt.Errorf("usage service: load timezone: %w", err)
	}
	totalCredits, peakDailyCredits, firstCaptureDay, err := s.captures.GetCaptureUsage(ctx, userID, location)
	if err != nil {
		return Usage{}, fmt.Errorf("usage service: read settled captures: %w", err)
	}
	if err := validateSafeInteger("total credits", totalCredits); err != nil {
		return Usage{}, err
	}
	if err := validateSafeInteger("peak daily credits", peakDailyCredits); err != nil {
		return Usage{}, err
	}
	taskCount, activeDays, err := s.tasks.GetTaskUsage(ctx, userID, location)
	if err != nil {
		return Usage{}, fmt.Errorf("usage service: read logical tasks: %w", err)
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

	trendStart := today.AddDate(0, 0, -(DefaultTrendDays - 1))
	dailyCredits, err := s.captures.GetDailyCaptureUsage(
		ctx,
		userID,
		location,
		trendStart,
		today.AddDate(0, 0, 1),
	)
	if err != nil {
		return Usage{}, fmt.Errorf("usage service: read daily settled captures: %w", err)
	}
	result.Trend = make([]DailyUsage, 0, DefaultTrendDays)
	for dayIndex := 0; dayIndex < DefaultTrendDays; dayIndex++ {
		date := trendStart.AddDate(0, 0, dayIndex).Format(time.DateOnly)
		credits := dailyCredits[date]
		if err := validateSafeInteger("daily credits", credits); err != nil {
			return Usage{}, err
		}
		result.Trend = append(result.Trend, DailyUsage{Date: date, Credits: credits})
	}
	if err := ValidateUsage(result); err != nil {
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

func ValidateUsage(value Usage) error {
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
		return fmt.Errorf("%w: average daily credits", ErrUnsafeNumber)
	}
	for _, day := range value.Trend {
		if err := validateSafeInteger("daily credits", day.Credits); err != nil {
			return err
		}
	}
	if value.Timezone != ShanghaiTimezone {
		return fmt.Errorf("usage response: unsupported timezone %q", value.Timezone)
	}
	if len(value.Trend) != DefaultTrendDays {
		return fmt.Errorf("usage response: trend bucket count = %d, want %d", len(value.Trend), DefaultTrendDays)
	}
	return nil
}

func validateSafeInteger(name string, value int64) error {
	if value < 0 || value > MaxSafeInteger {
		return fmt.Errorf("%w: %s", ErrUnsafeNumber, name)
	}
	return nil
}
