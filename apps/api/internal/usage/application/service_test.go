package application

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/billing"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	"ideaboard/api/internal/testdb"
	usagedomain "ideaboard/api/internal/usage/domain"
	usageports "ideaboard/api/internal/usage/ports"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
		ServiceOptions{Now: func() time.Time { return now }},
	)

	result, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, usagedomain.Range30Days)
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

func TestServiceGetCurrentBuildsEverySupportedShanghaiRangeIncludingToday(t *testing.T) {
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 13, 23, 59, 0, 0, location)
	tests := []struct {
		usageRange usagedomain.Range
		wantDays   int
		wantFirst  string
	}{
		{usageRange: usagedomain.Range7Days, wantDays: 7, wantFirst: "2026-07-07"},
		{usageRange: usagedomain.Range30Days, wantDays: 30, wantFirst: "2026-06-14"},
		{usageRange: usagedomain.Range1Year, wantDays: 365, wantFirst: "2025-07-14"},
	}
	for _, tt := range tests {
		t.Run(string(tt.usageRange), func(t *testing.T) {
			service := NewService(
				captureUsageReaderStub{dailyCredits: map[string]int64{"2026-07-13": 9}},
				&taskUsageReaderStub{},
				ServiceOptions{Now: func() time.Time { return now }},
			)

			result, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, tt.usageRange)
			if err != nil {
				t.Fatalf("GetCurrent() error = %v", err)
			}
			if len(result.Trend) != tt.wantDays {
				t.Fatalf("trend buckets = %d, want %d", len(result.Trend), tt.wantDays)
			}
			if result.Trend[0].Date != tt.wantFirst || result.Trend[len(result.Trend)-1] != (DailyUsage{Date: "2026-07-13", Credits: 9}) {
				t.Fatalf("trend bounds = %s..%+v, want %s..2026-07-13", result.Trend[0].Date, result.Trend[len(result.Trend)-1], tt.wantFirst)
			}
		})
	}
}

func TestServiceGetCurrentRejectsUnsafeCaptureBeforeReadingTasks(t *testing.T) {
	tasks := &taskUsageReaderStub{}
	service := NewService(
		captureUsageReaderStub{totalCredits: MaxSafeInteger + 1},
		tasks,
		ServiceOptions{},
	)

	_, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, usagedomain.Range30Days)
	if !errors.Is(err, usageports.ErrUnsafeNumber) {
		t.Fatalf("GetCurrent() error = %v, want usageports.ErrUnsafeNumber", err)
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
			if err := ValidateUsage(value, usagedomain.Range30Days); !errors.Is(err, usageports.ErrUnsafeNumber) {
				t.Fatalf("ValidateUsage() error = %v, want usageports.ErrUnsafeNumber", err)
			}
		})
	}
}

func TestServiceGetCurrentRejectsUnsafeDailyCapture(t *testing.T) {
	service := NewService(
		captureUsageReaderStub{dailyCredits: map[string]int64{"2026-07-13": MaxSafeInteger + 1}},
		&taskUsageReaderStub{},
		ServiceOptions{Now: func() time.Time { return time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC) }},
	)

	_, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, usagedomain.Range30Days)
	if !errors.Is(err, usageports.ErrUnsafeNumber) {
		t.Fatalf("GetCurrent() error = %v, want usageports.ErrUnsafeNumber", err)
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
		ServiceOptions{Now: func() time.Time { return time.Date(2026, time.July, 13, 8, 0, 0, 0, location) }},
	)
	result, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, usagedomain.Range30Days)
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
		ServiceOptions{Now: func() time.Time { return time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC) }},
	)

	result, err := service.GetCurrent(context.Background(), 42, domain.UsageFilter{}, usagedomain.Range30Days)
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

func (s captureUsageReaderStub) GetCaptureUsage(context.Context, int64, domain.UsageFilter, *time.Location) (int64, int64, time.Time, error) {
	return s.totalCredits, s.peakDailyCredits, s.firstCaptureDay, s.err
}

func (s captureUsageReaderStub) GetDailyCaptureUsage(context.Context, int64, domain.UsageFilter, *time.Location, time.Time, time.Time) (map[string]int64, error) {
	return s.dailyCredits, s.err
}

type taskUsageReaderStub struct {
	taskCount  int64
	activeDays int64
	err        error
	calls      int
}

