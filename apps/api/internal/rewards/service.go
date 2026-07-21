package rewards

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/credits"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const rewardCodeProjectionSQL = `id, display_name, code_hint, credit_amount, max_redemptions,
       redemption_count, expires_at, enabled, created_by, created_at, updated_at`

func (s *Service) CreateRewardCode(ctx context.Context, command CreateRewardCodeCommand) (CreatedRewardCode, error) {
	if s == nil || s.pool == nil || command.ActorUserID <= 0 {
		return CreatedRewardCode{}, ErrInvalidCommand
	}
	displayName := strings.TrimSpace(command.DisplayName)
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if displayName == "" || utf8.RuneCountInString(displayName) > 120 ||
		idempotencyKey == "" || utf8.RuneCountInString(idempotencyKey) > 128 ||
		command.MaxRedemptions < 0 {
		return CreatedRewardCode{}, ErrInvalidCommand
	}
	if _, err := credits.NewCreditAmount(command.CreditAmount); err != nil {
		return CreatedRewardCode{}, errors.Join(ErrInvalidCommand, err)
	}
	normalized, err := normalizeCode(command.Code)
	if err != nil {
		return CreatedRewardCode{}, ErrInvalidCommand
	}
	expiresAt := cloneOptionalTime(command.ExpiresAt)
	if expiresAt != nil && !s.currentTime().Before(*expiresAt) {
		return CreatedRewardCode{}, ErrInvalidCommand
	}
	digests := s.keyring.lookupDigests(normalized)
	current, previous := splitCodeDigestCandidates(digests)
	fingerprint, err := rewardCodeCreateFingerprint(displayName, command.CreditAmount, command.MaxRedemptions, expiresAt)
	if err != nil {
		return CreatedRewardCode{}, fmt.Errorf("fingerprint reward Code creation: %w", err)
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return CreatedRewardCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRewardCodeIdentity(ctx, tx, normalized); err != nil {
		return CreatedRewardCode{}, err
	}
	var rewardCode RewardCode
	var persistedFingerprint []byte
	err = scanRewardCodeWithFingerprint(tx.QueryRow(ctx, `SELECT `+rewardCodeProjectionSQL+`, request_fingerprint
FROM reward_codes
WHERE created_by = $1 AND idempotency_key = $2`, command.ActorUserID, idempotencyKey), &rewardCode, &persistedFingerprint, s.currentTime())
	if err == nil {
		matches, err := rewardCodeReplayMatches(ctx, tx, rewardCode.ID, persistedFingerprint, fingerprint, digests)
		if err != nil {
			return CreatedRewardCode{}, err
		}
		if !matches {
			return CreatedRewardCode{}, ErrRewardCodeConflict
		}
		if err := tx.Rollback(ctx); err != nil {
			return CreatedRewardCode{}, fmt.Errorf("release reward Code creation replay: %w", err)
		}
		return CreatedRewardCode{RewardCode: rewardCode, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CreatedRewardCode{}, fmt.Errorf("load idempotent reward Code creation: %w", err)
	}
	var codeExists bool
	if err := tx.QueryRow(ctx, `SELECT
  EXISTS (SELECT 1 FROM reward_code_digest_aliases
		  WHERE (key_version = $1 AND code_digest = $2)
             OR ($3 <> '' AND key_version = $3 AND code_digest = $4))`,
		current.version, current.digest, previous.version, previous.digest,
	).Scan(&codeExists); err != nil {
		return CreatedRewardCode{}, fmt.Errorf("check reward Code uniqueness: %w", err)
	}
	if codeExists {
		return CreatedRewardCode{}, ErrCodeCollision
	}
	err = scanRewardCode(tx.QueryRow(ctx, `
