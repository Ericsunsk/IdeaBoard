package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	backend Backend
}

type Backend struct {
	ExecuteCreditCommand               func(context.Context, pgx.Tx, domain.CommandType, int64, int64, string, domain.CommandMetadata) (ports.CommandResult, error)
	ExecuteCreditPurchase              func(context.Context, pgx.Tx, int64, int64, domain.CommandMetadata) (ports.CommandResult, error)
	ExecuteCreditPurchaseRefundHold    func(context.Context, pgx.Tx, int64, int64, domain.CommandMetadata) (ports.CommandResult, error)
	ExecuteCreditPurchaseRefund        func(context.Context, pgx.Tx, int64, string, domain.CommandMetadata) (ports.CommandResult, error)
	ExecuteCreditPaymentDisputeDebit   func(context.Context, pgx.Tx, int64, int64, domain.CommandMetadata) (ports.PaymentDisputeDebitResult, error)
	ExecuteCreditPaymentDisputeRestore func(context.Context, pgx.Tx, int64, string, int64, domain.CommandMetadata) (ports.CommandResult, error)
}

func NewService(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits commands: %w", err)
	}
	if !backendComplete(backend) {
		return nil, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func backendComplete(backend Backend) bool {
	return backend.ExecuteCreditCommand != nil &&
		backend.ExecuteCreditPurchase != nil &&
		backend.ExecuteCreditPurchaseRefundHold != nil &&
		backend.ExecuteCreditPurchaseRefund != nil &&
		backend.ExecuteCreditPaymentDisputeDebit != nil &&
		backend.ExecuteCreditPaymentDisputeRestore != nil
}

type transactionCommands struct {
	tx      pgx.Tx
	backend Backend
}

func NewTransactionCommands(tx pgx.Tx, backend Backend) ports.Commands {
	return &transactionCommands{tx: tx, backend: backend}
}

// ManagedCommands is the single pool-owned entry point for Command
// execution. It runs run inside one managed (or ambient guard-carried)
// transaction and returns its result. Role faces in ports select which
// methods callers may use; this runner carries no per-Command knowledge.
func ManagedCommands[T any](ctx context.Context, s *Service, run func(ports.Commands) (T, error)) (T, error) {
	return withManagedTransactionResult(ctx, s, func(commands *transactionCommands) (T, error) {
		return run(commands)
	})
}

func withManagedTransactionResult[T any](
	ctx context.Context,
	service *Service,
	run func(*transactionCommands) (T, error),
) (T, error) {
	var empty T
	if service == nil || service.pool == nil {
		return empty, fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)
	}
	// Ambient guard transaction (control audit atomicity, step 3): when
	// the guard staged its mutation transaction in the context, the
	// business writes join it as participants. The transaction is
	// BORROWED (the guard began it and the guard commits or rolls it
	// back), so this path never begins, commits, or rolls back: it only
	// runs the business statements on the carrier. With no carrier the
	// legacy owned-transaction path below runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return run(&transactionCommands{tx: ambient, backend: service.backend})
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return empty, fmt.Errorf("credits service: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dbutil.RollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credits transaction", "err", rollbackErr)
		}
	}()
	result, err := run(&transactionCommands{tx: tx, backend: service.backend})
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, fmt.Errorf("credits service: commit transaction: %w", err)
	}
	committed = true
	return result, nil
}

func (c *transactionCommands) Grant(ctx context.Context, command ports.GrantCommand) (ports.CommandResult, error) {
	return executeGrant(ctx, command, c.execute)
}

func (c *transactionCommands) Purchase(ctx context.Context, command ports.PurchaseCommand) (ports.CommandResult, error) {
	return executePurchase(ctx, command, c.executePurchase)
}

func (c *transactionCommands) Deduct(ctx context.Context, command ports.DeductionCommand) (ports.CommandResult, error) {
	return executeDeduction(ctx, command, c.execute)
}

func (c *transactionCommands) Correct(ctx context.Context, command ports.CorrectionCommand) (ports.CommandResult, error) {
	return executeCorrection(ctx, command, c.execute)
}

func (c *transactionCommands) Hold(ctx context.Context, command ports.HoldCommand) (ports.CommandResult, error) {
	return executeHold(ctx, command, c.execute)
}

func (c *transactionCommands) PurchaseRefundHold(ctx context.Context, command ports.PurchaseRefundHoldCommand) (ports.CommandResult, error) {
	return executePurchaseRefundHold(ctx, command, c.executePurchaseRefundHold)
}

func (c *transactionCommands) PurchaseRefund(ctx context.Context, command ports.PurchaseRefundCommand) (ports.CommandResult, error) {
	return executePurchaseRefund(ctx, command, c.executePurchaseRefund)
}

