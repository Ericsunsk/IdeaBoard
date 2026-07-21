package rewards

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/credits"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreditService interface {
	InTx(pgx.Tx) credits.Commands
	GetWallet(context.Context, int64) (credits.WalletDetails, error)
}

type Service struct {
	pool    *pgxpool.Pool
	credits CreditService
	keyring CodeKeyring
	now     func() time.Time
}

type ServiceOptions struct {
	Now func() time.Time
}

const (
	defaultRedemptionHistoryLimit = 20
	maxRedemptionHistoryLimit     = 100
	maskedCodeHintPrefix          = "•••• "
)

const redemptionHistorySQL = `
SELECT redemptions.id, redemptions.reward_code_display_name, redemptions.credit_amount,
       redemptions.masked_code_hint, redemptions.redeemed_at
FROM reward_redemptions redemptions
WHERE redemptions.user_id = $1
  AND ($2::BIGINT <= 0 OR redemptions.id < $2)
ORDER BY redemptions.id DESC
LIMIT $3`

func NewService(pool *pgxpool.Pool, creditService CreditService, keyring CodeKeyring) *Service {
	return NewServiceWithOptions(pool, creditService, keyring, ServiceOptions{})
}

func NewServiceWithOptions(
	pool *pgxpool.Pool,
	creditService CreditService,
	keyring CodeKeyring,
	options ServiceOptions,
) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{pool: pool, credits: creditService, keyring: keyring, now: now}
}

// ValidateActiveCodeKeyVersions prevents removing a key required to redeem a
// persisted Code or authenticate an idempotent create replay.
func (s *Service) ValidateActiveCodeKeyVersions(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("validate reward Code key versions: service unavailable")
	}
	previousVersion := ""
	if s.keyring.previous != nil {
		previousVersion = s.keyring.previous.Version
	}
	var unsupportedCodeExists bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM reward_codes
  WHERE key_version <> $1 AND ($2 = '' OR key_version <> $2)
)`, s.keyring.current.Version, previousVersion).Scan(&unsupportedCodeExists)
	if err != nil {
		return fmt.Errorf("validate active reward Code key versions: %w", err)
	}
	if unsupportedCodeExists {
		return fmt.Errorf("persisted reward Codes require an unconfigured HMAC key version")
	}
	return nil
}

func (s *Service) Redeem(ctx context.Context, command RedeemCommand) (RedeemResult, error) {
	if command.UserID <= 0 {
		return RedeemResult{}, ErrInvalidCommand
	}
	if s == nil || s.credits == nil {
		return RedeemResult{}, fmt.Errorf("rewards credit service unavailable")
	}
	if _, err := s.credits.GetWallet(ctx, command.UserID); err != nil {
		if errors.Is(err, credits.ErrWalletNotFound) {
			slog.Error("reward redemption Wallet is missing")
		}
		return RedeemResult{}, err
	}
	normalized, err := normalizeCode(command.Code)
	if err != nil {
		return RedeemResult{}, ErrCodeUnavailable
	}
	result, found, err := s.redeemRewardCode(ctx, command, s.keyring.lookupDigests(normalized))
	if err != nil {
		return RedeemResult{}, err
	}
	if !found {
		return RedeemResult{}, ErrCodeUnavailable
	}
	return result, nil
}

func (s *Service) ListRedemptions(
	ctx context.Context,
	input ListRedemptionsInput,
) (RedemptionHistoryPage, error) {
	if s == nil || s.pool == nil || input.UserID <= 0 || input.BeforeID < 0 {
		return RedemptionHistoryPage{}, ErrInvalidCommand
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultRedemptionHistoryLimit
	}
	if limit < 1 || limit > maxRedemptionHistoryLimit {
		return RedemptionHistoryPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, redemptionHistorySQL, input.UserID, input.BeforeID, limit+1)
	if err != nil {
		return RedemptionHistoryPage{}, fmt.Errorf("list reward Redemption history: %w", err)
	}
	defer rows.Close()
	items := make([]RedemptionHistoryItem, 0, limit+1)
	for rows.Next() {
		var item RedemptionHistoryItem
		if err := rows.Scan(
			&item.ID,
			&item.RewardCodeDisplayName,
			&item.CreditedAmount,
			&item.MaskedCodeHint,
			&item.RedeemedAt,
		); err != nil {
			return RedemptionHistoryPage{}, fmt.Errorf("scan reward Redemption history: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return RedemptionHistoryPage{}, fmt.Errorf("read reward Redemption history: %w", err)
	}
	page := RedemptionHistoryPage{Redemptions: items}
	if len(items) > limit {
		page.Redemptions = items[:limit]
		page.NextBeforeID = page.Redemptions[len(page.Redemptions)-1].ID
	}
	return page, nil
}

func splitCodeDigestCandidates(candidates []codeDigest) (codeDigest, codeDigest) {
	current := candidates[0]
	if len(candidates) == 1 {
		return current, codeDigest{}
	}
	return current, candidates[1]
}

func cloneOptionalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC().Truncate(time.Millisecond)
	return &cloned
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

type rowScanner interface {
	Scan(...any) error
}

func (s *Service) begin(ctx context.Context) (pgx.Tx, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("rewards service unavailable")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin rewards transaction: %w", err)
	}
	return tx, nil
}
