package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/adminaction"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *Service) CreateCampaign(ctx context.Context, command CreateCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	displayName := strings.TrimSpace(command.DisplayName)
	internalNote := strings.TrimSpace(command.InternalNote)
	if displayName == "" || utf8.RuneCountInString(displayName) > 120 ||
		utf8.RuneCountInString(internalNote) > 1000 || command.MaxRedemptions <= 0 {
		return Campaign{}, ErrInvalidCommand
	}
	if _, err := credits.NewCreditAmount(command.CreditAmount); err != nil {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	startsAt, expiresAt, err := normalizeCampaignSchedule(command.StartsAt, command.ExpiresAt)
	if err != nil {
		return Campaign{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var campaign Campaign
	err = scanCampaign(tx.QueryRow(ctx, `
INSERT INTO reward_campaigns (
  display_name, internal_note, credit_amount, mode, status,
  max_redemptions, starts_at, expires_at, created_by
) VALUES ($1, $2, $3, 'public', 'draft', $4, $5, $6, $7)
RETURNING id, display_name, internal_note, credit_amount, mode, status,
          max_redemptions, redemption_count, starts_at, expires_at,
          created_by, created_at, updated_at`,
		displayName, internalNote, command.CreditAmount, command.MaxRedemptions,
		startsAt, expiresAt, metadata.ActorID(),
	), &campaign, s.currentTime())
	if err != nil {
		return Campaign{}, fmt.Errorf("create reward campaign: %w", err)
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "create_campaign", metadata, map[string]any{
		"display_name": displayName, "credit_amount": command.CreditAmount,
		"max_redemptions": command.MaxRedemptions, "starts_at": startsAt, "expires_at": expiresAt,
	}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign creation: %w", err)
	}
	return campaign, nil
}

func (s *Service) UpdateCampaign(ctx context.Context, command UpdateCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	displayName := strings.TrimSpace(command.DisplayName)
	internalNote := strings.TrimSpace(command.InternalNote)
	if displayName == "" || utf8.RuneCountInString(displayName) > 120 ||
		utf8.RuneCountInString(internalNote) > 1000 || command.MaxRedemptions <= 0 {
		return Campaign{}, ErrInvalidCommand
	}
	if _, err := credits.NewCreditAmount(command.CreditAmount); err != nil {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	startsAt, expiresAt, err := normalizeCampaignSchedule(command.StartsAt, command.ExpiresAt)
	if err != nil {
		return Campaign{}, err
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	campaign, err := s.lockCampaign(ctx, tx, command.CampaignID)
	if err != nil {
		return Campaign{}, err
	}
	var codeIssued bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM reward_codes WHERE campaign_id = $1)`, campaign.ID,
	).Scan(&codeIssued); err != nil {
		return Campaign{}, fmt.Errorf("check reward campaign issuance: %w", err)
	}
	if codeIssued && campaignRulesChanged(campaign, displayName, command.CreditAmount, command.MaxRedemptions, startsAt, expiresAt) {
		return Campaign{}, ErrCampaignRulesFrozen
	}
	if err := scanCampaign(tx.QueryRow(ctx, `
UPDATE reward_campaigns
SET display_name = $2,
    internal_note = $3,
    credit_amount = $4,
    max_redemptions = $5,
    starts_at = $6,
    expires_at = $7
WHERE id = $1
RETURNING id, display_name, internal_note, credit_amount, mode, status,
          max_redemptions, redemption_count, starts_at, expires_at,
          created_by, created_at, updated_at`,
		campaign.ID, displayName, internalNote, command.CreditAmount,
		command.MaxRedemptions, startsAt, expiresAt,
	), &campaign, s.currentTime()); err != nil {
		return Campaign{}, fmt.Errorf("update reward campaign: %w", err)
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "update_campaign", metadata, map[string]any{
		"display_name": displayName, "credit_amount": command.CreditAmount,
		"max_redemptions": command.MaxRedemptions, "starts_at": startsAt, "expires_at": expiresAt,
	}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign update: %w", err)
	}
	return campaign, nil
}

func (s *Service) IssuePublicCode(ctx context.Context, command IssuePublicCodeCommand) (IssuedPublicCode, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return IssuedPublicCode{}, errors.Join(ErrInvalidCommand, err)
	}
	plaintext, err := normalizeCode(command.Code)
	if err != nil {
		return IssuedPublicCode{}, err
	}
	current, previous := splitCodeDigestCandidates(s.keyring.lookupDigests(plaintext))

	tx, err := s.begin(ctx)
	if err != nil {
		return IssuedPublicCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status CampaignLifecycleStatus
	if err := tx.QueryRow(ctx,
		`SELECT status FROM reward_campaigns WHERE id = $1 FOR UPDATE`, command.CampaignID,
	).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IssuedPublicCode{}, ErrCampaignNotFound
		}
		return IssuedPublicCode{}, fmt.Errorf("lock reward campaign: %w", err)
	}
	if status != CampaignLifecycleDraft {
		return IssuedPublicCode{}, ErrCampaignNotDraft
	}
	var codeExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM reward_codes
  WHERE (key_version = $1 AND code_digest = $2)
     OR ($3 <> '' AND key_version = $3 AND code_digest = $4)
)`, current.version, current.digest, previous.version, previous.digest).Scan(&codeExists); err != nil {
		return IssuedPublicCode{}, fmt.Errorf("check reward code reuse: %w", err)
	}
	if codeExists {
		return IssuedPublicCode{}, ErrPublicCodeExists
	}
	var code RewardCode
	err = tx.QueryRow(ctx, `
INSERT INTO reward_codes (
  campaign_id, key_version, code_digest, code_hint, issued_by
) VALUES ($1, $2, $3, $4, $5)
RETURNING id, campaign_id, code_hint, issued_by, issued_at`,
		command.CampaignID, current.version, current.digest, plaintext[len(plaintext)-4:], metadata.ActorID(),
	).Scan(&code.ID, &code.CampaignID, &code.Hint, &code.IssuedBy, &code.IssuedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return IssuedPublicCode{}, ErrPublicCodeExists
		}
		return IssuedPublicCode{}, fmt.Errorf("issue public reward code: %w", err)
	}
	if err := insertAdminAction(ctx, tx, command.CampaignID, "issue_public_code", metadata, map[string]any{
		"code_id": code.ID, "code_hint": code.Hint, "key_version": current.version,
	}); err != nil {
		return IssuedPublicCode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedPublicCode{}, fmt.Errorf("commit public reward code issuance: %w", err)
	}
	return IssuedPublicCode{Code: code, Plaintext: plaintext}, nil
}

func (s *Service) ActivateCampaign(ctx context.Context, command ActivateCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var campaign Campaign
	err = scanCampaign(tx.QueryRow(ctx, `
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE id = $1
FOR UPDATE`, command.CampaignID), &campaign, s.currentTime())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Campaign{}, ErrCampaignNotFound
		}
		return Campaign{}, fmt.Errorf("lock reward campaign: %w", err)
	}
	if campaign.LifecycleStatus != CampaignLifecycleDraft {
		return Campaign{}, ErrCampaignNotDraft
	}
	if campaign.ExpiresAt != nil && !s.currentTime().Before(*campaign.ExpiresAt) {
		return Campaign{}, ErrCampaignExpired
	}
	var codeExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM reward_codes WHERE campaign_id = $1 AND revoked_at IS NULL)`, campaign.ID,
	).Scan(&codeExists); err != nil {
		return Campaign{}, fmt.Errorf("check public reward code: %w", err)
	}
	if !codeExists || campaign.MaxRedemptions <= 0 || campaign.CreditAmount <= 0 {
		return Campaign{}, ErrPublicCodeRequired
	}
	err = scanCampaign(tx.QueryRow(ctx, `
