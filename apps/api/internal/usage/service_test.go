package usage

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestServiceGetCurrentUsesShanghaiCaptureDaysAndLogicalTasks(t *testing.T) {
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, time.July, 13, 8, 0, 0, 0, location)
	service := NewService(
		captureUsageReaderStub{
			totalCredits:     130,
			peakDailyCredits: 100,
			firstCaptureDay:  time.Date(2026, time.July, 11, 0, 0, 0, 0, location),
			dailyCredits: map[string]int64{
				"2026-06-14": 20,
				"2026-07-13": 110,
			},
		},
		&taskUsageReaderStub{taskCount: 4, activeDays: 2},
		func() time.Time { return now },
	)

	result, err := service.GetCurrent(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetCurrent() error = %v", err)
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
	if len(result.Trend) != 30 {
		t.Fatalf("trend buckets = %d, want 30", len(result.Trend))
	}
	if result.Trend[0] != (DailyUsage{Date: "2026-06-14", Credits: 20}) {
		t.Fatalf("first trend bucket = %+v", result.Trend[0])
	}
	if result.Trend[1] != (DailyUsage{Date: "2026-06-15", Credits: 0}) {
		t.Fatalf("missing day was not zero-filled: %+v", result.Trend[1])
	}
	if result.Trend[29] != (DailyUsage{Date: "2026-07-13", Credits: 110}) {
		t.Fatalf("today trend bucket = %+v", result.Trend[29])
	}
}

func TestServiceGetCurrentRejectsUnsafeCaptureBeforeReadingTasks(t *testing.T) {
	tasks := &taskUsageReaderStub{}
	service := NewService(
		captureUsageReaderStub{totalCredits: MaxSafeInteger + 1},
		tasks,
		time.Now,
	)

	_, err := service.GetCurrent(context.Background(), 42)
	if !errors.Is(err, ErrUnsafeNumber) {
		t.Fatalf("GetCurrent() error = %v, want ErrUnsafeNumber", err)
	}
	if tasks.calls != 0 {
		t.Fatalf("task reader calls = %d, want 0 before unsafe numerator rejection", tasks.calls)
	}
}

func TestValidateUsageRejectsEveryUnsafeExportedNumber(t *testing.T) {
	tests := map[string]Usage{
		"total":   {Lifetime: LifetimeSummary{TotalCredits: MaxSafeInteger + 1}},
		"peak":    {Lifetime: LifetimeSummary{PeakDailyCredits: MaxSafeInteger + 1}},
		"tasks":   {Lifetime: LifetimeSummary{TaskCount: MaxSafeInteger + 1}},
		"days":    {Lifetime: LifetimeSummary{ActiveDays: MaxSafeInteger + 1}},
		"average": {Lifetime: LifetimeSummary{AverageDailyCredits: math.Inf(1)}},
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateUsage(value); !errors.Is(err, ErrUnsafeNumber) {
				t.Fatalf("ValidateUsage() error = %v, want ErrUnsafeNumber", err)
			}
		})
	}
}

func TestServiceGetCurrentRejectsUnsafeDailyCapture(t *testing.T) {
	service := NewService(
		captureUsageReaderStub{dailyCredits: map[string]int64{"2026-07-13": MaxSafeInteger + 1}},
		&taskUsageReaderStub{},
		func() time.Time { return time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC) },
	)

	_, err := service.GetCurrent(context.Background(), 42)
	if !errors.Is(err, ErrUnsafeNumber) {
		t.Fatalf("GetCurrent() error = %v, want ErrUnsafeNumber", err)
	}
}

func TestServiceGetCurrentAcceptsMaximumSafeInteger(t *testing.T) {
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(
		captureUsageReaderStub{
			totalCredits:     MaxSafeInteger,
			peakDailyCredits: MaxSafeInteger,
			firstCaptureDay:  time.Date(2026, time.July, 13, 0, 0, 0, 0, location),
		},
		&taskUsageReaderStub{taskCount: MaxSafeInteger, activeDays: MaxSafeInteger},
		func() time.Time { return time.Date(2026, time.July, 13, 8, 0, 0, 0, location) },
	)
	result, err := service.GetCurrent(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetCurrent() error = %v", err)
	}
	if result.Lifetime.TotalCredits != MaxSafeInteger || result.Lifetime.AverageDailyCredits != float64(MaxSafeInteger) {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceGetCurrentReturnsZeroesWithoutCapturesOrTasks(t *testing.T) {
	service := NewService(
		captureUsageReaderStub{},
		&taskUsageReaderStub{},
		func() time.Time { return time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC) },
	)

	result, err := service.GetCurrent(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetCurrent() error = %v", err)
	}
	if result.Lifetime != (LifetimeSummary{}) {
		t.Fatalf("lifetime = %+v, want all zeroes", result.Lifetime)
	}
	if len(result.Trend) != 30 || result.Trend[0].Credits != 0 || result.Trend[29].Credits != 0 {
		t.Fatalf("trend = %+v, want 30 zero buckets", result.Trend)
	}
}

type captureUsageReaderStub struct {
	totalCredits     int64
	peakDailyCredits int64
	firstCaptureDay  time.Time
	dailyCredits     map[string]int64
	err              error
}

func (s captureUsageReaderStub) GetCaptureUsage(context.Context, int64, *time.Location) (int64, int64, time.Time, error) {
	return s.totalCredits, s.peakDailyCredits, s.firstCaptureDay, s.err
}

func (s captureUsageReaderStub) GetDailyCaptureUsage(context.Context, int64, *time.Location, time.Time, time.Time) (map[string]int64, error) {
	return s.dailyCredits, s.err
}

type taskUsageReaderStub struct {
	taskCount  int64
	activeDays int64
	err        error
	calls      int
}

func (s *taskUsageReaderStub) GetTaskUsage(context.Context, int64, *time.Location) (int64, int64, error) {
	s.calls++
	return s.taskCount, s.activeDays, s.err
}
