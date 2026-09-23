package postgres

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/jackc/pgx/v5"
)

func ExecuteCreditCommand(
	ctx context.Context,
	tx pgx.Tx,
	commandType domain.CommandType,
	userID int64,
	amount int64,
	relatedTransactionID string,
	metadata domain.CommandMetadata,
) (ports.CommandResult, error) {
	if tx == nil {
		return ports.CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditCommandSQL,
		commandType, userID, amount, strings.TrimSpace(relatedTransactionID),
		metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID,
		metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID,
	))
}

func ExecuteCreditPurchase(
	ctx context.Context,
	tx pgx.Tx,
	userID int64,
	amount int64,
	metadata domain.CommandMetadata,
) (ports.CommandResult, error) {
	if tx == nil {
		return ports.CommandResult{}, fmt.Errorf("credits service: transaction not configured")
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

func ExecuteCreditPurchaseRefundHold(ctx context.Context, tx pgx.Tx, userID, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if tx == nil {
		return ports.CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPurchaseRefundHoldSQL, userID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
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

func ExecuteCreditPurchaseRefund(ctx context.Context, tx pgx.Tx, userID int64, holdID string, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if tx == nil {
		return ports.CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPurchaseRefundSQL, userID, holdID, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func ExecuteCreditPaymentDisputeDebit(ctx context.Context, tx pgx.Tx, userID, requestedAmount int64, metadata domain.CommandMetadata) (ports.PaymentDisputeDebitResult, error) {
	if tx == nil {
		return ports.PaymentDisputeDebitResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanPaymentDisputeDebit(tx.QueryRow(ctx, creditPaymentDisputeDebitSQL, userID, requestedAmount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func ExecuteCreditPaymentDisputeRestore(ctx context.Context, tx pgx.Tx, userID int64, originalTransactionID string, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if tx == nil {
		return ports.CommandResult{}, fmt.Errorf("credits service: transaction not configured")
	}
	return scanCreditCommand(tx.QueryRow(ctx, creditPaymentDisputeRestoreSQL, userID, originalTransactionID, amount, metadata.IdempotencyKey, metadata.Actor.Type, metadata.Actor.ID, metadata.Source, metadata.Reason, metadata.BusinessReference.Type, metadata.BusinessReference.ID))
}

func scanCreditCommand(row rowScanner) (ports.CommandResult, error) {
	var result ports.CommandResult
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
		return ports.CommandResult{}, MapCreditDatabaseError(err)
	}
	return result, nil
}

func scanPaymentDisputeDebit(row rowScanner) (ports.PaymentDisputeDebitResult, error) {
	var result ports.PaymentDisputeDebitResult
	err := row.Scan(
		&result.TransactionID, &result.CommandType, &result.Amount, &result.Wallet.UserID,
		&result.Wallet.Available, &result.Wallet.Reserved, &result.Replayed, &result.CreatedAt,
		&result.RequestedAmount, &result.UnrecoveredAmount,
	)
	if err != nil {
		return ports.PaymentDisputeDebitResult{}, MapCreditDatabaseError(err)
	}
	return result, nil
}
