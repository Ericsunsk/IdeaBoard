package redemptions

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/errorrender"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreditGateway interface {
	Grant(context.Context, pgx.Tx, CreditGrantRequest) (CreditGrantResult, error)
	ReadWallet(context.Context, int64) (WalletSnapshot, error)
}

type CreditGrantRequest struct {
	UserID                int64
	Amount                int64
	IdempotencyKey        string
	ActorType             string
	ActorID               string
	Source                string
	Reason                string
	BusinessReferenceType string
	BusinessReferenceID   string
}

type CreditGrantResult struct {
	TransactionID    string
	AvailableBalance int64
}

type WalletSnapshot struct{ Available int64 }

// Service is the user-facing redemption service.
type Service struct {
	pool    *pgxpool.Pool
	credits CreditGateway
	keyring rewardsdomain.CodeKeyring
	locker  ports.CodeLocker
	now     func() time.Time
}

// ServiceOptions configures the service.
type ServiceOptions struct {
	Now    func() time.Time
	Locker ports.CodeLocker
}

// NewServiceWithOptions creates a service with custom options.
func NewServiceWithOptions(pool *pgxpool.Pool, credits CreditGateway, keyring rewardsdomain.CodeKeyring, options ServiceOptions) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{pool: pool, credits: credits, keyring: keyring, locker: options.Locker, now: now}
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// begin opens the write transaction for one redemption, or reuses
// the ambient guard transaction staged in the context. The bool
// reports ownership: true means the caller opened the transaction
// and must commit or roll it back; false means the transaction is
// BORROWED from the guard, which alone commits or rolls it back,
// so the caller must do neither. With no carrier the pool path
// below runs unchanged.
func (s *Service) begin(ctx context.Context) (pgx.Tx, bool, error) {
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return ambient, false, nil
	}
	if s == nil || s.pool == nil {
		return nil, false, errors.New("rewards service unavailable")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin rewards transaction: %w", err)
	}
	return tx, true, nil
}

const (
	defaultRedemptionHistoryLimit = 20
	maxRedemptionHistoryLimit     = 100
)

// Redeem attempts to redeem a reward code for the requesting user.
func (s *Service) Redeem(ctx context.Context, command RedeemCommand) (RedeemResult, error) {
	if command.UserID <= 0 {
		return RedeemResult{}, ErrInvalidCommand
	}
	if s == nil || s.credits == nil {
		return RedeemResult{}, fmt.Errorf("rewards credit service unavailable")
	}
	if _, err := s.credits.ReadWallet(ctx, command.UserID); err != nil {
		if errors.Is(err, ports.ErrWalletNotFound) {
			return RedeemResult{}, ErrCodeUnavailable
		}
		return RedeemResult{}, err
	}
	normalized, err := rewardsdomain.NormalizeCode(command.Code)
	if err != nil {
		return RedeemResult{}, ErrCodeUnavailable
	}
	result, found, err := s.redeemRewardCode(ctx, command, s.keyring.LookupDigests(normalized))
	if err != nil {
		return RedeemResult{}, err
	}
	if !found {
		return RedeemResult{}, ErrCodeUnavailable
	}
	return result, nil
}

// --- Internal redemption flow ---

func (s *Service) redeemRewardCode(
	ctx context.Context,
	command RedeemCommand,
	candidates []CodeDigest,
) (RedeemResult, bool, error) {
	current, previous := splitCodeDigestCandidates(candidates)
	tx, owned, err := s.begin(ctx)
	if err != nil {
		return RedeemResult{}, false, err
	}
	defer func() {
		if owned {
			_ = tx.Rollback(ctx)
		}
	}()

	rewardCodeID, found, err := lockRewardCodeForRedemption(ctx, tx, s.locker, current, previous)
	if err != nil {
		return RedeemResult{}, true, err
	}
	if !found {
		return RedeemResult{}, false, nil
	}

	rewardCode, err := s.loadRewardCode(ctx, tx, rewardCodeID)
	if errors.Is(err, ErrRewardCodeNotFound) {
		return RedeemResult{}, false, nil
	}
	if err != nil {
		return RedeemResult{}, true, err
	}

	redemption, replayed, err := s.loadRewardRedemption(ctx, tx, command.UserID, rewardCodeID)
	if err != nil {
		return RedeemResult{}, true, err
	}
	if replayed {
		return s.finishRewardRedemptionReplay(ctx, tx, command.UserID, redemption, owned)
	}

	redemptionID, grant, err := s.grantRewardCodeCredits(ctx, tx, command.UserID, rewardCode)
	if err != nil {
		return RedeemResult{}, true, err
	}
	redemption, err = insertRewardRedemption(ctx, tx, redemptionID, command.UserID, rewardCode, grant)
	if err != nil {
		return RedeemResult{}, true, err
	}
	if owned {
		if err := tx.Commit(ctx); err != nil {
			return RedeemResult{}, true, fmt.Errorf("commit reward Code redemption: %w", err)
		}
	}
	return canonicalRedeemResult(redemption, grant.AvailableBalance, false), true, nil
}