INSERT INTO reward_codes (
  display_name, key_version, code_digest, code_hint, credit_amount,
  max_redemptions, expires_at, created_by, idempotency_key, request_fingerprint
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (created_by, idempotency_key) DO NOTHING
RETURNING `+rewardCodeProjectionSQL,
		displayName, current.version, current.digest, normalized[len(normalized)-4:], command.CreditAmount,
		command.MaxRedemptions, expiresAt, command.ActorUserID, idempotencyKey, fingerprint,
	), &rewardCode, s.currentTime())
	if errors.Is(err, pgx.ErrNoRows) {
		persistedFingerprint = nil
		err = scanRewardCodeWithFingerprint(tx.QueryRow(ctx, `SELECT `+rewardCodeProjectionSQL+`, request_fingerprint
FROM reward_codes
WHERE created_by = $1 AND idempotency_key = $2`, command.ActorUserID, idempotencyKey), &rewardCode, &persistedFingerprint, s.currentTime())
		if err != nil {
			return CreatedRewardCode{}, fmt.Errorf("load idempotent reward Code creation: %w", err)
		}
		matches, err := rewardCodeReplayMatches(ctx, tx, rewardCode.ID, persistedFingerprint, fingerprint, digests)
		if err != nil {
			return CreatedRewardCode{}, err
		}
		if !matches {
			return CreatedRewardCode{}, ErrRewardCodeConflict
		}
		if err := tx.Rollback(ctx); err != nil {
			return CreatedRewardCode{}, fmt.Errorf("release reward Code creation replay: %w", err)
		}
		return CreatedRewardCode{RewardCode: rewardCode, Replayed: true}, nil
	}
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return CreatedRewardCode{}, ErrCodeCollision
		}
		return CreatedRewardCode{}, fmt.Errorf("create reward Code: %w", err)
	}
	for _, candidate := range digests {
		if _, err := tx.Exec(ctx, `INSERT INTO reward_code_digest_aliases (reward_code_id, key_version, code_digest)
VALUES ($1, $2, $3)`, rewardCode.ID, candidate.version, candidate.digest); err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "23505" {
				return CreatedRewardCode{}, ErrCodeCollision
			}
			return CreatedRewardCode{}, fmt.Errorf("record reward Code digest alias: %w", err)
		}
	}
	metadata, err := json.Marshal(map[string]any{
		"display_name": displayName, "credit_amount": command.CreditAmount,
		"max_redemptions": command.MaxRedemptions, "expires_at": expiresAt,
		"masked_code_hint": rewardCode.MaskedCodeHint,
	})
	if err != nil {
		return CreatedRewardCode{}, fmt.Errorf("encode reward Code journal metadata: %w", err)
	}
	targetSnapshot, err := json.Marshal(rewardCodeJournalSnapshot(rewardCode))
	if err != nil {
		return CreatedRewardCode{}, fmt.Errorf("encode reward Code journal target: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO reward_admin_actions (
  reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
)
VALUES ($1, $1, 'create_reward_code', $2, $3::JSONB, $4::JSONB)`,
		rewardCode.ID, command.ActorUserID, string(metadata), string(targetSnapshot)); err != nil {
		return CreatedRewardCode{}, fmt.Errorf("record reward Code creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedRewardCode{}, fmt.Errorf("commit reward Code creation: %w", err)
	}
	return CreatedRewardCode{RewardCode: rewardCode, Plaintext: normalized}, nil
}

