package credits

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) GetCaptureUsage(
	ctx context.Context,
	userID int64,
	location *time.Location,
) (totalCredits int64, peakDailyCredits int64, firstCaptureDay time.Time, err error) {
	if s == nil || s.pool == nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: database pool not configured")
	}
	if userID <= 0 {
		return 0, 0, time.Time{}, ErrInvalidCommand
	}
	if location == nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: usage location is required")
	}

	var rawFirstCaptureDay string
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
FROM daily_captures`, userID, location.String()).Scan(
		&totalCredits,
		&peakDailyCredits,
		&rawFirstCaptureDay,
	)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: aggregate capture usage: %w", err)
	}
	if rawFirstCaptureDay == "" {
		return totalCredits, peakDailyCredits, time.Time{}, nil
	}
	firstCaptureDay, err = time.ParseInLocation(time.DateOnly, rawFirstCaptureDay, location)
	if err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("credits service: parse first capture day: %w", err)
	}
	return totalCredits, peakDailyCredits, firstCaptureDay, nil
}
