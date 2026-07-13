package usage

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskstore"
)

func TestServiceGetLifetimeUsesShanghaiCaptureDaysAndLogicalTasks(t *testing.T) {
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, time.July, 13, 8, 0, 0, 0, location)
	service := NewService(
		captureUsageReaderStub{summary: credits.CaptureUsageSummary{
			TotalCredits:     130,
			PeakDailyCredits: 100,
			FirstCaptureDay:  time.Date(2026, time.July, 11, 0, 0, 0, 0, location),
		}},
		taskUsageReaderStub{summary: taskstore.UsageSummary{TaskCount: 4, ActiveDays: 2}},
		func() time.Time { return now },
	)

	result, err := service.GetLifetime(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetLifetime() error = %v", err)
	}
	if result.Timezone != ShanghaiTimezone {
		t.Fatalf("timezone = %q, want %q", result.Timezone, ShanghaiTimezone)
	}
	if result.Lifetime.TotalCredits != 130 || result.Lifetime.PeakDailyCredits != 100 {
		t.Fatalf("credit totals = %+v", result.Lifetime)
	}
	if result.Lifetime.AverageDailyCredits != 43.3 {
		t.Fatalf("average daily credits = %v, want 43.3", result.Lifetime.AverageDailyCredits)
	}
	if result.Lifetime.TaskCount != 4 || result.Lifetime.ActiveDays != 2 {
		t.Fatalf("task totals = %+v", result.Lifetime)
	}
}

func TestServiceGetLifetimeReturnsZeroesWithoutCapturesOrTasks(t *testing.T) {
	service := NewService(
		captureUsageReaderStub{},
		taskUsageReaderStub{},
		func() time.Time { return time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC) },
	)

	result, err := service.GetLifetime(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetLifetime() error = %v", err)
	}
	if result.Lifetime != (LifetimeSummary{}) {
		t.Fatalf("lifetime = %+v, want all zeroes", result.Lifetime)
	}
}

type captureUsageReaderStub struct {
	summary credits.CaptureUsageSummary
	err     error
}

func (s captureUsageReaderStub) GetCaptureUsage(context.Context, int64, string) (credits.CaptureUsageSummary, error) {
	return s.summary, s.err
}

type taskUsageReaderStub struct {
	summary taskstore.UsageSummary
	err     error
}

func (s taskUsageReaderStub) GetUsageSummary(context.Context, int64, string) (taskstore.UsageSummary, error) {
	return s.summary, s.err
}