func (s *Service) ListRewardCodes(ctx context.Context, input ListRewardCodesInput) (RewardCodePage, error) {
	if s == nil || s.pool == nil || input.BeforeID < 0 {
		return RewardCodePage{}, ErrInvalidCommand
	}
	limit := input.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return RewardCodePage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `SELECT `+rewardCodeProjectionSQL+`
FROM reward_codes
WHERE ($1::BIGINT <= 0 OR id < $1)
ORDER BY id DESC
LIMIT $2`, input.BeforeID, limit+1)
	if err != nil {
		return RewardCodePage{}, fmt.Errorf("list reward Codes: %w", err)
	}
	defer rows.Close()
	items := make([]RewardCode, 0, limit+1)
	for rows.Next() {
		var item RewardCode
		if err := scanRewardCode(rows, &item, s.currentTime()); err != nil {
			return RewardCodePage{}, fmt.Errorf("scan reward Code list: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return RewardCodePage{}, fmt.Errorf("read reward Code list: %w", err)
	}
	page := RewardCodePage{RewardCodes: items}
	if len(items) > limit {
		page.RewardCodes = items[:limit]
		page.NextBeforeID = page.RewardCodes[len(page.RewardCodes)-1].ID
	}
	return page, nil
}

func (s *Service) redeemRewardCode(
	ctx context.Context,
	command RedeemCommand,
	candidates []codeDigest,
) (RedeemResult, bool, error) {
	current, previous := splitCodeDigestCandidates(candidates)
	tx, err := s.begin(ctx)
	if err != nil {
		return RedeemResult{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var rewardCode RewardCode
	err = scanRewardCode(tx.QueryRow(ctx, `SELECT `+rewardCodeProjectionSQL+`
FROM reward_codes
WHERE id = (
  SELECT reward_code_id
  FROM reward_code_digest_aliases
  WHERE (key_version = $1 AND code_digest = $2)
     OR ($3 <> '' AND key_version = $3 AND code_digest = $4)
  ORDER BY CASE WHEN key_version = $1 THEN 0 ELSE 1 END
  LIMIT 1
)
FOR UPDATE`, current.version, current.digest, previous.version, previous.digest), &rewardCode, s.currentTime())
	if errors.Is(err, pgx.ErrNoRows) {
		return RedeemResult{}, false, nil
	}
	if err != nil {
		return RedeemResult{}, true, fmt.Errorf("lock reward Code: %w", err)
	}
	var redemption Redemption
	err = tx.QueryRow(ctx, `SELECT id, reward_code_id, user_id, reward_code_display_name,
       masked_code_hint, credit_amount, credit_transaction_id, available_balance_after, redeemed_at
FROM reward_redemptions
WHERE user_id = $1 AND reward_code_id = $2`, command.UserID, rewardCode.ID).Scan(
		&redemption.ID, &redemption.RewardCodeID, &redemption.UserID,
		&redemption.RewardCodeDisplayName, &redemption.MaskedCodeHint, &redemption.CreditAmount,
		&redemption.CreditTransactionID, &redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if err == nil {
		if err := tx.Rollback(ctx); err != nil {
			return RedeemResult{}, true, fmt.Errorf("release reward Code replay lock: %w", err)
		}
		wallet, err := s.credits.GetWallet(ctx, command.UserID)
		if err != nil {
			return RedeemResult{}, true, fmt.Errorf("read reward Code replay Wallet: %w", err)
		}
		return canonicalRedeemResult(redemption, wallet.Available, true), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RedeemResult{}, true, fmt.Errorf("find reward Code redemption: %w", err)
	}
	var redemptionID int64
	if err := tx.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('reward_redemptions', 'id'))`).Scan(&redemptionID); err != nil {
		return RedeemResult{}, true, fmt.Errorf("allocate reward Code redemption id: %w", err)
	}
	var capacityReserved bool
	if err := tx.QueryRow(ctx, `SELECT reserve_reward_code_redemption_v1($1)`, rewardCode.ID).Scan(&capacityReserved); err != nil {
		return RedeemResult{}, true, fmt.Errorf("reserve reward Code capacity: %w", err)
	}
	if !capacityReserved {
		return RedeemResult{}, true, ErrCodeUnavailable
	}
	amount, err := credits.NewCreditAmount(rewardCode.CreditAmount)
	if err != nil {
		return RedeemResult{}, true, errors.Join(ErrInvalidCommand, err)
	}
	reference := strconv.FormatInt(redemptionID, 10)
	grant, err := s.credits.InTx(tx).Grant(ctx, credits.GrantCommand{
		UserID: command.UserID,
		Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey:    "reward-code-redemption:" + reference,
			Actor:             credits.Actor{Type: credits.ActorUser, ID: strconv.FormatInt(command.UserID, 10)},
			Source:            "reward_redemption",
			Reason:            "兑换奖励",
			BusinessReference: credits.BusinessReference{Type: "reward_code_redemption", ID: "rc:" + reference},
		},
	})
	if err != nil {
		return RedeemResult{}, true, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO reward_redemptions (
  id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
  credit_amount, credit_transaction_id, available_balance_after
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
          credit_amount, credit_transaction_id, available_balance_after, redeemed_at`,
		redemptionID, rewardCode.ID, command.UserID, rewardCode.DisplayName, rewardCode.MaskedCodeHint,
		rewardCode.CreditAmount,
		grant.TransactionID, grant.Wallet.Available,
	).Scan(
		&redemption.ID, &redemption.RewardCodeID, &redemption.UserID,
		&redemption.RewardCodeDisplayName, &redemption.MaskedCodeHint, &redemption.CreditAmount,
		&redemption.CreditTransactionID, &redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if err != nil {
		return RedeemResult{}, true, fmt.Errorf("record reward Code redemption: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RedeemResult{}, true, fmt.Errorf("commit reward Code redemption: %w", err)
	}
	return canonicalRedeemResult(redemption, grant.Wallet.Available, false), true, nil
}

func canonicalRedeemResult(redemption Redemption, balance int64, replayed bool) RedeemResult {
	return RedeemResult{
		Redemption: redemption,
		HistoryItem: RedemptionHistoryItem{
			ID: redemption.ID, RewardCodeDisplayName: redemption.RewardCodeDisplayName,
			CreditedAmount: redemption.CreditAmount, MaskedCodeHint: redemption.MaskedCodeHint,
			RedeemedAt: redemption.RedeemedAt,
		},
		CreditedAmount: redemption.CreditAmount, AvailableBalance: balance, Replayed: replayed,
	}
}

func rewardCodeCreateFingerprint(
	displayName string,
	creditAmount int64,
	maxRedemptions int64,
	expiresAt *time.Time,
) ([]byte, error) {
	payload, err := json.Marshal(struct {
		DisplayName    string     `json:"display_name"`
		CreditAmount   int64      `json:"credit_amount"`
		MaxRedemptions int64      `json:"max_redemptions"`
		ExpiresAt      *time.Time `json:"expires_at"`
	}{displayName, creditAmount, maxRedemptions, expiresAt})
	if err != nil {
		return nil, err
	}
	fingerprint := sha256.Sum256(payload)
	return fingerprint[:], nil
}

func rewardCodeReplayMatches(
	ctx context.Context,
	tx pgx.Tx,
	rewardCodeID int64,
	persistedFingerprint []byte,
	expectedFingerprint []byte,
	digests []codeDigest,
) (bool, error) {
	if !bytes.Equal(persistedFingerprint, expectedFingerprint) {
		return false, nil
	}
	current, previous := splitCodeDigestCandidates(digests)
	var aliasMatches bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
  SELECT 1
  FROM reward_code_digest_aliases
  WHERE reward_code_id = $1
    AND ((key_version = $2 AND code_digest = $3)
      OR ($4 <> '' AND key_version = $4 AND code_digest = $5))
)`, rewardCodeID, current.version, current.digest, previous.version, previous.digest).Scan(&aliasMatches); err != nil {
		return false, fmt.Errorf("verify idempotent reward Code creation: %w", err)
	}
	return aliasMatches, nil
}

func scanRewardCode(row rowScanner, rewardCode *RewardCode, now time.Time) error {
	var hint string
	if err := row.Scan(
		&rewardCode.ID, &rewardCode.DisplayName, &hint, &rewardCode.CreditAmount,
		&rewardCode.MaxRedemptions, &rewardCode.RedemptionCount, &rewardCode.ExpiresAt,
		&rewardCode.Enabled, &rewardCode.CreatedBy, &rewardCode.CreatedAt, &rewardCode.UpdatedAt,
	); err != nil {
		return err
	}
	rewardCode.MaskedCodeHint = maskedCodeHintPrefix + hint
	rewardCode.Status = deriveRewardCodeStatus(*rewardCode, now)
	return nil
}

func scanRewardCodeWithFingerprint(row rowScanner, rewardCode *RewardCode, fingerprint *[]byte, now time.Time) error {
	var hint string
	if err := row.Scan(
		&rewardCode.ID, &rewardCode.DisplayName, &hint, &rewardCode.CreditAmount,
		&rewardCode.MaxRedemptions, &rewardCode.RedemptionCount, &rewardCode.ExpiresAt,
		&rewardCode.Enabled, &rewardCode.CreatedBy, &rewardCode.CreatedAt, &rewardCode.UpdatedAt,
		fingerprint,
	); err != nil {
		return err
	}
	rewardCode.MaskedCodeHint = maskedCodeHintPrefix + hint
	rewardCode.Status = deriveRewardCodeStatus(*rewardCode, now)
	return nil
}

func deriveRewardCodeStatus(rewardCode RewardCode, now time.Time) RewardCodeStatus {
	if rewardCode.ExpiresAt != nil && !now.Before(*rewardCode.ExpiresAt) {
		return RewardCodeStatusExpired
	}
	if rewardCode.MaxRedemptions > 0 && rewardCode.RedemptionCount >= rewardCode.MaxRedemptions {
		return RewardCodeStatusExhausted
	}
	if !rewardCode.Enabled {
		return RewardCodeStatusDisabled
	}
	return RewardCodeStatusEnabled
}

func rewardCodeJournalSnapshot(rewardCode RewardCode) map[string]any {
	return map[string]any{
		"id": rewardCode.ID, "display_name": rewardCode.DisplayName,
		"masked_code_hint": rewardCode.MaskedCodeHint, "credit_amount": rewardCode.CreditAmount,
		"max_redemptions": rewardCode.MaxRedemptions, "redemption_count": rewardCode.RedemptionCount,
		"expires_at": rewardCode.ExpiresAt, "enabled": rewardCode.Enabled,
	}
}
