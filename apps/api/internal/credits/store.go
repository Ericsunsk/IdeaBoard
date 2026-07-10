package credits

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func normalizeMetadata(metadata CommandMetadata) (CommandMetadata, error) {
	metadata.IdempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)
	metadata.Actor.Type = ActorType(strings.TrimSpace(string(metadata.Actor.Type)))
	metadata.Actor.ID = strings.TrimSpace(metadata.Actor.ID)
	metadata.Source = strings.TrimSpace(metadata.Source)
	metadata.Reason = strings.TrimSpace(metadata.Reason)
	metadata.BusinessReference.Type = strings.TrimSpace(metadata.BusinessReference.Type)
	metadata.BusinessReference.ID = strings.TrimSpace(metadata.BusinessReference.ID)
	if metadata.IdempotencyKey == "" || len(metadata.IdempotencyKey) > 160 ||
		metadata.Actor.Type == "" || metadata.Actor.ID == "" || len(metadata.Actor.ID) > 128 ||
		metadata.Source == "" || len(metadata.Source) > 64 || metadata.Reason == "" ||
		metadata.BusinessReference.Type == "" || len(metadata.BusinessReference.Type) > 64 ||
		metadata.BusinessReference.ID == "" || len(metadata.BusinessReference.ID) > 160 {
		return CommandMetadata{}, ErrInvalidCommand
	}
	switch metadata.Actor.Type {
	case ActorAdmin, ActorSystem, ActorService, ActorUser:
	default:
		return CommandMetadata{}, ErrInvalidCommand
	}
	return metadata, nil
}

func walletSystemShard(userID int64) string {
	return fmt.Sprintf("wallet:%d", userID)
}

func executeCreditCommand(
	ctx context.Context,
	tx pgx.Tx,
	commandType CommandType,
	userID int64,
	amount int64,
	relatedTransactionID string,
	metadata CommandMetadata,
) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	var result CommandResult
	err := tx.QueryRow(ctx, `
SELECT transaction_id,
       command_type,
       command_amount,
       wallet_user_id,
       available_balance_after,
       reserved_balance_after,
       replayed,
       created_at
FROM execute_credit_command($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		commandType,
		userID,
		amount,
		strings.TrimSpace(relatedTransactionID),
		metadata.IdempotencyKey,
		metadata.Actor.Type,
		metadata.Actor.ID,
		metadata.Source,
		metadata.Reason,
		metadata.BusinessReference.Type,
		metadata.BusinessReference.ID,
	).Scan(
		&result.TransactionID,
		&result.CommandType,
		&result.Amount,
		&result.Wallet.UserID,
		&result.Wallet.Available,
		&result.Wallet.Reserved,
		&result.Replayed,
		&result.CreatedAt,
	)
	if err != nil {
		return CommandResult{}, mapCreditDatabaseError(err)
	}
	return result, nil
}

func mapCreditDatabaseError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("credits service: execute command: %w", err)
	}
	switch postgresError.Code {
	case "IB001":
		return ErrIdempotencyConflict
	case "IB002":
		return ErrWalletNotFound
	case "IB003":
		return ErrWalletFrozen
	case "IB004":
		return ErrInsufficientCredits
	case "IB005":
		return ErrHoldNotFound
	case "IB006":
		return ErrHoldSettled
	case "IB007":
		return ErrChargeNotFound
	case "IB008":
		return ErrRefundExceedsCharge
	case "IB009":
		return ErrInvalidCommand
	default:
		return fmt.Errorf("credits service: execute command: %w", err)
	}
}
