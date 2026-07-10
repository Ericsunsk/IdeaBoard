package credits

import (
	"context"
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
	pool *pgxpool.Pool
}

type transactionCommands struct {
	tx pgx.Tx
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// InTx lets Tasking compose Credits commands into its existing PostgreSQL
// transaction. The caller owns commit and rollback.
func (s *Service) InTx(tx pgx.Tx) Commands {
	return &transactionCommands{tx: tx}
}

func (s *Service) Grant(ctx context.Context, command GrantCommand) (CommandResult, error) {
	return withManagedTransaction(ctx, s, func(commands *transactionCommands) (CommandResult, error) {
		return commands.Grant(ctx, command)
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

func (c *transactionCommands) Grant(ctx context.Context, command GrantCommand) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return executeCreditCommand(ctx, c.tx, CommandGrant, command.UserID, command.Amount.Int64(), "", metadata)
}

func (c *transactionCommands) Hold(ctx context.Context, command HoldCommand) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return executeCreditCommand(ctx, c.tx, CommandHold, command.UserID, command.Amount.Int64(), "", metadata)
}

func (c *transactionCommands) Capture(ctx context.Context, command CaptureCommand) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" {
		return CommandResult{}, ErrInvalidCommand
	}
	return executeCreditCommand(ctx, c.tx, CommandCapture, command.UserID, 0, command.HoldTransactionID, metadata)
}

func (c *transactionCommands) Release(ctx context.Context, command ReleaseCommand) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || strings.TrimSpace(command.HoldTransactionID) == "" {
		return CommandResult{}, ErrInvalidCommand
	}
	return executeCreditCommand(ctx, c.tx, CommandRelease, command.UserID, 0, command.HoldTransactionID, metadata)
}

func (c *transactionCommands) Refund(ctx context.Context, command RefundCommand) (CommandResult, error) {
	metadata, err := normalizeMetadata(command.Metadata)
	if err != nil || command.UserID <= 0 || strings.TrimSpace(command.ChargeTransactionID) == "" || !isValidAmount(command.Amount) {
		return CommandResult{}, ErrInvalidCommand
	}
	return executeCreditCommand(ctx, c.tx, CommandRefund, command.UserID, command.Amount.Int64(), command.ChargeTransactionID, metadata)
}

func isValidAmount(amount CreditAmount) bool {
	return amount > 0 && amount.Int64() <= MaxCreditAmount
}