func (s *Service) loadRewardCode(ctx context.Context, tx pgx.Tx, rewardCodeID int64) (RewardCode, error) {
	var rc RewardCode
	var hint string
	err := tx.QueryRow(ctx, `SELECT id, display_name, code_hint, credit_amount,
       max_redemptions, redemption_count, expires_at, enabled, created_by,
       created_at, updated_at
FROM reward_codes WHERE id = $1`, rewardCodeID).Scan(
		&rc.ID, &rc.DisplayName, &hint, &rc.CreditAmount,
		&rc.MaxRedemptions, &rc.RedemptionCount, &rc.ExpiresAt,
		&rc.Enabled, &rc.CreatedBy, &rc.CreatedAt, &rc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardCode{}, ErrRewardCodeNotFound
	}
	if err != nil {
		return RewardCode{}, fmt.Errorf("load reward Code: %w", err)
	}
	rc.MaskedCodeHint = rewardsdomain.MaskedCodeHintPrefix + hint
	rc.Status = deriveStatus(rc, s.currentTime())
	return rc, nil
}

func (s *Service) loadRewardRedemption(ctx context.Context, tx pgx.Tx, userID, rewardCodeID int64) (Redemption, bool, error) {
	var redemption Redemption
	err := tx.QueryRow(ctx, `SELECT id, reward_code_id, user_id, reward_code_display_name,
       masked_code_hint, credit_amount, credit_transaction_id, available_balance_after, redeemed_at
FROM reward_redemptions
WHERE user_id = $1 AND reward_code_id = $2`, userID, rewardCodeID).Scan(
		&redemption.ID, &redemption.RewardCodeID, &redemption.UserID,
		&redemption.RewardCodeDisplayName, &redemption.MaskedCodeHint, &redemption.CreditAmount,
		&redemption.CreditTransactionID, &redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Redemption{}, false, nil
	}
	if err != nil {
		return Redemption{}, false, fmt.Errorf("find reward Code redemption: %w", err)
	}
	return redemption, true, nil
}

func (s *Service) finishRewardRedemptionReplay(ctx context.Context, tx pgx.Tx, userID int64, redemption Redemption, owned bool) (RedeemResult, bool, error) {
	if owned {
		if err := tx.Rollback(ctx); err != nil {
			return RedeemResult{}, true, fmt.Errorf("release reward Code replay lock: %w", err)
		}
	}
	wallet, err := s.credits.ReadWallet(ctx, userID)
	if err != nil {
		return RedeemResult{}, true, fmt.Errorf("read reward Code replay Wallet: %w", err)
	}
	return canonicalRedeemResult(redemption, wallet.Available, true), true, nil
}