UPDATE reward_campaigns
SET status = 'active'
WHERE id = $1 AND status = 'draft'
RETURNING id, display_name, internal_note, credit_amount, mode, status,
          max_redemptions, redemption_count, starts_at, expires_at,
	          created_by, created_at, updated_at`, campaign.ID), &campaign, s.currentTime())
	if err != nil {
		return Campaign{}, fmt.Errorf("activate reward campaign: %w", err)
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "activate_campaign", metadata, map[string]any{}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign activation: %w", err)
	}
	return campaign, nil
}

func (s *Service) PauseCampaign(ctx context.Context, command PauseCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	campaign, err := s.lockCampaign(ctx, tx, command.CampaignID)
	if err != nil {
		return Campaign{}, err
	}
	if campaign.LifecycleStatus != CampaignLifecycleActive {
		return Campaign{}, ErrCampaignNotActive
	}
	if err := s.updateCampaignLifecycle(ctx, tx, &campaign, CampaignLifecyclePaused); err != nil {
		return Campaign{}, err
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "pause_campaign", metadata, map[string]any{}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign pause: %w", err)
	}
	return campaign, nil
}

func (s *Service) ResumeCampaign(ctx context.Context, command ResumeCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	campaign, err := s.lockCampaign(ctx, tx, command.CampaignID)
	if err != nil {
		return Campaign{}, err
	}
	if campaign.LifecycleStatus != CampaignLifecyclePaused {
		return Campaign{}, ErrCampaignNotPaused
	}
	if campaign.RedemptionCount >= campaign.MaxRedemptions {
		return Campaign{}, ErrCampaignExhausted
	}
	if campaign.ExpiresAt != nil && !s.currentTime().Before(*campaign.ExpiresAt) {
		return Campaign{}, ErrCampaignExpired
	}
	var codeExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM reward_codes WHERE campaign_id = $1 AND revoked_at IS NULL)`, campaign.ID,
	).Scan(&codeExists); err != nil {
		return Campaign{}, fmt.Errorf("check resumable reward code: %w", err)
	}
	if !codeExists {
		return Campaign{}, ErrPublicCodeRequired
	}
	if err := s.updateCampaignLifecycle(ctx, tx, &campaign, CampaignLifecycleActive); err != nil {
		return Campaign{}, err
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "resume_campaign", metadata, map[string]any{}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign resume: %w", err)
	}
	return campaign, nil
}

