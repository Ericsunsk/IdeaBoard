package rewards

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const rewardCodeProjectionSQL = `id, display_name, code_hint, credit_amount, max_redemptions,
       redemption_count, expires_at, enabled, created_by, created_at, updated_at`

func (s *Service) CreateRewardCode(ctx context.Context, command CreateRewardCodeCommand) (CreatedRewardCode, error) {
	input, err := s.prepareRewardCodeCreation(command)
	if err != nil {
		return CreatedRewardCode{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return CreatedRewardCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRewardCodeIdentity(ctx, tx, input.normalized); err != nil {
		return CreatedRewardCode{}, err
	}
	if replay, ok, err := s.loadRewardCodeReplay(ctx, tx, input, command.ActorUserID); err != nil {
		return CreatedRewardCode{}, err
	} else if ok {
		return finishRewardCodeReplay(ctx, tx, replay)
	}
	if err := ensureRewardCodeCodeAvailable(ctx, tx, input.current, input.previous); err != nil {
		return CreatedRewardCode{}, err
	}
	rewardCode, err := s.insertRewardCodeDefinition(ctx, tx, input, command.ActorUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		replay, ok, replayErr := s.loadRewardCodeReplay(ctx, tx, input, command.ActorUserID)
		if replayErr != nil {
			return CreatedRewardCode{}, replayErr
		}
		if !ok {
			return CreatedRewardCode{}, fmt.Errorf("load idempotent reward Code creation: %w", err)
		}
		return finishRewardCodeReplay(ctx, tx, replay)
	}
	if err != nil {
		return CreatedRewardCode{}, err
	}
	if err := recordRewardCodeDigestAliases(ctx, tx, rewardCode.ID, input.digests); err != nil {
		return CreatedRewardCode{}, err
	}
	if err := recordRewardCodeCreation(ctx, tx, rewardCode, input.displayName, input.creditAmount, input.maxRedemptions, input.expiresAt, command.ActorUserID); err != nil {
		return CreatedRewardCode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedRewardCode{}, fmt.Errorf("commit reward Code creation: %w", err)
	}
	return CreatedRewardCode{RewardCode: rewardCode, Plaintext: input.normalized}, nil
}

type rewardCodeCreationInput struct {
	displayName    string
	idempotencyKey string
	normalized     string
	creditAmount   int64
	maxRedemptions int64
	expiresAt      *time.Time
	digests        []codeDigest
	current        codeDigest
	previous       codeDigest
	fingerprint    []byte
}

func (s *Service) prepareRewardCodeCreation(command CreateRewardCodeCommand) (rewardCodeCreationInput, error) {
	if s == nil || s.pool == nil || command.ActorUserID <= 0 {
		return rewardCodeCreationInput{}, ErrInvalidCommand
	}
	displayName := strings.TrimSpace(command.DisplayName)
	idempotencyKey := strings.TrimSpace(command.IdempotencyKey)
	if displayName == "" || utf8.RuneCountInString(displayName) > 120 ||
		idempotencyKey == "" || utf8.RuneCountInString(idempotencyKey) > 128 ||
		command.MaxRedemptions < 0 {
		return rewardCodeCreationInput{}, ErrInvalidCommand
	}
	if _, err := domain.NewCreditAmount(command.CreditAmount); err != nil {
		return rewardCodeCreationInput{}, errors.Join(ErrInvalidCommand, err)
	}
	normalized, err := normalizeCode(command.Code)
	if err != nil {
		return rewardCodeCreationInput{}, ErrInvalidCommand
	}
	expiresAt := cloneOptionalTime(command.ExpiresAt)
	if expiresAt != nil && !s.currentTime().Before(*expiresAt) {
		return rewardCodeCreationInput{}, ErrInvalidCommand
	}
	digests := s.keyring.lookupDigests(normalized)
	current, previous := splitCodeDigestCandidates(digests)
	fingerprint, err := rewardCodeCreateFingerprint(displayName, command.CreditAmount, command.MaxRedemptions, expiresAt)
	if err != nil {
		return rewardCodeCreationInput{}, fmt.Errorf("fingerprint reward Code creation: %w", err)
	}
	return rewardCodeCreationInput{
		displayName: displayName, idempotencyKey: idempotencyKey, normalized: normalized,
		creditAmount: command.CreditAmount, maxRedemptions: command.MaxRedemptions,
		expiresAt: expiresAt, digests: digests, current: current, previous: previous,
		fingerprint: fingerprint,
	}, nil
}

func (s *Service) loadRewardCodeReplay(ctx context.Context, tx pgx.Tx, input rewardCodeCreationInput, actorUserID int64) (RewardCode, bool, error) {
	var rewardCode RewardCode
	var persistedFingerprint []byte
	err := scanRewardCodeWithFingerprint(tx.QueryRow(ctx, `SELECT `+rewardCodeProjectionSQL+`, request_fingerprint
FROM reward_codes
WHERE created_by = $1 AND idempotency_key = $2`, actorUserID, input.idempotencyKey), &rewardCode, &persistedFingerprint, s.currentTime())
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardCode{}, false, nil
	}
	if err != nil {
		return RewardCode{}, false, fmt.Errorf("load idempotent reward Code creation: %w", err)
	}
	matches, err := rewardCodeReplayMatches(ctx, tx, rewardCode.ID, persistedFingerprint, input.fingerprint, input.digests)
	if err != nil {
		return RewardCode{}, false, err
	}
	if !matches {
		return RewardCode{}, false, ErrRewardCodeConflict
	}
	return rewardCode, true, nil
}

func finishRewardCodeReplay(ctx context.Context, tx pgx.Tx, rewardCode RewardCode) (CreatedRewardCode, error) {
	if err := tx.Rollback(ctx); err != nil {
		return CreatedRewardCode{}, fmt.Errorf("release reward Code creation replay: %w", err)
	}
	return CreatedRewardCode{RewardCode: rewardCode, Replayed: true}, nil
}

func ensureRewardCodeCodeAvailable(ctx context.Context, tx pgx.Tx, current, previous codeDigest) error {
	var codeExists bool
	if err := tx.QueryRow(ctx, `SELECT
  EXISTS (SELECT 1 FROM reward_code_digest_aliases
		  WHERE (key_version = $1 AND code_digest = $2)
             OR ($3 <> '' AND key_version = $3 AND code_digest = $4))`,
		current.version, current.digest, previous.version, previous.digest,
	).Scan(&codeExists); err != nil {
		return fmt.Errorf("check reward Code uniqueness: %w", err)
	}
	if codeExists {
		return ErrCodeCollision
	}
	return nil
}

func (s *Service) insertRewardCodeDefinition(ctx context.Context, tx pgx.Tx, input rewardCodeCreationInput, actorUserID int64) (RewardCode, error) {
	var rewardCode RewardCode
	err := scanRewardCode(tx.QueryRow(ctx, `
INSERT INTO reward_codes (
  display_name, key_version, code_digest, code_hint, credit_amount,
  max_redemptions, expires_at, created_by, idempotency_key, request_fingerprint
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (created_by, idempotency_key) DO NOTHING
RETURNING `+rewardCodeProjectionSQL,
		input.displayName, input.current.version, input.current.digest, input.normalized[len(input.normalized)-4:], input.creditAmount,
		input.maxRedemptions, input.expiresAt, actorUserID, input.idempotencyKey, input.fingerprint,
	), &rewardCode, s.currentTime())
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardCode{}, err
	}
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return RewardCode{}, ErrCodeCollision
		}
		return RewardCode{}, fmt.Errorf("create reward Code: %w", err)
	}
	return rewardCode, nil
}

