package credits

import (
	"context"
	"database/sql"
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
	return scanCreditCommand(tx.QueryRow(ctx, creditCommandSQL,
		commandType, userID, amount, strings.TrimSpace(relatedTransactionID),
		metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID,
		metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID,
	))
}

func executeCreditPurchase(
	ctx context.Context,
	tx pgx.Tx,
	userID int64,
	amount int64,
	metadata CommandMetadata,
) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPurchaseSQL,
		userID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID,
		metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID,
	))
}

const creditCommandSQL = `
SELECT transaction_id,
       command_type,
       command_amount,
       wallet_user_id,
       available_balance_after,
       reserved_balance_after,
       replayed,
       created_at
FROM execute_credit_command_v2($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

const creditPurchaseSQL = `
SELECT transaction_id,
       command_type,
       command_amount,
       wallet_user_id,
       available_balance_after,
       reserved_balance_after,
       replayed,
       created_at
FROM execute_credit_purchase_v1($1, $2, $3, $4, $5, $6, $7, $8, $9)`

const creditPurchaseRefundHoldSQL = `
SELECT transaction_id,
       command_type,
       command_amount,
       wallet_user_id,
       available_balance_after,
       reserved_balance_after,
       replayed,
       created_at
FROM execute_credit_purchase_refund_hold_v1($1, $2, $3, $4, $5, $6, $7, $8, $9)`

const creditPurchaseRefundSQL = `
SELECT transaction_id,
       command_type,
       command_amount,
       wallet_user_id,
       available_balance_after,
       reserved_balance_after,
       replayed,
       created_at
FROM execute_credit_purchase_refund_v1($1, $2, $3, $4, $5, $6, $7, $8, $9)`

func executeCreditPurchaseRefundHold(ctx context.Context, tx pgx.Tx, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPurchaseRefundHoldSQL, userID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeSQLCreditPurchaseRefundHold(ctx context.Context, tx *sql.Tx, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanCreditCommand(tx.QueryRowContext(ctx, creditPurchaseRefundHoldSQL, userID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

const creditPaymentDisputeDebitSQL = `
SELECT transaction_id, command_type, command_amount, wallet_user_id,
       available_balance_after, reserved_balance_after, replayed, created_at,
       requested_amount, unrecovered_amount
FROM execute_credit_payment_dispute_debit_v1($1, $2, $3, $4, $5, $6, $7, $8, $9)`

const creditPaymentDisputeRestoreSQL = `
SELECT transaction_id, command_type, command_amount, wallet_user_id,
       available_balance_after, reserved_balance_after, replayed, created_at
FROM execute_credit_payment_dispute_restore_v1($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

func executeCreditPurchaseRefund(ctx context.Context, tx pgx.Tx, userID int64, holdID string, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPurchaseRefundSQL, userID, holdID, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeSQLCreditPurchaseRefund(ctx context.Context, tx *sql.Tx, userID int64, holdID string, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanCreditCommand(tx.QueryRowContext(ctx, creditPurchaseRefundSQL, userID, holdID, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeCreditPaymentDisputeDebit(ctx context.Context, tx pgx.Tx, userID, requestedAmount int64, metadata CommandMetadata) (PaymentDisputeDebitResult, error) {
	if tx == nil {
		return PaymentDisputeDebitResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanPaymentDisputeDebit(tx.QueryRow(ctx, creditPaymentDisputeDebitSQL, userID, requestedAmount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeSQLCreditPaymentDisputeDebit(ctx context.Context, tx *sql.Tx, userID, requestedAmount int64, metadata CommandMetadata) (PaymentDisputeDebitResult, error) {
	if tx == nil {
		return PaymentDisputeDebitResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanPaymentDisputeDebit(tx.QueryRowContext(ctx, creditPaymentDisputeDebitSQL, userID, requestedAmount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeCreditPaymentDisputeRestore(ctx context.Context, tx pgx.Tx, userID int64, originalTransactionID string, amount int64, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPaymentDisputeRestoreSQL, userID, originalTransactionID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func executeSQLCreditPaymentDisputeRestore(ctx context.Context, tx *sql.Tx, userID int64, originalTransactionID string, amount int64, metadata CommandMetadata) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanCreditCommand(tx.QueryRowContext(ctx, creditPaymentDisputeRestoreSQL, userID, originalTransactionID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

type rowScanner interface {
	Scan(...any) error
}

func executeSQLCreditCommand(
	ctx context.Context,
	tx *sql.Tx,
	commandType CommandType,
	userID int64,
	amount int64,
	relatedTransactionID string,
	metadata CommandMetadata,
) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanCreditCommand(tx.QueryRowContext(ctx, creditCommandSQL,
		commandType, userID, amount, strings.TrimSpace(relatedTransactionID),
		metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID,
		metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID,
	))
}

func executeSQLCreditPurchase(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	amount int64,
	metadata CommandMetadata,
) (CommandResult, error) {
	if tx == nil {
		return CommandResult{}, fmt.Errorf("credits service: SQL transaction not configured")
	}
	return scanCreditCommand(tx.QueryRowContext(ctx, creditPurchaseSQL,
		userID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID,
		metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID,
	))
}

func scanCreditCommand(row rowScanner) (CommandResult, error) {
	var result CommandResult
	err := row.Scan(
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

func scanPaymentDisputeDebit(row rowScanner) (PaymentDisputeDebitResult, error) {
	var result PaymentDisputeDebitResult
	err := row.Scan(
		&result.TransactionID, &result.CommandType, &result.Amount, &result.Wallet.UserID,
		&result.Wallet.Available, &result.Wallet.Reserved, &result.Replayed, &result.CreatedAt,
		&result.RequestedAmount, &result.UnrecoveredAmount,
	)
	if err != nil {
		return PaymentDisputeDebitResult{}, mapCreditDatabaseError(err)
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
	case "IB010":
		return ErrWelcomeIdentityKeyMismatch
	case "IB011":
		return ErrWalletRestricted
	case "IB012":
		return ErrWalletRestrictionNotFound
	case "IB013":
		return ErrHoldSettlementExceeds
	default:
		return fmt.Errorf("credits service: execute command: %w", err)
	}
}
