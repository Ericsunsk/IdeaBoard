package rewards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ideaboard/api/internal/credits"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) UpdateRewardCode(ctx context.Context, command UpdateRewardCodeCommand) (UpdatedRewardCode, error) {
	displayName := strings.TrimSpace(command.DisplayName)
	if !s.validAdminMutation(command.RewardCodeID, command.ActorUserID) ||
		displayName == "" || utf8.RuneCountInString(displayName) > 120 || command.MaxRedemptions < 0 {
		return UpdatedRewardCode{}, ErrInvalidCommand
	}
	if _, err := credits.NewCreditAmount(command.CreditAmount); err != nil {
		return UpdatedRewardCode{}, errors.Join(ErrInvalidCommand, err)
	}

	var normalized string
	var currentDigest, previousDigest codeDigest
	if command.Code != nil {
		var err error
		normalized, err = normalizeCode(*command.Code)
		if err != nil {
			return UpdatedRewardCode{}, ErrInvalidCommand
		}
		currentDigest, previousDigest = splitCodeDigestCandidates(s.keyring.lookupDigests(normalized))
	}

	tx, err := s.begin(ctx)
	if err != nil {
		return UpdatedRewardCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if command.Code != nil {
		if err := lockRewardCodeIdentity(ctx, tx, normalized); err != nil {
			return UpdatedRewardCode{}, err
		}
	}
	if err := lockRewardCodeID(ctx, tx, command.RewardCodeID); err != nil {
		return UpdatedRewardCode{}, err
	}
	rotationReplayed := false
	if command.Code != nil {
		aliasOwnerID, aliasExists, err := findRewardCodeAliasOwner(ctx, tx, currentDigest, previousDigest)
		if err != nil {
			return UpdatedRewardCode{}, err
		}
		if aliasExists && aliasOwnerID != command.RewardCodeID {
			return UpdatedRewardCode{}, ErrCodeCollision
		}
		rotationReplayed = aliasExists
	}

	var found bool
	if err := tx.QueryRow(ctx, `SELECT update_reward_code_definition_v1($1, $2, $3, $4, $5, $6)`,
		command.RewardCodeID, displayName, command.CreditAmount, command.MaxRedemptions,
		cloneOptionalTime(command.ExpiresAt), command.ActorUserID,
	).Scan(&found); err != nil {
		return UpdatedRewardCode{}, mapRewardManagementError("update reward Code", err)
	}
	if !found {
		return UpdatedRewardCode{}, ErrRewardCodeNotFound
	}

	if command.Code != nil && !rotationReplayed {
		var previousVersion any
		var previousCodeDigest any
		if previousDigest.version != "" {
			previousVersion = previousDigest.version
			previousCodeDigest = previousDigest.digest
		}
		if err := tx.QueryRow(ctx, `SELECT rotate_reward_code_v1($1, $2, $3, $4, $5, $6, $7)`,
			command.RewardCodeID, currentDigest.version, currentDigest.digest,
			normalized[len(normalized)-4:], previousVersion, previousCodeDigest, command.ActorUserID,
		).Scan(&found); err != nil {
			return UpdatedRewardCode{}, mapRewardManagementError("rotate reward Code", err)
		}
		if !found {
			return UpdatedRewardCode{}, ErrRewardCodeNotFound
		}
	}

	updated, err := s.loadRewardCode(ctx, tx, command.RewardCodeID)
	if err != nil {
		return UpdatedRewardCode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UpdatedRewardCode{}, fmt.Errorf("commit reward Code update: %w", err)
	}
	result := UpdatedRewardCode{RewardCode: updated}
	if command.Code != nil {
		result.Replayed = &rotationReplayed
		if !rotationReplayed {
			result.Plaintext = normalized
		}
	}
	return result, nil
}

func (s *Service) SetRewardCodeEnabled(ctx context.Context, command SetRewardCodeEnabledCommand) (RewardCode, error) {
	if !s.validAdminMutation(command.RewardCodeID, command.ActorUserID) {
		return RewardCode{}, ErrInvalidCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return RewardCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRewardCodeID(ctx, tx, command.RewardCodeID); err != nil {
		return RewardCode{}, err
	}
	var found bool
	if err := tx.QueryRow(ctx, `SELECT set_reward_code_enabled_v1($1, $2, $3)`,
		command.RewardCodeID, command.Enabled, command.ActorUserID,
	).Scan(&found); err != nil {
		return RewardCode{}, mapRewardManagementError("set reward Code enabled intent", err)
	}
	if !found {
		return RewardCode{}, ErrRewardCodeNotFound
	}
	updated, err := s.loadRewardCode(ctx, tx, command.RewardCodeID)
	if err != nil {
		return RewardCode{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RewardCode{}, fmt.Errorf("commit reward Code enabled intent: %w", err)
	}
	return updated, nil
}

func (s *Service) DeleteRewardCode(ctx context.Context, command DeleteRewardCodeCommand) (DeletedRewardCode, error) {
	if !s.validAdminMutation(command.RewardCodeID, command.ActorUserID) {
		return DeletedRewardCode{}, ErrInvalidCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return DeletedRewardCode{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRewardCodeID(ctx, tx, command.RewardCodeID); err != nil {
		return DeletedRewardCode{}, err
	}
	var deleted bool
	if err := tx.QueryRow(ctx, `SELECT delete_reward_code_v1($1, $2)`,
		command.RewardCodeID, command.ActorUserID,
	).Scan(&deleted); err != nil {
		return DeletedRewardCode{}, mapRewardManagementError("delete reward Code", err)
	}
	if !deleted {
		return DeletedRewardCode{}, ErrRewardCodeNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return DeletedRewardCode{}, fmt.Errorf("commit reward Code deletion: %w", err)
	}
	return DeletedRewardCode{RewardCodeID: command.RewardCodeID}, nil
}

func (s *Service) validAdminMutation(rewardCodeID, actorUserID int64) bool {
	return s != nil && s.pool != nil && rewardCodeID > 0 && actorUserID > 0
}

func (s *Service) loadRewardCode(ctx context.Context, tx pgx.Tx, rewardCodeID int64) (RewardCode, error) {
	var rewardCode RewardCode
	err := scanRewardCode(tx.QueryRow(ctx, `SELECT `+rewardCodeProjectionSQL+`
FROM reward_codes WHERE id = $1`, rewardCodeID), &rewardCode, s.currentTime())
	if errors.Is(err, pgx.ErrNoRows) {
		return RewardCode{}, ErrRewardCodeNotFound
	}
	if err != nil {
		return RewardCode{}, fmt.Errorf("load reward Code: %w", err)
	}
	return rewardCode, nil
}

func findRewardCodeAliasOwner(
	ctx context.Context,
	tx pgx.Tx,
	current codeDigest,
	previous codeDigest,
) (int64, bool, error) {
	var rewardCodeID int64
	err := tx.QueryRow(ctx, `SELECT reward_code_id
FROM reward_code_digest_aliases
WHERE (key_version = $1 AND code_digest = $2)
   OR ($3 <> '' AND key_version = $3 AND code_digest = $4)
ORDER BY CASE WHEN key_version = $1 THEN 0 ELSE 1 END
LIMIT 1`, current.version, current.digest, previous.version, previous.digest).Scan(&rewardCodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find reward Code digest alias: %w", err)
	}
	return rewardCodeID, true, nil
}

func mapRewardManagementError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return ErrCodeCollision
		case "IB515":
			return ErrCapBelowRedemptions
		case "IB516", "23503":
			return ErrRewardCodeRedeemed
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
