package credits

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

type Service struct {
	pool                     *pgxpool.Pool
	reconciliationDispatcher ReconciliationRunDispatcher
}

type transactionCommands struct {
	tx pgx.Tx
}

type sqlTransactionCommands struct {
	tx *sql.Tx
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func NewServiceWithReconciliation(
	pool *pgxpool.Pool,
	dispatcher ReconciliationRunDispatcher,
) (*Service, error) {
	if pool == nil {
		return nil, fmt.Errorf("configure Credits reconciliation: pool is nil")
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("configure Credits reconciliation: dispatcher is nil")
	}
	return &Service{pool: pool, reconciliationDispatcher: dispatcher}, nil
}

// InTx lets Tasking compose Credits commands into its existing PostgreSQL
// transaction. The caller owns commit and rollback.
func (s *Service) InTx(tx pgx.Tx) Commands {
	return &transactionCommands{tx: tx}
}

// CommandsInSQLTx composes Credits commands into an existing database/sql
// transaction without requiring an application-lifetime Service. The caller
// owns commit and rollback.
func CommandsInSQLTx(tx *sql.Tx) (Commands, error) {
	if tx == nil {
		return nil, fmt.Errorf("credits: SQL transaction is nil")
	}
	return &sqlTransactionCommands{tx: tx}, nil
}

// LifecycleInTx lets Identity compose Wallet changes with user writes.
// The caller owns commit and rollback.
func (s *Service) LifecycleInTx(tx pgx.Tx) WalletLifecycle {
	return &transactionCommands{tx: tx}
}

func (s *Service) Grant(ctx context.Context, command GrantCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Grant(ctx, command)
	})
}

func (s *Service) Purchase(ctx context.Context, command PurchaseCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Purchase(ctx, command)
	})
}

func (s *Service) PurchaseRefundHold(ctx context.Context, command PurchaseRefundHoldCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.PurchaseRefundHold(ctx, command)
	})
}

func (s *Service) PurchaseRefund(ctx context.Context, command PurchaseRefundCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.PurchaseRefund(ctx, command)
	})
}

func (s *Service) PaymentDisputeDebit(ctx context.Context, command PaymentDisputeDebitCommand) (PaymentDisputeDebitResult, error) {
	return withManagedTransactionResult(ctx, s, func(commands *transactionCommands) (PaymentDisputeDebitResult, error) {
		return commands.PaymentDisputeDebit(ctx, command)
	})
}

func (s *Service) PaymentDisputeRestore(ctx context.Context, command PaymentDisputeRestoreCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.PaymentDisputeRestore(ctx, command)
	})
}

func (s *Service) Deduct(ctx context.Context, command DeductionCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Deduct(ctx, command)
	})
}

func (s *Service) Correct(ctx context.Context, command CorrectionCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Correct(ctx, command)
	})
}

func (s *Service) Hold(ctx context.Context, command HoldCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Hold(ctx, command)
	})
}

func (s *Service) Capture(ctx context.Context, command CaptureCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Capture(ctx, command)
	})
}

func (s *Service) Release(ctx context.Context, command ReleaseCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Release(ctx, command)
	})
}

func (s *Service) Refund(ctx context.Context, command RefundCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Refund(ctx, command)
	})
}

func withManagedTransaction(
	ctx context.Context,
	service *Service,
	run func(*transactionCommands) (CommandResult, error),
) (CommandResult, error) {
	if service == nil || service.pool == nil {
		return CommandResult{}, fmt.Errorf("credits service: database pool not configured")
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CommandResult{}, fmt.Errorf("credits service: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits transaction", "err", rollbackErr)
		}
	}()
	result, err := run(&transactionCommands{tx: tx})
	if err != nil {
		return CommandResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CommandResult{}, fmt.Errorf("credits service: commit transaction: %w", err)
	}
	committed = true
	return result, nil
}

func withManagedTransactionResult(
	ctx context.Context,
	service *Service,
	run func(*transactionCommands) (PaymentDisputeDebitResult, error),
) (PaymentDisputeDebitResult, error) {
	if service == nil || service.pool == nil {
		return PaymentDisputeDebitResult{}, fmt.Errorf("credits service: database pool not configured")
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PaymentDisputeDebitResult{}, fmt.Errorf("credits service: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits transaction", "err", rollbackErr)
		}
	}()
	result, err := run(&transactionCommands{tx: tx})
	if err != nil {
		return PaymentDisputeDebitResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentDisputeDebitResult{}, fmt.Errorf("credits service: commit transaction: %w", err)
	}
	committed = true
	return result, nil
}

