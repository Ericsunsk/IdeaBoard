package credits

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) RestrictionsInTx(tx pgx.Tx) WalletRestrictionCommands {
	return &transactionCommands{tx: tx}
}

// RestrictionsInSQLTx lets Payments compose restriction changes with its
// database/sql transaction and the same Credits-owned SECURITY DEFINER seam.
func RestrictionsInSQLTx(tx *sql.Tx) WalletRestrictionCommands {
	return &sqlTransactionCommands{tx: tx}
}

func (s *Service) RestrictWallet(
	ctx context.Context,
	command RestrictWalletCommand,
) (WalletRestrictionChange, error) {
	return withManagedRestrictionTransaction(ctx, s, func(commands *transactionCommands) (WalletRestrictionChange, error) {
		return commands.RestrictWallet(ctx, command)
	})
}

func (s *Service) ReleaseWalletRestriction(
	ctx context.Context,
	command ReleaseWalletRestrictionCommand,
) (WalletRestrictionChange, error) {
	return withManagedRestrictionTransaction(ctx, s, func(commands *transactionCommands) (WalletRestrictionChange, error) {
		return commands.ReleaseWalletRestriction(ctx, command)
	})
}

func (c *transactionCommands) RestrictWallet(
	ctx context.Context,
	command RestrictWalletCommand,
) (WalletRestrictionChange, error) {
	command, err := normalizeRestrictWalletCommand(command)
	if err != nil {
		return WalletRestrictionChange{}, err
	}
	if c == nil || c.tx == nil {
		return WalletRestrictionChange{}, fmt.Errorf("credits restrictions: transaction not configured")
	}

	var result WalletRestrictionChange
	err = c.tx.QueryRow(ctx, `
SELECT restriction_id, replayed
FROM create_credit_wallet_restriction($1, $2, $3, $4, $5, $6, $7, $8)`,
		command.UserID,
		command.Type,
		command.Source,
		command.RelatedObject.Type,
		command.RelatedObject.ID,
		command.Reason,
		command.Actor.Type,
		command.Actor.ID,
	).Scan(&result.RestrictionID, &result.Replayed)
	if err != nil {
		return WalletRestrictionChange{}, mapCreditDatabaseError(err)
	}
	return result, nil
}

func (c *sqlTransactionCommands) RestrictWallet(ctx context.Context, command RestrictWalletCommand) (WalletRestrictionChange, error) {
	command, err := normalizeRestrictWalletCommand(command)
	if err != nil || c == nil || c.tx == nil {
		return WalletRestrictionChange{}, errOrRestrictionTransaction(err)
	}
	var result WalletRestrictionChange
	err = c.tx.QueryRowContext(ctx, `
SELECT restriction_id, replayed
FROM create_credit_wallet_restriction($1, $2, $3, $4, $5, $6, $7, $8)`,
		command.UserID, command.Type, command.Source, command.RelatedObject.Type,
		command.RelatedObject.ID, command.Reason, command.Actor.Type, command.Actor.ID,
	).Scan(&result.RestrictionID, &result.Replayed)
	if err != nil {
		return WalletRestrictionChange{}, mapCreditDatabaseError(err)
	}
	return result, nil
}

func (c *transactionCommands) ReleaseWalletRestriction(
	ctx context.Context,
	command ReleaseWalletRestrictionCommand,
) (WalletRestrictionChange, error) {
	command, err := normalizeReleaseWalletRestrictionCommand(command)
	if err != nil {
		return WalletRestrictionChange{}, err
	}
	if c == nil || c.tx == nil {
		return WalletRestrictionChange{}, fmt.Errorf("credits restrictions: transaction not configured")
	}

	var result WalletRestrictionChange
	err = c.tx.QueryRow(ctx, `
SELECT restriction_id, replayed
FROM release_credit_wallet_restriction($1, $2, $3, $4)`,
		command.RestrictionID,
		command.Actor.Type,
		command.Actor.ID,
		command.Reason,
	).Scan(&result.RestrictionID, &result.Replayed)
	if err != nil {
		return WalletRestrictionChange{}, mapCreditDatabaseError(err)
	}
	return result, nil
}