func (s *Service) DeleteCampaign(ctx context.Context, command DeleteCampaignCommand) error {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 {
		return errors.Join(ErrInvalidCommand, err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	campaign, err := s.lockCampaign(ctx, tx, command.CampaignID)
	if err != nil {
		return err
	}
	if campaign.LifecycleStatus != CampaignLifecycleDraft {
		return ErrCampaignNotEmpty
	}
	var codeIssued bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM reward_codes WHERE campaign_id = $1)`, campaign.ID,
	).Scan(&codeIssued); err != nil {
		return fmt.Errorf("check deletable reward campaign: %w", err)
	}
	if codeIssued {
		return ErrCampaignNotEmpty
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "delete_campaign", metadata, map[string]any{
		"display_name": campaign.DisplayName,
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM reward_campaigns WHERE id = $1`, campaign.ID); err != nil {
		return fmt.Errorf("delete empty reward Draft: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reward campaign deletion: %w", err)
	}
	return nil
}

func (s *Service) CancelCampaign(ctx context.Context, command CancelCampaignCommand) (Campaign, error) {
	metadata, err := adminaction.New(command.ActorUserID, command.Reason)
	if err != nil || command.CampaignID <= 0 || !command.Confirmed {
		return Campaign{}, errors.Join(ErrInvalidCommand, err)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	campaign, err := s.lockCampaign(ctx, tx, command.CampaignID)
	if err != nil {
		return Campaign{}, err
	}
	if campaign.LifecycleStatus == CampaignLifecycleCancelled {
		return campaign, nil
	}
	var codeExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM reward_codes WHERE campaign_id = $1)`, campaign.ID,
	).Scan(&codeExists); err != nil {
		return Campaign{}, fmt.Errorf("check cancellable reward campaign: %w", err)
	}
	if !codeExists {
		return Campaign{}, ErrPublicCodeRequired
	}
	if err := s.updateCampaignLifecycle(ctx, tx, &campaign, CampaignLifecycleCancelled); err != nil {
		return Campaign{}, err
	}
	revokedAt := s.currentTime()
	result, err := tx.Exec(ctx, `
UPDATE reward_codes
SET revoked_at = $2, revoked_by = $3
WHERE campaign_id = $1 AND revoked_at IS NULL`, campaign.ID, revokedAt, metadata.ActorID())
	if err != nil {
		return Campaign{}, fmt.Errorf("revoke cancelled reward Codes: %w", err)
	}
	if err := insertAdminAction(ctx, tx, campaign.ID, "cancel_campaign", metadata, map[string]any{
		"revoked_code_count": result.RowsAffected(),
	}); err != nil {
		return Campaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, fmt.Errorf("commit reward campaign cancellation: %w", err)
	}
	return campaign, nil
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
	candidates := s.keyring.lookupDigests(normalized)
	current, previous := splitCodeDigestCandidates(candidates)

	tx, err := s.begin(ctx)
	if err != nil {
		return RedeemResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var codeID, campaignID int64
	err = tx.QueryRow(ctx, `
SELECT id, campaign_id
FROM reward_codes
WHERE (key_version = $1 AND code_digest = $2)
   OR ($3 <> '' AND key_version = $3 AND code_digest = $4)
ORDER BY CASE WHEN key_version = $1 THEN 0 ELSE 1 END
LIMIT 1`, current.version, current.digest, previous.version, previous.digest).Scan(&codeID, &campaignID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedeemResult{}, ErrCodeUnavailable
		}
		return RedeemResult{}, fmt.Errorf("lookup reward code: %w", err)
	}

	var campaign Campaign
	if err := scanCampaign(tx.QueryRow(ctx, `
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE id = $1
FOR UPDATE`, campaignID), &campaign, s.currentTime()); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedeemResult{}, ErrCodeUnavailable
		}
		return RedeemResult{}, fmt.Errorf("lock reward campaign: %w", err)
	}
	var lockedCodeID int64
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `
SELECT id, revoked_at
FROM reward_codes
WHERE id = $1 AND campaign_id = $2
FOR UPDATE`, codeID, campaign.ID).Scan(&lockedCodeID, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RedeemResult{}, ErrCodeUnavailable
		}
		return RedeemResult{}, fmt.Errorf("lock reward code: %w", err)
	}

	redemption, found, err := findRedemption(ctx, tx, command.UserID, campaign.ID)
	if err != nil {
		return RedeemResult{}, err
	}
	if found {
		if redemption.CodeID != codeID {
			return RedeemResult{}, ErrAlreadyRedeemed
		}
		if err := tx.Rollback(ctx); err != nil {
			return RedeemResult{}, fmt.Errorf("release reward replay locks: %w", err)
		}
		wallet, err := s.credits.GetWallet(ctx, command.UserID)
		if err != nil {
			return RedeemResult{}, fmt.Errorf("read reward replay Wallet: %w", err)
		}
		return RedeemResult{
			Redemption: redemption, CreditedAmount: redemption.CreditAmount,
			AvailableBalance: wallet.Available, Replayed: true,
		}, nil
	}
	now := s.currentTime()
	if revokedAt != nil || campaign.LifecycleStatus != CampaignLifecycleActive ||
		(campaign.StartsAt != nil && now.Before(*campaign.StartsAt)) ||
		(campaign.ExpiresAt != nil && !now.Before(*campaign.ExpiresAt)) {
		return RedeemResult{}, ErrCodeUnavailable
	}
	var redemptionID int64
	if err := tx.QueryRow(ctx,
		`SELECT nextval(pg_get_serial_sequence('reward_redemptions', 'id'))`,
	).Scan(&redemptionID); err != nil {
		return RedeemResult{}, fmt.Errorf("allocate reward redemption id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE reward_campaigns
SET redemption_count = redemption_count + 1
WHERE id = $1 AND status = 'active' AND redemption_count < max_redemptions`, campaign.ID); err != nil {
		return RedeemResult{}, fmt.Errorf("reserve reward campaign capacity: %w", err)
	}
	var reserved bool
	if err := tx.QueryRow(ctx,
		`SELECT redemption_count <= max_redemptions AND redemption_count > $2 FROM reward_campaigns WHERE id = $1`,
		campaign.ID, campaign.RedemptionCount,
	).Scan(&reserved); err != nil {
		return RedeemResult{}, fmt.Errorf("verify reward campaign capacity: %w", err)
	}
	if !reserved {
		return RedeemResult{}, ErrCampaignExhausted
	}
	amount, err := credits.NewCreditAmount(campaign.CreditAmount)
	if err != nil {
		return RedeemResult{}, errors.Join(ErrInvalidCommand, err)
	}
	redemptionReference := strconv.FormatInt(redemptionID, 10)
	grant, err := s.credits.InTx(tx).Grant(ctx, credits.GrantCommand{
		UserID: command.UserID,
		Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey:    "reward-redemption:" + redemptionReference,
			Actor:             credits.Actor{Type: credits.ActorUser, ID: strconv.FormatInt(command.UserID, 10)},
			Source:            "reward_redemption",
			Reason:            "兑换奖励",
			BusinessReference: credits.BusinessReference{Type: "reward_redemption", ID: redemptionReference},
		},
	})
	if err != nil {
		return RedeemResult{}, err
	}
	err = tx.QueryRow(ctx, `
INSERT INTO reward_redemptions (
  id, campaign_id, code_id, user_id, credit_amount,
  credit_transaction_id, available_balance_after
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, campaign_id, code_id, user_id, credit_amount,
          credit_transaction_id, available_balance_after, redeemed_at`,
		redemptionID, campaign.ID, codeID, command.UserID, campaign.CreditAmount,
		grant.TransactionID, grant.Wallet.Available,
	).Scan(
		&redemption.ID, &redemption.CampaignID, &redemption.CodeID, &redemption.UserID,
		&redemption.CreditAmount, &redemption.CreditTransactionID,
		&redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if err != nil {
		return RedeemResult{}, fmt.Errorf("record reward redemption: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RedeemResult{}, fmt.Errorf("commit reward redemption: %w", err)
	}
	return RedeemResult{
		Redemption: redemption, CreditedAmount: campaign.CreditAmount,
		AvailableBalance: grant.Wallet.Available,
	}, nil
}

func splitCodeDigestCandidates(candidates []codeDigest) (codeDigest, codeDigest) {
	current := candidates[0]
	if len(candidates) == 1 {
		return current, codeDigest{}
	}
	return current, candidates[1]
}

func (s *Service) GetCampaign(ctx context.Context, campaignID int64) (Campaign, error) {
	if s == nil || s.pool == nil || campaignID <= 0 {
		return Campaign{}, ErrCampaignNotFound
	}
	var campaign Campaign
	if err := scanCampaign(s.pool.QueryRow(ctx, `
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE id = $1`, campaignID), &campaign, s.currentTime()); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Campaign{}, ErrCampaignNotFound
		}
		return Campaign{}, fmt.Errorf("get reward campaign: %w", err)
	}
	return campaign, nil
}