func (s *Service) grantRewardCodeCredits(ctx context.Context, tx pgx.Tx, userID int64, rewardCode RewardCode) (int64, CreditGrantResult, error) {
	var redemptionID int64
	if err := tx.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('reward_redemptions', 'id'))`).Scan(&redemptionID); err != nil {
		return 0, CreditGrantResult{}, fmt.Errorf("allocate reward Code redemption id: %w", err)
	}
	var capacityReserved bool
	if err := tx.QueryRow(ctx, `SELECT reserve_reward_code_redemption_v1($1)`, rewardCode.ID).Scan(&capacityReserved); err != nil {
		return 0, CreditGrantResult{}, fmt.Errorf("reserve reward Code capacity: %w", err)
	}
	if !capacityReserved {
		return 0, CreditGrantResult{}, ErrCodeUnavailable
	}
	reference := strconv.FormatInt(redemptionID, 10)
	grant, err := s.credits.Grant(ctx, tx, CreditGrantRequest{
		UserID: userID,
		Amount: rewardCode.CreditAmount, IdempotencyKey: "reward-code-redemption:" + reference,
		ActorType: "user", ActorID: strconv.FormatInt(userID, 10), Source: "reward_redemption", Reason: errorrender.MsgGrantRewardCodeCredits,
		BusinessReferenceType: "reward_code_redemption", BusinessReferenceID: "rc:" + reference,
	})
	if err != nil {
		return 0, CreditGrantResult{}, err
	}
	return redemptionID, grant, nil
}

func insertRewardRedemption(ctx context.Context, tx pgx.Tx, redemptionID, userID int64, rewardCode RewardCode, grant CreditGrantResult) (Redemption, error) {
	var redemption Redemption
	err := tx.QueryRow(ctx, `INSERT INTO reward_redemptions (
  id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
  credit_amount, credit_transaction_id, available_balance_after
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
          credit_amount, credit_transaction_id, available_balance_after, redeemed_at`,
		redemptionID, rewardCode.ID, userID, rewardCode.DisplayName, rewardCode.MaskedCodeHint,
		rewardCode.CreditAmount,
		grant.TransactionID, grant.AvailableBalance,
	).Scan(
		&redemption.ID, &redemption.RewardCodeID, &redemption.UserID,
		&redemption.RewardCodeDisplayName, &redemption.MaskedCodeHint, &redemption.CreditAmount,
		&redemption.CreditTransactionID, &redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if err != nil {
		return Redemption{}, fmt.Errorf("record reward Code redemption: %w", err)
	}
	return redemption, nil
}

func canonicalRedeemResult(redemption Redemption, balance int64, replayed bool) RedeemResult {
	return RedeemResult{
		Redemption: redemption,
		HistoryItem: RedemptionHistoryItem{
			ID:                    redemption.ID,
			RewardCodeDisplayName: redemption.RewardCodeDisplayName,
			CreditedAmount:        redemption.CreditAmount,
			MaskedCodeHint:        redemption.MaskedCodeHint,
			RedeemedAt:            redemption.RedeemedAt,
		},
		CreditedAmount:   redemption.CreditAmount,
		AvailableBalance: balance,
		Replayed:         replayed,
	}
}

// --- List ---

// ListRedemptions returns a page of the authenticated user's redemption history.
func (s *Service) ListRedemptions(ctx context.Context, input ListRedemptionsInput) (RedemptionHistoryPage, error) {
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

const redemptionHistorySQL = `
SELECT redemptions.id, redemptions.reward_code_display_name, redemptions.credit_amount,
       redemptions.masked_code_hint, redemptions.redeemed_at
FROM reward_redemptions redemptions
WHERE redemptions.user_id = $1
  AND ($2::BIGINT <= 0 OR redemptions.id < $2)
ORDER BY redemptions.id DESC
LIMIT $3`

// --- Status derivation ---

func deriveStatus(rc RewardCode, now time.Time) ports.RewardCodeStatus {
	if rc.ExpiresAt != nil && !now.Before(*rc.ExpiresAt) {
		return ports.RewardCodeStatusExpired
	}
	if rc.MaxRedemptions > 0 && rc.RedemptionCount >= rc.MaxRedemptions {
		return ports.RewardCodeStatusExhausted
	}
	if !rc.Enabled {
		return ports.RewardCodeStatusDisabled
	}
	return ports.RewardCodeStatusEnabled
}

// --- Lock and lookup helpers ---

func lockRewardCodeForRedemption(ctx context.Context, tx pgx.Tx, locker ports.CodeLocker, current, previous CodeDigest) (int64, bool, error) {
	rewardCodeID, found, err := findRewardCodeAliasOwner(ctx, tx, current, previous)
	if err != nil || !found {
		return rewardCodeID, found, err
	}
	if locker == nil {
		return 0, true, fmt.Errorf("reward code locker unavailable")
	}
	if err := locker.LockRewardCodeID(ctx, tx, rewardCodeID); err != nil {
		return 0, true, err
	}
	lockedOwnerID, stillExists, err := findRewardCodeAliasOwner(ctx, tx, current, previous)
	if err != nil {
		return 0, true, err
	}
	if !stillExists || lockedOwnerID != rewardCodeID {
		return 0, false, nil
	}
	return rewardCodeID, true, nil
}

func findRewardCodeAliasOwner(ctx context.Context, tx pgx.Tx, current, previous CodeDigest) (int64, bool, error) {
	var rewardCodeID int64
	err := tx.QueryRow(ctx, `SELECT reward_code_id
FROM reward_code_digest_aliases
WHERE (key_version = $1 AND code_digest = $2)
   OR ($3 <> '' AND key_version = $3 AND code_digest = $4)
ORDER BY CASE WHEN key_version = $1 THEN 0 ELSE 1 END
LIMIT 1`, current.Version, current.Digest, previous.Version, previous.Digest).Scan(&rewardCodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find reward Code digest alias: %w", err)
	}
	return rewardCodeID, true, nil
}
