// Package usage owns the user-facing read model that composes Tasking and
// Credits facts without owning either context's persistence or write paths.
package usage

import (
	"context"
	"fmt"
	"math"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskstore"
)

const ShanghaiTimezone = "Asia/Shanghai"

type LifetimeSummary struct {
	TotalCredits        int64   `json:"total_credits"`
	PeakDailyCredits    int64   `json:"peak_daily_credits"`
	AverageDailyCredits float64 `json:"average_daily_credits"`
	TaskCount           int64   `json:"task_count"`
	ActiveDays          int64   `json:"active_days"`
}

type LifetimeUsage struct {
	Timezone string          `json:"timezone"`
	Lifetime LifetimeSummary `json:"lifetime"`
}

type Service struct {
	captures credits.CaptureUsageReader
	tasks    taskstore.UsageReader
	now      func() time.Time
}

func NewService(
	captures credits.CaptureUsageReader,
	tasks taskstore.UsageReader,
	now func() time.Time,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{captures: captures, tasks: tasks, now: now}
}

func (s *Service) GetLifetime(ctx context.Context, userID int64) (LifetimeUsage, error) {
	if s == nil || s.captures == nil || s.tasks == nil {
		return LifetimeUsage{}, fmt.Errorf("usage service: readers not configured")
	}
	if userID <= 0 {
		return LifetimeUsage{}, fmt.Errorf("usage service: invalid user id")
	}
	captures, err := s.captures.GetCaptureUsage(ctx, userID, ShanghaiTimezone)
	if err != nil {
		return LifetimeUsage{}, fmt.Errorf("usage service: read settled captures: %w", err)
	}
	tasks, err := s.tasks.GetUsageSummary(ctx, userID, ShanghaiTimezone)
	if err != nil {
		return LifetimeUsage{}, fmt.Errorf("usage service: read logical tasks: %w", err)
	}

	result := LifetimeUsage{
		Timezone: ShanghaiTimezone,
		Lifetime: LifetimeSummary{
			TotalCredits:     captures.TotalCredits,
			PeakDailyCredits: captures.PeakDailyCredits,
			TaskCount:        tasks.TaskCount,
			ActiveDays:       tasks.ActiveDays,
		},
	}
	if captures.FirstCaptureDay.IsZero() {
		return result, nil
	}
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		return LifetimeUsage{}, fmt.Errorf("usage service: load timezone: %w", err)
	}
	firstDay := dateInLocation(captures.FirstCaptureDay, location)
	today := dateInLocation(s.now(), location)
	days := int(today.Sub(firstDay).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	result.Lifetime.AverageDailyCredits = roundOneDecimal(float64(captures.TotalCredits) / float64(days))
	return result, nil
}

func dateInLocation(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func roundOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}