func recordRewardCodeDigestAliases(ctx context.Context, tx pgx.Tx, rewardCodeID int64, digests []codeDigest) error {
	for _, candidate := range digests {
		if _, err := tx.Exec(ctx, `INSERT INTO reward_code_digest_aliases (reward_code_id, key_version, code_digest)
VALUES ($1, $2, $3)`, rewardCodeID, candidate.version, candidate.digest); err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "23505" {
				return ErrCodeCollision
			}
			return fmt.Errorf("record reward Code digest alias: %w", err)
		}
	}
	return nil
}

func recordRewardCodeCreation(ctx context.Context, tx pgx.Tx, rewardCode RewardCode, displayName string, creditAmount, maxRedemptions int64, expiresAt *time.Time, actorUserID int64) error {
	metadata, err := json.Marshal(map[string]any{
		"display_name": displayName, "credit_amount": creditAmount,
		"max_redemptions": maxRedemptions, "expires_at": expiresAt,
		"masked_code_hint": rewardCode.MaskedCodeHint,
	})
	if err != nil {
		return fmt.Errorf("encode reward Code journal metadata: %w", err)
	}
	targetSnapshot, err := json.Marshal(rewardCodeJournalSnapshot(rewardCode))
	if err != nil {
		return fmt.Errorf("encode reward Code journal target: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO reward_admin_actions (
  reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
)
VALUES ($1, $1, 'create_reward_code', $2, $3::JSONB, $4::JSONB)`,
		rewardCode.ID, actorUserID, string(metadata), string(targetSnapshot)); err != nil {
		return fmt.Errorf("record reward Code creation: %w", err)
	}
	return nil
}

func (s *Service) SearchRewardCodes(ctx context.Context, input SearchRewardCodesInput) (RewardCodePage, error) {
	query := strings.TrimSpace(input.Query)
	if s == nil || s.pool == nil || input.BeforeID < 0 || !validRewardCodeStatusFilter(input.Status) ||
		utf8.RuneCountInString(query) > maxRawCodeBytes {
		return RewardCodePage{}, ErrInvalidCommand
	}
	limit := input.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return RewardCodePage{}, ErrInvalidCommand
	}
	var current, previous codeDigest
	nameQuery := query
	// Code-shaped input stays out of PostgreSQL text parameters so database error logs cannot expose it.
	if normalized, err := normalizeCode(query); err == nil {
		current, previous = splitCodeDigestCandidates(s.keyring.lookupDigests(normalized))
		nameQuery = ""
	}
	now := s.currentTime()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return RewardCodePage{}, fmt.Errorf("begin reward Code search: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	arguments := []any{
		query != "", nameQuery, current.version, current.digest, previous.version, previous.digest,
		string(input.Status), now,
	}
	var total int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)
FROM reward_codes codes
WHERE `+rewardCodeSearchPredicateSQL, arguments...).Scan(&total); err != nil {
		return RewardCodePage{}, fmt.Errorf("count filtered reward Codes: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+rewardCodeProjectionSQL+`
FROM reward_codes codes
WHERE `+rewardCodeSearchPredicateSQL+`
  AND ($9::BIGINT <= 0 OR id < $9)
ORDER BY id DESC
LIMIT $10`, append(arguments, input.BeforeID, limit+1)...)
	if err != nil {
		return RewardCodePage{}, fmt.Errorf("search reward Codes: %w", err)
	}
	defer rows.Close()
	items := make([]RewardCode, 0, limit+1)
	for rows.Next() {
		var item RewardCode
		if err := scanRewardCode(rows, &item, now); err != nil {
			return RewardCodePage{}, fmt.Errorf("scan reward Code search: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return RewardCodePage{}, fmt.Errorf("read reward Code search: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return RewardCodePage{}, fmt.Errorf("commit reward Code search: %w", err)
	}
	page := RewardCodePage{RewardCodes: items, Total: total}
	if len(items) > limit {
		page.RewardCodes = items[:limit]
		page.NextBeforeID = page.RewardCodes[len(page.RewardCodes)-1].ID
	}
	return page, nil
}

const rewardCodeSearchPredicateSQL = `
(
  NOT $1::BOOLEAN
  OR ($2::TEXT <> '' AND STRPOS(LOWER(codes.display_name), LOWER($2)) > 0)
  OR EXISTS (
    SELECT 1
    FROM reward_code_digest_aliases aliases
    WHERE aliases.reward_code_id = codes.id
      AND (($3::TEXT <> '' AND aliases.key_version = $3 AND aliases.code_digest = $4::BYTEA)
        OR ($5::TEXT <> '' AND aliases.key_version = $5 AND aliases.code_digest = $6::BYTEA))
  )
)
AND (
  $7::TEXT = ''
  OR CASE
    WHEN codes.expires_at IS NOT NULL AND $8::TIMESTAMPTZ >= codes.expires_at THEN 'expired'
    WHEN codes.max_redemptions > 0 AND codes.redemption_count >= codes.max_redemptions THEN 'exhausted'
    WHEN NOT codes.enabled THEN 'disabled'
    ELSE 'enabled'
  END = $7
)`

func validRewardCodeStatusFilter(status RewardCodeStatus) bool {
	switch status {
	case "", RewardCodeStatusExpired, RewardCodeStatusExhausted,
		RewardCodeStatusDisabled, RewardCodeStatusEnabled:
		return true
	default:
		return false
	}
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
	rewardCodeID, found, err := lockRewardCodeForRedemption(ctx, tx, current, previous)
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
	redemption, replayed, err := s.loadRewardRedemption(ctx, tx, command.UserID, rewardCode.ID)
	if err != nil {
		return RedeemResult{}, true, err
	}
	if replayed {
		return s.finishRewardRedemptionReplay(ctx, tx, command.UserID, redemption)
	}
	redemptionID, grant, err := s.grantRewardCodeCredits(ctx, tx, command.UserID, rewardCode)
	if err != nil {
		return RedeemResult{}, true, err
	}
	redemption, err = insertRewardRedemption(ctx, tx, redemptionID, command.UserID, rewardCode, grant)
	if err != nil {
		return RedeemResult{}, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RedeemResult{}, true, fmt.Errorf("commit reward Code redemption: %w", err)
	}
	return canonicalRedeemResult(redemption, grant.Wallet.Available, false), true, nil
}

func lockRewardCodeForRedemption(ctx context.Context, tx pgx.Tx, current, previous codeDigest) (int64, bool, error) {
	rewardCodeID, found, err := findRewardCodeAliasOwner(ctx, tx, current, previous)
	if err != nil || !found {
		return rewardCodeID, found, err
	}
	if err := lockRewardCodeID(ctx, tx, rewardCodeID); err != nil {
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

func (s *Service) finishRewardRedemptionReplay(ctx context.Context, tx pgx.Tx, userID int64, redemption Redemption) (RedeemResult, bool, error) {
	if err := tx.Rollback(ctx); err != nil {
		return RedeemResult{}, true, fmt.Errorf("release reward Code replay lock: %w", err)
	}
	wallet, err := s.credits.GetWallet(ctx, userID)
	if err != nil {
		return RedeemResult{}, true, fmt.Errorf("read reward Code replay Wallet: %w", err)
	}
	return canonicalRedeemResult(redemption, wallet.Available, true), true, nil
}

func (s *Service) grantRewardCodeCredits(ctx context.Context, tx pgx.Tx, userID int64, rewardCode RewardCode) (int64, ports.CommandResult, error) {
	var redemptionID int64
	if err := tx.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('reward_redemptions', 'id'))`).Scan(&redemptionID); err != nil {
		return 0, ports.CommandResult{}, fmt.Errorf("allocate reward Code redemption id: %w", err)
	}
	var capacityReserved bool
	if err := tx.QueryRow(ctx, `SELECT reserve_reward_code_redemption_v1($1)`, rewardCode.ID).Scan(&capacityReserved); err != nil {
		return 0, ports.CommandResult{}, fmt.Errorf("reserve reward Code capacity: %w", err)
	}
	if !capacityReserved {
		return 0, ports.CommandResult{}, ErrCodeUnavailable
	}
	amount, err := domain.NewCreditAmount(rewardCode.CreditAmount)
	if err != nil {
		return 0, ports.CommandResult{}, errors.Join(ErrInvalidCommand, err)
	}
	reference := strconv.FormatInt(redemptionID, 10)
	grant, err := s.credits.InTx(tx).Grant(ctx, ports.GrantCommand{
		UserID: userID,
		Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "reward-code-redemption:" + reference,
			Actor:             domain.Actor{Type: domain.ActorUser, ID: strconv.FormatInt(userID, 10)},
			Source:            "reward_redemption",
			Reason:            "兑换奖励",
			BusinessReference: domain.BusinessReference{Type: "reward_code_redemption", ID: "rc:" + reference},
		},
	})
	if err != nil {
		return 0, ports.CommandResult{}, err
	}
	return redemptionID, grant, nil
}

func insertRewardRedemption(ctx context.Context, tx pgx.Tx, redemptionID, userID int64, rewardCode RewardCode, grant ports.CommandResult) (Redemption, error) {
	var redemption Redemption
	err := tx.QueryRow(ctx, `INSERT INTO reward_redemptions (
  id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
  credit_amount, credit_transaction_id, available_balance_after
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, reward_code_id, user_id, reward_code_display_name, masked_code_hint,
          credit_amount, credit_transaction_id, available_balance_after, redeemed_at`,
		redemptionID, rewardCode.ID, userID, rewardCode.DisplayName, rewardCode.MaskedCodeHint,
		rewardCode.CreditAmount,
		grant.TransactionID, grant.Wallet.Available,
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