func (s *taskUsageReaderStub) GetTaskUsage(context.Context, int64, domain.UsageFilter, *time.Location) (int64, int64, error) {
	s.calls++
	return s.taskCount, s.activeDays, s.err
}
func TestServiceGetCurrentReportsDurableLogicalTasks(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	const userID int64 = 901
	seedUsageUser(t, ctx, pool, userID, "US0901")
	seedUsageUser(t, ctx, pool, 902, "US0902")

	creditService := credits.NewService(pool)
	firstCapture := captureUsageCredits(t, ctx, creditService, userID, 20, "usage-capture-1")
	latestCapture := captureUsageCredits(t, ctx, creditService, userID, 100, "usage-capture-2")
	_ = captureUsageCredits(t, ctx, creditService, 902, 10, "usage-other-user-capture")
	if _, err := creditService.Administrative().Refund(ctx, ports.RefundCommand{
		BillingAccountID:    usageAccountOf(t, ctx, creditService, userID),
		ChargeTransactionID: firstCapture.TransactionID,
		Amount:              mustUsageCreditAmount(t, 20),
		Metadata:            usageCreditMetadata("usage-refund"),
	}); err != nil {
		t.Fatalf("refund usage capture: %v", err)
	}

	taskIDs := []string{
		"usage-free-completed",
		"usage-free-failed",
		"usage-free-cancelled",
		"usage-free-pending",
	}
	if _, err := billingpostgres.NewStore(pool).PublishPrice(ctx, usagePriceInput("ai-image:generation", "Usage Free", 0)); err != nil {
		t.Fatalf("publish usage price: %v", err)
	}
	seedDurableUsageSnapshots(t, ctx, pool, userID, taskIDs)
	seedDurableUsageSnapshots(t, ctx, pool, 902, []string{"usage-other-user"})

	newUsageService := func() *Service {
		return NewService(creditService, taskbilling.NewUsageStore(pool), ServiceOptions{Now: func() time.Time {
			return latestCapture.CreatedAt
		}})
	}
	service := newUsageService()

	beforeDelete, err := service.GetCurrent(ctx, usageAccountOf(t, ctx, creditService, userID), domain.UsageFilter{}, usagedomain.Range30Days)
	if err != nil {
		t.Fatalf("GetCurrent() error = %v", err)
	}
	if beforeDelete.Lifetime.TotalCredits != 120 || beforeDelete.Lifetime.PeakDailyCredits != 120 {
		t.Fatalf("capture summary = %+v, want total=120 peak=120", beforeDelete.Lifetime)
	}
	if beforeDelete.Lifetime.AverageDailyCredits <= 0 {
		t.Fatalf("daily average = %v, want positive value", beforeDelete.Lifetime.AverageDailyCredits)
	}
	if beforeDelete.Lifetime.TaskCount != 4 || beforeDelete.Lifetime.ActiveDays != 1 {
		t.Fatalf("task summary = %+v, want count=4 active_days=1", beforeDelete.Lifetime)
	}
	if len(beforeDelete.Trend) != usagedomain.Range30Days.Days() {
		t.Fatalf("trend buckets = %d, want %d", len(beforeDelete.Trend), usagedomain.Range30Days.Days())
	}
	var trendTotal int64
	for _, day := range beforeDelete.Trend {
		trendTotal += day.Credits
	}
	if trendTotal != 120 {
		t.Fatalf("trend total = %d, want gross captures 120", trendTotal)
	}

}

func seedUsageUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID int64, storageUID string) {
	t.Helper()
	if _, err := db.Exec(ctx, `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)`, userID, fmt.Sprintf("usage-user-%d", userID), storageUID); err != nil {
		t.Fatalf("seed user %d: %v", userID, err)
	}
}

func usageAccountOf(t *testing.T, ctx context.Context, service *credits.Service, userID int64) int64 {
	t.Helper()
	accountID, err := service.ResolvePersonalAccount(ctx, userID)
	if err != nil {
		t.Fatalf("resolve personal account %d: %v", userID, err)
	}
	return accountID
}

func captureUsageCredits(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, key string) ports.CommandResult {
	t.Helper()
	accountID := usageAccountOf(t, ctx, service, userID)
	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: accountID,
		Amount:           mustUsageCreditAmount(t, amount),
		Metadata:         usageCreditMetadata("usage-grant-" + key),
	}); err != nil {
		t.Fatalf("grant user %d: %v", userID, err)
	}
	hold, err := service.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: accountID,
		Amount:           mustUsageCreditAmount(t, amount),
		Metadata:         usageCreditMetadata("usage-hold-" + key),
	})
	if err != nil {
		t.Fatalf("hold user %d: %v", userID, err)
	}
	capture, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            mustUsageCreditAmount(t, amount),
		Metadata:          usageCreditMetadata("usage-capture-" + key),
	})
	if err != nil {
		t.Fatalf("capture user %d: %v", userID, err)
	}
	return capture
}

func mustUsageCreditAmount(t *testing.T, value int64) domain.CreditAmount {
	t.Helper()
	amount, err := domain.NewCreditAmount(value)
	if err != nil {
		t.Fatalf("credit amount %d: %v", value, err)
	}
	return amount
}

func usageCreditMetadata(key string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey: key,
		Actor:          domain.Actor{Type: domain.ActorService, ID: "usage-test"},
		Source:         "usage_test",
		Reason:         "usage summary fixture",
		BusinessReference: domain.BusinessReference{
			Type: "usage_test",
			ID:   key,
		},
	}
}

// Read-model fixtures need immutable facts, not a second submission orchestrator.
func seedDurableUsageSnapshots(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int64, taskIDs []string) {
	t.Helper()
	var accountID int64
	if err := pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		t.Fatalf("resolve snapshot billing account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, billing_account_id, service, sku, display_name, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at
)
SELECT fixture.task_id, $1, $2, 'generation', price.sku, skus.display_name,
       price.price_version_id, price.version, 0, 1, 0, clock_timestamp()
FROM billing_price_versions price
JOIN billing_skus skus USING (sku)
CROSS JOIN unnest($3::text[]) AS fixture(task_id)
WHERE price.sku = 'ai-image:generation' AND price.version = 1`, userID, accountID, taskIDs); err != nil {
		t.Fatalf("seed task billing snapshots: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO task_billing_settlements (task_id, state, settled_at)
SELECT task_id, 'free', clock_timestamp() FROM unnest($1::text[]) AS fixture(task_id)`, taskIDs); err != nil {
		t.Fatalf("seed task billing settlements: %v", err)
	}
}

func usagePriceInput(sku billing.SKU, displayName string, unitCredits int64) billing.PublishPriceInput {
	namespace, _, _ := strings.Cut(string(sku), ":")
	return billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: sku, Namespace: namespace, ProductKind: "test-feature", DisplayName: displayName,
		},
		UnitCredits: unitCredits,
		ActorID:     1,
	}
}