func (s *Service) ListCampaigns(ctx context.Context, input ListCampaignsInput) (CampaignPage, error) {
	if s == nil || s.pool == nil {
		return CampaignPage{}, fmt.Errorf("rewards service unavailable")
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.BeforeID < 0 || input.Limit < 1 || input.Limit > 100 {
		return CampaignPage{}, ErrInvalidCommand
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE ($1::BIGINT <= 0 OR id < $1)
ORDER BY id DESC
LIMIT $2`, input.BeforeID, input.Limit+1)
	if err != nil {
		return CampaignPage{}, fmt.Errorf("list reward campaigns: %w", err)
	}
	defer rows.Close()
	campaigns := make([]Campaign, 0, input.Limit+1)
	for rows.Next() {
		var campaign Campaign
		if err := scanCampaign(rows, &campaign, s.currentTime()); err != nil {
			return CampaignPage{}, fmt.Errorf("scan reward campaign: %w", err)
		}
		campaigns = append(campaigns, campaign)
	}
	if err := rows.Err(); err != nil {
		return CampaignPage{}, fmt.Errorf("read reward campaigns: %w", err)
	}
	page := CampaignPage{Campaigns: campaigns}
	if len(campaigns) > input.Limit {
		page.Campaigns = campaigns[:input.Limit]
		page.NextBeforeID = page.Campaigns[len(page.Campaigns)-1].ID
	}
	return page, nil
}

func normalizeCampaignSchedule(startsAt, expiresAt *time.Time) (*time.Time, *time.Time, error) {
	startsAt = cloneCampaignTime(startsAt)
	expiresAt = cloneCampaignTime(expiresAt)
	if startsAt != nil && expiresAt != nil && !startsAt.Before(*expiresAt) {
		return nil, nil, ErrInvalidCommand
	}
	return startsAt, expiresAt, nil
}

func cloneCampaignTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC().Truncate(time.Millisecond)
	return &cloned
}

func campaignRulesChanged(
	campaign Campaign,
	displayName string,
	creditAmount int64,
	maxRedemptions int64,
	startsAt *time.Time,
	expiresAt *time.Time,
) bool {
	return campaign.DisplayName != displayName ||
		campaign.CreditAmount != creditAmount ||
		campaign.MaxRedemptions != maxRedemptions ||
		!sameCampaignTime(campaign.StartsAt, startsAt) ||
		!sameCampaignTime(campaign.ExpiresAt, expiresAt)
}

func sameCampaignTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func deriveCampaignStatus(campaign Campaign, now time.Time) CampaignStatus {
	switch campaign.LifecycleStatus {
	case CampaignLifecycleDraft:
		return CampaignStatusDraft
	case CampaignLifecycleCancelled:
		return CampaignStatusCancelled
	case CampaignLifecyclePaused:
		// Time and capacity remain higher-precedence facts than an operational pause.
	}
	if campaign.ExpiresAt != nil && !now.Before(*campaign.ExpiresAt) {
		return CampaignStatusExpired
	}
	if campaign.RedemptionCount >= campaign.MaxRedemptions {
		return CampaignStatusExhausted
	}
	if campaign.LifecycleStatus == CampaignLifecyclePaused {
		return CampaignStatusPaused
	}
	if campaign.StartsAt != nil && now.Before(*campaign.StartsAt) {
		return CampaignStatusScheduled
	}
	return CampaignStatusActive
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) lockCampaign(ctx context.Context, tx pgx.Tx, campaignID int64) (Campaign, error) {
	var campaign Campaign
	if err := scanCampaign(tx.QueryRow(ctx, `
SELECT id, display_name, internal_note, credit_amount, mode, status,
       max_redemptions, redemption_count, starts_at, expires_at,
       created_by, created_at, updated_at
FROM reward_campaigns
WHERE id = $1
FOR UPDATE`, campaignID), &campaign, s.currentTime()); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Campaign{}, ErrCampaignNotFound
		}
		return Campaign{}, fmt.Errorf("lock reward campaign: %w", err)
	}
	return campaign, nil
}

func (s *Service) updateCampaignLifecycle(
	ctx context.Context,
	tx pgx.Tx,
	campaign *Campaign,
	status CampaignLifecycleStatus,
) error {
	if err := scanCampaign(tx.QueryRow(ctx, `
UPDATE reward_campaigns
SET status = $2
WHERE id = $1
RETURNING id, display_name, internal_note, credit_amount, mode, status,
          max_redemptions, redemption_count, starts_at, expires_at,
          created_by, created_at, updated_at`, campaign.ID, status), campaign, s.currentTime()); err != nil {
		return fmt.Errorf("update reward campaign lifecycle: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanCampaign(row rowScanner, campaign *Campaign, now time.Time) error {
	if err := row.Scan(
		&campaign.ID, &campaign.DisplayName, &campaign.InternalNote, &campaign.CreditAmount,
		&campaign.Mode, &campaign.LifecycleStatus, &campaign.MaxRedemptions, &campaign.RedemptionCount,
		&campaign.StartsAt, &campaign.ExpiresAt,
		&campaign.CreatedBy, &campaign.CreatedAt, &campaign.UpdatedAt,
	); err != nil {
		return err
	}
	campaign.Status = deriveCampaignStatus(*campaign, now)
	return nil
}

func findRedemption(ctx context.Context, tx pgx.Tx, userID, campaignID int64) (Redemption, bool, error) {
	var redemption Redemption
	err := tx.QueryRow(ctx, `
SELECT id, campaign_id, code_id, user_id, credit_amount,
       credit_transaction_id, available_balance_after, redeemed_at
FROM reward_redemptions
WHERE user_id = $1 AND campaign_id = $2`, userID, campaignID).Scan(
		&redemption.ID, &redemption.CampaignID, &redemption.CodeID, &redemption.UserID,
		&redemption.CreditAmount, &redemption.CreditTransactionID,
		&redemption.AvailableBalanceAfter, &redemption.RedeemedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Redemption{}, false, nil
	}
	if err != nil {
		return Redemption{}, false, fmt.Errorf("find reward redemption replay: %w", err)
	}
	return redemption, true, nil
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

func insertAdminAction(
	ctx context.Context,
	tx pgx.Tx,
	campaignID int64,
	actionType string,
	metadata adminaction.Metadata,
	actionMetadata map[string]any,
) error {
	encoded, err := json.Marshal(actionMetadata)
	if err != nil {
		return fmt.Errorf("encode reward Admin action: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO reward_admin_actions (
  campaign_id, action_type, actor_user_id, reason, metadata
) VALUES ($1, $2, $3, $4, $5::JSONB)`,
		campaignID, actionType, metadata.ActorID(), metadata.Reason(), string(encoded),
	)
	if err != nil {
		return fmt.Errorf("record reward Admin action: %w", err)
	}
	return nil
}