func (c *transactionCommands) Grant(ctx context.Context, command GrantCommand) (CommandResult, error) {
	return executeGrant(ctx, command, c.execute)
}

func (c *transactionCommands) Purchase(ctx context.Context, command PurchaseCommand) (CommandResult, error) {
	return executePurchase(ctx, command, c.executePurchase)
}

func (c *transactionCommands) Deduct(ctx context.Context, command DeductionCommand) (CommandResult, error) {
	return executeDeduction(ctx, command, c.execute)
}

func (c *transactionCommands) Correct(ctx context.Context, command CorrectionCommand) (CommandResult, error) {
	return executeCorrection(ctx, command, c.execute)
}

func (c *transactionCommands) ProvisionWallet(ctx context.Context, userID int64) error {
	if c == nil || c.tx == nil || userID <= 0 {
		return ErrInvalidCommand
	}
	if _, err := c.tx.Exec(ctx, `SELECT provision_credit_wallet($1)`, userID); err != nil {
		return mapCreditDatabaseError(err)
	}
	return nil
}

func (c *transactionCommands) BindWelcomeIdentityKey(ctx context.Context, keyFingerprint string) error {
	keyFingerprint = strings.TrimSpace(keyFingerprint)
	if c == nil || c.tx == nil || len(keyFingerprint) != 64 {
		return ErrInvalidCommand
	}
	if _, err := c.tx.Exec(ctx, `SELECT bind_credit_welcome_identity_key($1)`, keyFingerprint); err != nil {
		return mapCreditDatabaseError(err)
	}
	return nil
}

func (c *transactionCommands) SetWalletFrozen(ctx context.Context, userID int64, frozen bool) error {
	if c == nil || c.tx == nil || userID <= 0 {
		return ErrInvalidCommand
	}
	status := "active"
	if frozen {
		status = "frozen"
	}
	if _, err := c.tx.Exec(ctx, `SELECT set_credit_wallet_status($1, $2)`, userID, status); err != nil {
		return mapCreditDatabaseError(err)
	}
	return nil
}

func (c *transactionCommands) ClaimWelcomeGrant(
	ctx context.Context,
	identityFingerprint string,
	command GrantCommand,
) (CommandResult, bool, error) {
	identityFingerprint = strings.TrimSpace(identityFingerprint)
	if c == nil || c.tx == nil || len(identityFingerprint) != 64 || command.UserID <= 0 {
		return CommandResult{}, false, ErrInvalidCommand
	}
	var claimed bool
	if err := c.tx.QueryRow(ctx, `SELECT claim_credit_welcome($1, $2)`, identityFingerprint, command.UserID).Scan(&claimed); err != nil {
		return CommandResult{}, false, mapCreditDatabaseError(err)
	}
	if !claimed {
		return CommandResult{}, false, nil
	}
	result, err := c.Grant(ctx, command)
	if err != nil {
		return CommandResult{}, false, err
	}
	return result, true, nil
}

func (c *transactionCommands) Hold(ctx context.Context, command HoldCommand) (CommandResult, error) {
	return executeHold(ctx, command, c.execute)
}

func (c *transactionCommands) PurchaseRefundHold(ctx context.Context, command PurchaseRefundHoldCommand) (CommandResult, error) {
	return executePurchaseRefundHold(ctx, command, c.executePurchaseRefundHold)
}

func (c *transactionCommands) PurchaseRefund(ctx context.Context, command PurchaseRefundCommand) (CommandResult, error) {
	return executePurchaseRefund(ctx, command, c.executePurchaseRefund)
}

func (c *transactionCommands) PaymentDisputeDebit(ctx context.Context, command PaymentDisputeDebitCommand) (PaymentDisputeDebitResult, error) {
	return executePaymentDisputeDebit(ctx, command, c.executePaymentDisputeDebit)
}

func (c *transactionCommands) PaymentDisputeRestore(ctx context.Context, command PaymentDisputeRestoreCommand) (CommandResult, error) {
	return executePaymentDisputeRestore(ctx, command, c.executePaymentDisputeRestore)
}

func (c *transactionCommands) Capture(ctx context.Context, command CaptureCommand) (CommandResult, error) {
	return executeCapture(ctx, command, c.execute)
}

func (c *transactionCommands) Release(ctx context.Context, command ReleaseCommand) (CommandResult, error) {
	return executeRelease(ctx, command, c.execute)
}