func (c *transactionCommands) PaymentDisputeDebit(ctx context.Context, command ports.PaymentDisputeDebitCommand) (ports.PaymentDisputeDebitResult, error) {
	return executePaymentDisputeDebit(ctx, command, c.executePaymentDisputeDebit)
}

func (c *transactionCommands) PaymentDisputeRestore(ctx context.Context, command ports.PaymentDisputeRestoreCommand) (ports.CommandResult, error) {
	return executePaymentDisputeRestore(ctx, command, c.executePaymentDisputeRestore)
}

func (c *transactionCommands) Capture(ctx context.Context, command ports.CaptureCommand) (ports.CommandResult, error) {
	return executeCapture(ctx, command, c.execute)
}

func (c *transactionCommands) Release(ctx context.Context, command ports.ReleaseCommand) (ports.CommandResult, error) {
	return executeRelease(ctx, command, c.execute)
}

func (c *transactionCommands) Refund(ctx context.Context, command ports.RefundCommand) (ports.CommandResult, error) {
	return executeRefund(ctx, command, c.execute)
}

type commandExecutor func(context.Context, domain.CommandType, int64, int64, string, domain.CommandMetadata) (ports.CommandResult, error)
type purchaseExecutor func(context.Context, int64, int64, domain.CommandMetadata) (ports.CommandResult, error)
type purchaseRefundHoldExecutor func(context.Context, int64, int64, domain.CommandMetadata) (ports.CommandResult, error)
type purchaseRefundExecutor func(context.Context, int64, string, domain.CommandMetadata) (ports.CommandResult, error)
type paymentDisputeDebitExecutor func(context.Context, int64, int64, domain.CommandMetadata) (ports.PaymentDisputeDebitResult, error)
type paymentDisputeRestoreExecutor func(context.Context, int64, string, int64, domain.CommandMetadata) (ports.CommandResult, error)

func (c *transactionCommands) execute(ctx context.Context, commandType domain.CommandType, billingAccountID, amount int64, relatedID string, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.CommandResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditCommand(ctx, c.tx, commandType, billingAccountID, amount, relatedID, metadata)
}

func (c *transactionCommands) executePurchase(ctx context.Context, billingAccountID, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.CommandResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditPurchase(ctx, c.tx, billingAccountID, amount, metadata)
}

func (c *transactionCommands) executePaymentDisputeDebit(ctx context.Context, billingAccountID, requestedAmount int64, metadata domain.CommandMetadata) (ports.PaymentDisputeDebitResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.PaymentDisputeDebitResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditPaymentDisputeDebit(ctx, c.tx, billingAccountID, requestedAmount, metadata)
}

func (c *transactionCommands) executePaymentDisputeRestore(ctx context.Context, billingAccountID int64, originalTransactionID string, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.CommandResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditPaymentDisputeRestore(ctx, c.tx, billingAccountID, originalTransactionID, amount, metadata)
}

func (c *transactionCommands) executePurchaseRefundHold(ctx context.Context, billingAccountID, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.CommandResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditPurchaseRefundHold(ctx, c.tx, billingAccountID, amount, metadata)
}

func (c *transactionCommands) executePurchaseRefund(ctx context.Context, billingAccountID int64, holdID string, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	if c == nil || !backendComplete(c.backend) {
		return ports.CommandResult{}, fmt.Errorf("credits commands: persistence backend not configured")
	}
	return c.backend.ExecuteCreditPurchaseRefund(ctx, c.tx, billingAccountID, holdID, metadata)
}

func NormalizeMetadata(metadata domain.CommandMetadata) (domain.CommandMetadata, error) {
	metadata.IdempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)
	metadata.Actor.Type = domain.ActorType(strings.TrimSpace(string(metadata.Actor.Type)))
	metadata.Actor.ID = strings.TrimSpace(metadata.Actor.ID)
	metadata.Source = strings.TrimSpace(metadata.Source)
	metadata.Reason = strings.TrimSpace(metadata.Reason)
	if metadata.Actor.Type == domain.ActorAdmin {
		metadata.Reason = ""
	}
	metadata.BusinessReference.Type = strings.TrimSpace(metadata.BusinessReference.Type)
	metadata.BusinessReference.ID = strings.TrimSpace(metadata.BusinessReference.ID)
	if metadata.IdempotencyKey == "" || len(metadata.IdempotencyKey) > 160 ||
		metadata.Actor.Type == "" || metadata.Actor.ID == "" || len(metadata.Actor.ID) > 128 ||
		metadata.Source == "" || len(metadata.Source) > 64 || (metadata.Actor.Type != domain.ActorAdmin && metadata.Reason == "") ||
		metadata.BusinessReference.Type == "" || len(metadata.BusinessReference.Type) > 64 ||
		metadata.BusinessReference.ID == "" || len(metadata.BusinessReference.ID) > 160 {
		return domain.CommandMetadata{}, domain.ErrInvalidCommand
	}
	switch metadata.Actor.Type {
	case domain.ActorAdmin, domain.ActorSystem, domain.ActorService, domain.ActorUser:
	default:
		return domain.CommandMetadata{}, domain.ErrInvalidCommand
	}
	return metadata, nil
}