func (c *sqlTransactionCommands) ReleaseWalletRestriction(ctx context.Context, command ReleaseWalletRestrictionCommand) (WalletRestrictionChange, error) {
	command, err := normalizeReleaseWalletRestrictionCommand(command)
	if err != nil || c == nil || c.tx == nil {
		return WalletRestrictionChange{}, errOrRestrictionTransaction(err)
	}
	var result WalletRestrictionChange
	err = c.tx.QueryRowContext(ctx, `
SELECT restriction_id, replayed
FROM release_credit_wallet_restriction($1, $2, $3, $4)`,
		command.RestrictionID, command.Actor.Type, command.Actor.ID, command.Reason,
	).Scan(&result.RestrictionID, &result.Replayed)
	if err != nil {
		return WalletRestrictionChange{}, mapCreditDatabaseError(err)
	}
	return result, nil
}

func errOrRestrictionTransaction(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("credits restrictions: transaction not configured")
}

func withManagedRestrictionTransaction(
	ctx context.Context,
	service *Service,
	run func(*transactionCommands) (WalletRestrictionChange, error),
) (WalletRestrictionChange, error) {
	if service == nil || service.pool == nil {
		return WalletRestrictionChange{}, fmt.Errorf("credits restrictions: database pool not configured")
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WalletRestrictionChange{}, fmt.Errorf("credits restrictions: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credit Wallet restriction transaction", "err", rollbackErr)
		}
	}()

	result, err := run(&transactionCommands{tx: tx})
	if err != nil {
		return WalletRestrictionChange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WalletRestrictionChange{}, fmt.Errorf("credits restrictions: commit transaction: %w", err)
	}
	committed = true
	return result, nil
}

func normalizeRestrictWalletCommand(command RestrictWalletCommand) (RestrictWalletCommand, error) {
	command.Type = strings.TrimSpace(command.Type)
	command.Source = strings.TrimSpace(command.Source)
	command.RelatedObject.Type = strings.TrimSpace(command.RelatedObject.Type)
	command.RelatedObject.ID = strings.TrimSpace(command.RelatedObject.ID)
	command.Reason = strings.TrimSpace(command.Reason)
	command.Actor.Type = ActorType(strings.TrimSpace(string(command.Actor.Type)))
	command.Actor.ID = strings.TrimSpace(command.Actor.ID)
	if command.UserID <= 0 || command.Type == "" || len(command.Type) > 64 ||
		command.Source == "" || len(command.Source) > 64 ||
		command.RelatedObject.Type == "" || len(command.RelatedObject.Type) > 64 ||
		command.RelatedObject.ID == "" || len(command.RelatedObject.ID) > 160 ||
		command.Reason == "" || len(command.Reason) > 1000 ||
		!validRestrictionActor(command.Actor) {
		return RestrictWalletCommand{}, ErrInvalidCommand
	}
	return command, nil
}

func normalizeReleaseWalletRestrictionCommand(
	command ReleaseWalletRestrictionCommand,
) (ReleaseWalletRestrictionCommand, error) {
	command.RestrictionID = strings.TrimSpace(command.RestrictionID)
	command.Reason = strings.TrimSpace(command.Reason)
	command.Actor.Type = ActorType(strings.TrimSpace(string(command.Actor.Type)))
	command.Actor.ID = strings.TrimSpace(command.Actor.ID)
	if len(command.RestrictionID) != 36 || command.Reason == "" || len(command.Reason) > 1000 ||
		!validRestrictionActor(command.Actor) {
		return ReleaseWalletRestrictionCommand{}, ErrInvalidCommand
	}
	return command, nil
}

func validRestrictionActor(actor Actor) bool {
	if actor.ID == "" || len(actor.ID) > 128 {
		return false
	}
	switch actor.Type {
	case ActorAdmin, ActorSystem, ActorService:
		return true
	default:
		return false
	}
}
