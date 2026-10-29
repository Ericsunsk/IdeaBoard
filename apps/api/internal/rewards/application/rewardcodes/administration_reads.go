package rewardcodes

import (
	"context"
	"errors"
	"fmt"

	"ideaboard/api/internal/platform/pagination"

	"github.com/jackc/pgx/v5"
)

const (
	defaultRedemptionHistoryLimit = 20
	maxRedemptionHistoryLimit     = 100
)

func (s *Service) ListAdminRedemptions(
	ctx context.Context,
	input ListAdminRedemptionsInput,
) (AdminRedemptionPage, error) {
	if s == nil || s.pool == nil || input.RewardCodeID <= 0 || input.BeforeID < 0 {
		return AdminRedemptionPage{}, ErrInvalidCommand
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultRedemptionHistoryLimit
	}
	if limit < 1 || limit > maxRedemptionHistoryLimit {
		return AdminRedemptionPage{}, ErrInvalidCommand
	}
	var rewardCodeID int64
	if err := s.pool.QueryRow(ctx, `SELECT id FROM reward_codes WHERE id = $1`, input.RewardCodeID).Scan(&rewardCodeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminRedemptionPage{}, ErrRewardCodeNotFound
		}
		return AdminRedemptionPage{}, fmt.Errorf("load RewardCode for admin Redemption list: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, reward_code_id, user_id, credit_amount, redeemed_at
FROM reward_redemptions
WHERE reward_code_id = $1
  AND ($2::BIGINT <= 0 OR id < $2)
ORDER BY id DESC
LIMIT $3`, input.RewardCodeID, input.BeforeID, limit+1)
	if err != nil {
		return AdminRedemptionPage{}, fmt.Errorf("list admin RewardCode Redemptions: %w", err)
	}
	defer rows.Close()
	items := make([]AdminRedemptionSummary, 0, limit+1)
	for rows.Next() {
		var item AdminRedemptionSummary
		if err := rows.Scan(&item.ID, &item.RewardCodeID, &item.UserID, &item.CreditedAmount, &item.RedeemedAt); err != nil {
			return AdminRedemptionPage{}, fmt.Errorf("scan admin RewardCode Redemption: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AdminRedemptionPage{}, fmt.Errorf("read admin RewardCode Redemptions: %w", err)
	}
	page := AdminRedemptionPage{Redemptions: items}
	page.Redemptions, page.NextBeforeID = pagination.TruncateBeforeID(page.Redemptions, limit, func(item AdminRedemptionSummary) int64 { return item.ID })
	return page, nil
}

func (s *Service) GetAdminRedemption(
	ctx context.Context,
	input GetAdminRedemptionInput,
) (AdminRedemptionDetail, error) {
	if s == nil || s.pool == nil || input.RewardCodeID <= 0 || input.RedemptionID <= 0 {
		return AdminRedemptionDetail{}, ErrInvalidCommand
	}
	var detail AdminRedemptionDetail
	err := s.pool.QueryRow(ctx, `
SELECT id, reward_code_id, user_id, credit_amount, redeemed_at,
       credit_transaction_id, masked_code_hint
FROM reward_redemptions
WHERE reward_code_id = $1 AND id = $2`, input.RewardCodeID, input.RedemptionID).Scan(
		&detail.ID,
		&detail.RewardCodeID,
		&detail.UserID,
		&detail.CreditedAmount,
		&detail.RedeemedAt,
		&detail.CreditTransactionID,
		&detail.MaskedCodeHint,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminRedemptionDetail{}, ErrRedemptionNotFound
		}
		return AdminRedemptionDetail{}, fmt.Errorf("load admin RewardCode Redemption: %w", err)
	}
	return detail, nil
}