func executeGrant(ctx context.Context, command ports.GrantCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandGrant, command.BillingAccountID, command.Amount.Int64(), "", metadata)
}

func executePurchase(ctx context.Context, command ports.PurchaseCommand, execute purchaseExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != domain.ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_purchase" || metadata.BusinessReference.Type != "payment_order" {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, command.BillingAccountID, command.Amount.Int64(), metadata)
}

func executePurchaseRefund(ctx context.Context, command ports.PurchaseRefundCommand, execute purchaseRefundExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	holdID := strings.TrimSpace(command.HoldTransactionID)
	if err != nil || command.BillingAccountID <= 0 || holdID == "" || len(holdID) > 64 ||
		metadata.Actor.Type != domain.ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_refund" || metadata.BusinessReference.Type != "payment_refund" {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, command.BillingAccountID, holdID, metadata)
}

func executePurchaseRefundHold(ctx context.Context, command ports.PurchaseRefundHoldCommand, execute purchaseRefundHoldExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != domain.ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_refund_hold" || metadata.BusinessReference.Type != "payment_refund" {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, command.BillingAccountID, command.Amount.Int64(), metadata)
}

func executePaymentDisputeDebit(ctx context.Context, command ports.PaymentDisputeDebitCommand, execute paymentDisputeDebitExecutor) (ports.PaymentDisputeDebitResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || !isValidAmount(command.RequestedAmount) ||
		metadata.Actor.Type != domain.ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_dispute" || metadata.BusinessReference.Type != "payment_dispute" {
		return ports.PaymentDisputeDebitResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, command.BillingAccountID, command.RequestedAmount.Int64(), metadata)
}

func executePaymentDisputeRestore(ctx context.Context, command ports.PaymentDisputeRestoreCommand, execute paymentDisputeRestoreExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	originalID := strings.TrimSpace(command.OriginalTransactionID)
	if err != nil || command.BillingAccountID <= 0 || originalID == "" || len(originalID) > 64 || !isValidAmount(command.Amount) ||
		metadata.Actor.Type != domain.ActorService || metadata.Actor.ID != "payments" ||
		metadata.Source != "stripe_dispute" || metadata.BusinessReference.Type != "payment_dispute" {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, command.BillingAccountID, originalID, command.Amount.Int64(), metadata)
}

func executeDeduction(ctx context.Context, command ports.DeductionCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || metadata.Actor.Type != domain.ActorAdmin || command.BillingAccountID <= 0 || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandDeduction, command.BillingAccountID, command.Amount.Int64(), "", metadata)
}

func executeCorrection(ctx context.Context, command ports.CorrectionCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	originalTransactionID := strings.TrimSpace(command.OriginalTransactionID)
	if err != nil || metadata.Actor.Type != domain.ActorAdmin || command.BillingAccountID <= 0 || originalTransactionID == "" || len(originalTransactionID) > 64 || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandCorrection, command.BillingAccountID, command.Amount.Int64(), originalTransactionID, metadata)
}

func executeHold(ctx context.Context, command ports.HoldCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandHold, command.BillingAccountID, command.Amount.Int64(), "", metadata)
}

func executeCapture(ctx context.Context, command ports.CaptureCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandCapture, command.BillingAccountID, command.Amount.Int64(), command.HoldTransactionID, metadata)
}

func executeRelease(ctx context.Context, command ports.ReleaseCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	if err != nil || command.BillingAccountID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandRelease, command.BillingAccountID, command.Amount.Int64(), command.HoldTransactionID, metadata)
}

func executeRefund(ctx context.Context, command ports.RefundCommand, execute commandExecutor) (ports.CommandResult, error) {
	metadata, err := NormalizeMetadata(command.Metadata)
	chargeTransactionID := strings.TrimSpace(command.ChargeTransactionID)
	if err != nil || command.BillingAccountID <= 0 || chargeTransactionID == "" || len(chargeTransactionID) > 64 || !isValidAmount(command.Amount) {
		return ports.CommandResult{}, domain.ErrInvalidCommand
	}
	return execute(ctx, domain.CommandRefund, command.BillingAccountID, command.Amount.Int64(), chargeTransactionID, metadata)
}

func isValidAmount(amount domain.CreditAmount) bool {
	return amount > 0 && amount.Int64() <= domain.MaxCreditAmount
}
