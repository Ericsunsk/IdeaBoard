package credits

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CaptureUsageReader is the read-only Credits seam for product usage views.
// It exposes settled Capture facts without granting access to Ledger writes.
type CaptureUsageReader interface {
	GetCaptureUsage(context.Context, int64, string) (CaptureUsageSummary, error)
}

type CaptureUsageSummary struct {
	TotalCredits     int64
	PeakDailyCredits int64
	FirstCaptureDay  time.Time
}

func (s *Service) GetCaptureUsage(
	ctx context.Context,
	userID int64,
	timezone string,
) (CaptureUsageSummary, error) {
	if s == nil || s.pool == nil {
		return CaptureUsageSummary{}, fmt.Errorf("credits service: database pool not configured")
	}
	if userID <= 0 {
		return CaptureUsageSummary{}, ErrInvalidCommand
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return CaptureUsageSummary{}, fmt.Errorf("credits service: invalid usage timezone: %w", err)
	}

	var summary CaptureUsageSummary
	var firstCaptureDay string
	err = s.pool.QueryRow(ctx, `
WITH daily_captures AS (
  SELECT (created_at AT TIME ZONE $2)::date AS capture_day,
         SUM(command_amount)::numeric AS amount
  FROM credit_transactions
  WHERE wallet_user_id = $1
    AND command_type = 'capture'
  GROUP BY (created_at AT TIME ZONE $2)::date
)
SELECT COALESCE(SUM(amount), 0)::bigint,
       COALESCE(MAX(amount), 0)::bigint,
       COALESCE(MIN(capture_day)::text, '')
FROM daily_captures`, userID, timezone).Scan(
		&summary.TotalCredits,
		&summary.PeakDailyCredits,
		&firstCaptureDay,
	)
	if err != nil {
		return CaptureUsageSummary{}, fmt.Errorf("credits service: aggregate capture usage: %w", err)
	}
	if firstCaptureDay == "" {
		return summary, nil
	}
	summary.FirstCaptureDay, err = time.ParseInLocation(time.DateOnly, firstCaptureDay, location)
	if err != nil {
		return CaptureUsageSummary{}, fmt.Errorf("credits service: parse first capture day: %w", err)
	}
	return summary, nil
}