func (c *transactionCommands) Refund(ctx context.Context, command RefundCommand) (CommandResult, error) {
	return executeRefund(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Grant(ctx context.Context, command GrantCommand) (CommandResult, error) {
	return executeGrant(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Purchase(ctx context.Context, command PurchaseCommand) (CommandResult, error) {
	return executePurchase(ctx, command, c.executePurchase)
}

func (c *sqlTransactionCommands) Deduct(ctx context.Context, command DeductionCommand) (CommandResult, error) {
	return executeDeduction(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Correct(ctx context.Context, command CorrectionCommand) (CommandResult, error) {
	return executeCorrection(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Hold(ctx context.Context, command HoldCommand) (CommandResult, error) {
	return executeHold(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) PurchaseRefundHold(ctx context.Context, command PurchaseRefundHoldCommand) (CommandResult, error) {
	return executePurchaseRefundHold(ctx, command, c.executeSQLPurchaseRefundHold)
}

func (c *sqlTransactionCommands) PurchaseRefund(ctx context.Context, command PurchaseRefundCommand) (CommandResult, error) {
	return executePurchaseRefund(ctx, command, c.executeSQLPurchaseRefund)
}

func (c *sqlTransactionCommands) PaymentDisputeDebit(ctx context.Context, command PaymentDisputeDebitCommand) (PaymentDisputeDebitResult, error) {
	return executePaymentDisputeDebit(ctx, command, c.executePaymentDisputeDebit)
}

func (c *sqlTransactionCommands) PaymentDisputeRestore(ctx context.Context, command PaymentDisputeRestoreCommand) (CommandResult, error) {
	return executePaymentDisputeRestore(ctx, command, c.executePaymentDisputeRestore)
}

func (c *sqlTransactionCommands) Capture(ctx context.Context, command CaptureCommand) (CommandResult, error) {
	return executeCapture(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Release(ctx context.Context, command ReleaseCommand) (CommandResult, error) {
	return executeRelease(ctx, command, c.execute)
}

func (c *sqlTransactionCommands) Refund(ctx context.Context, command RefundCommand) (CommandResult, error) {
	return executeRefund(ctx, command, c.execute)
}

type commandExecutor func(context.Context, CommandType, int64, int64, string, CommandMetadata) (CommandResult, error)
type purchaseExecutor func(context.Context, int64, int64, CommandMetadata) (CommandResult, error)
type purchaseRefundHoldExecutor func(context.Context, int64, int64, CommandMetadata) (CommandResult, error)
type purchaseRefundExecutor func(context.Context, int64, string, CommandMetadata) (CommandResult, error)
type paymentDisputeDebitExecutor func(context.Context, int64, int64, CommandMetadata) (PaymentDisputeDebitResult, error)
type paymentDisputeRestoreExecutor func(context.Context, int64, string, int64, CommandMetadata) (CommandResult, error)

func (c *transactionCommands) execute(ctx context.Context, commandType CommandType, userID, amount int64, relatedID string, metadata CommandMetadata) (CommandResult, error) {
	return executeCreditCommand(ctx, c.tx, commandType, userID, amount, relatedID, metadata)
}

func (c *sqlTransactionCommands) execute(ctx context.Context, commandType CommandType, userID, amount int64, relatedID string, metadata CommandMetadata) (CommandResult, error) {
	return executeSQLCreditCommand(ctx, c.tx, commandType, userID, amount, relatedID, metadata)
}

func (c *transactionCommands) executePurchase(ctx context.Context, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeCreditPurchase(ctx, c.tx, userID, amount, metadata)
}

func (c *transactionCommands) executePaymentDisputeDebit(ctx context.Context, userID, requestedAmount int64, metadata CommandMetadata) (PaymentDisputeDebitResult, error) {
	return executeCreditPaymentDisputeDebit(ctx, c.tx, userID, requestedAmount, metadata)
}

func (c *transactionCommands) executePaymentDisputeRestore(ctx context.Context, userID int64, originalTransactionID string, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeCreditPaymentDisputeRestore(ctx, c.tx, userID, originalTransactionID, amount, metadata)
}

func (c *sqlTransactionCommands) executePurchase(ctx context.Context, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeSQLCreditPurchase(ctx, c.tx, userID, amount, metadata)
}

func (c *transactionCommands) executePurchaseRefundHold(ctx context.Context, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeCreditPurchaseRefundHold(ctx, c.tx, userID, amount, metadata)
}

func (c *sqlTransactionCommands) executeSQLPurchaseRefundHold(ctx context.Context, userID, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeSQLCreditPurchaseRefundHold(ctx, c.tx, userID, amount, metadata)
}

func (c *sqlTransactionCommands) executePaymentDisputeDebit(ctx context.Context, userID, requestedAmount int64, metadata CommandMetadata) (PaymentDisputeDebitResult, error) {
	return executeSQLCreditPaymentDisputeDebit(ctx, c.tx, userID, requestedAmount, metadata)
}

func (c *sqlTransactionCommands) executePaymentDisputeRestore(ctx context.Context, userID int64, originalTransactionID string, amount int64, metadata CommandMetadata) (CommandResult, error) {
	return executeSQLCreditPaymentDisputeRestore(ctx, c.tx, userID, originalTransactionID, amount, metadata)
}

func (c *transactionCommands) executePurchaseRefund(ctx context.Context, userID int64, holdID string, metadata CommandMetadata) (CommandResult, error) {
	return executeCreditPurchaseRefund(ctx, c.tx, userID, holdID, metadata)
}

func (c *sqlTransactionCommands) executeSQLPurchaseRefund(ctx context.Context, userID int64, holdID string, metadata CommandMetadata) (CommandResult, error) {
	return executeSQLCreditPurchaseRefund(ctx, c.tx, userID, holdID, metadata)
}

func executeGrant(ctx context.Context, command GrantCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandGrant, command.UserID, command.Amount.Int64(), "", metadata)
}

func executePurchase(ctx context.Context, command PurchaseCommand, execute purchaseExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_purchase" || metadata.BusinessReference.Type != "payment_order" {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, command.UserID, command.Amount.Int64(), metadata)
}

func executePurchaseRefund(ctx context.Context, command PurchaseRefundCommand, execute purchaseRefundExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	holdID := strings.TrimSpace(command.HoldTransactionID)
	if err != nil || command.UserID <= 0 || holdID == "" || len(holdID) > 64 ||
		metadata.Actor.Type != ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_refund" || metadata.BusinessReference.Type != "payment_refund" {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, command.UserID, holdID, metadata)
}

func executePurchaseRefundHold(ctx context.Context, command PurchaseRefundHoldCommand, execute purchaseRefundHoldExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_refund_hold" || metadata.BusinessReference.Type != "payment_refund" {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, command.UserID, command.Amount.Int64(), metadata)
}

func executePaymentDisputeDebit(ctx context.Context, command PaymentDisputeDebitCommand, execute paymentDisputeDebitExecutor) (PaymentDisputeDebitResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.RequestedAmount) ||
		metadata.Actor.Type != ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_dispute" || metadata.BusinessReference.Type != "payment_dispute" {
		return PaymentDisputeDebitResult{}, ErrInvalidCommand
	}
	return execute(ctx, command.UserID, command.RequestedAmount.Int64(), metadata)
}

func executePaymentDisputeRestore(ctx context.Context, command PaymentDisputeRestoreCommand, execute paymentDisputeRestoreExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	originalID := strings.TrimSpace(command.OriginalTransactionID)
	if err != nil || command.UserID <= 0 || originalID == "" || len(originalID) > 64 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_dispute" || metadata.BusinessReference.Type != "payment_dispute" {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, command.UserID, originalID, command.Amount.Int64(), metadata)
}

func executeDeduction(ctx context.Context, command DeductionCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || metadata.Actor.Type != ActorAdmin || command.UserID <= 0 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandDeduction, command.UserID, command.Amount.Int64(), "", metadata)
}

func executeCorrection(ctx context.Context, command CorrectionCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	originalTransactionID := strings.TrimSpace(command.OriginalTransactionID)
	if err != nil || metadata.Actor.Type != ActorAdmin || command.UserID <= 0 || originalTransactionID == "" || len(originalTransactionID) > 64 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandCorrection, command.UserID, command.Amount.Int64(), originalTransactionID, metadata)
}

func executeHold(ctx context.Context, command HoldCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandHold, command.UserID, command.Amount.Int64(), "", metadata)
}

func executeCapture(ctx context.Context, command CaptureCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandCapture, command.UserID, command.Amount.Int64(), command.HoldTransactionID, metadata)
}

func executeRelease(ctx context.Context, command ReleaseCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandRelease, command.UserID, command.Amount.Int64(), command.HoldTransactionID, metadata)
}

func executeRefund(ctx context.Context, command RefundCommand, execute commandExecutor) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	chargeTransactionID := strings.TrimSpace(command.ChargeTransactionID)
	if err != nil || command.UserID <= 0 || chargeTransactionID == "" || len(chargeTransactionID) > 64 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return execute(ctx, CommandRefund, command.UserID, command.Amount.Int64(), chargeTransactionID, metadata)
}

func isValidAmount(amount CreditAmount) bool {
	return amount > 0 && amount.Int64() <= MaxCreditAmount
}
