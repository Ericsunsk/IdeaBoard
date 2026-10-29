package wallet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/credits/application/commands"
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
	MapDatabaseError              func(error) error
	ResolvePersonalAccount        func(context.Context, pgx.Tx, int64) (int64, error)
	ProvisionAccountWallet        func(context.Context, pgx.Tx, int64) error
	BindIdentityKey               func(context.Context, pgx.Tx, string) error
	SetAccountWalletFrozen        func(context.Context, pgx.Tx, int64, bool) error
	SetSoleOwnedWalletsFrozen     func(context.Context, pgx.Tx, int64, bool) error
	RemoveSharedBillingMembership func(context.Context, pgx.Tx, int64) error
	ClaimWelcome                  func(context.Context, pgx.Tx, string, int64) (bool, error)
	ExecuteGrant                  func(context.Context, pgx.Tx, int64, int64, domain.CommandMetadata) (ports.CommandResult, error)
	GetWallet                     func(context.Context, pgx.Tx, int64) (domain.WalletDetails, error)
	RestrictWallet                func(context.Context, pgx.Tx, ports.RestrictWalletCommand) (ports.WalletRestrictionChange, error)
	ReleaseWallet                 func(context.Context, pgx.Tx, ports.ReleaseWalletRestrictionCommand) (ports.WalletRestrictionChange, error)
}

func NewService(pool *pgxpool.Pool, backend Backend) (*Service, error) {
	if err := database.RequirePool(pool); err != nil {
		return nil, fmt.Errorf("credits wallet: %w", err)
	}
	if !backendComplete(backend) {
		return nil, fmt.Errorf("credits wallet: persistence backend not configured")
	}
	return &Service{pool: pool, backend: backend}, nil
}

func backendComplete(backend Backend) bool {
	return backend.MapDatabaseError != nil && backend.ResolvePersonalAccount != nil && backend.ProvisionAccountWallet != nil &&
		backend.BindIdentityKey != nil && backend.SetAccountWalletFrozen != nil && backend.SetSoleOwnedWalletsFrozen != nil &&
		backend.RemoveSharedBillingMembership != nil && backend.ClaimWelcome != nil && backend.ExecuteGrant != nil &&
		backend.GetWallet != nil &&
		backend.RestrictWallet != nil && backend.ReleaseWallet != nil
}

type transactionLifecycle struct {
	tx      pgx.Tx
	backend Backend
}

func NewLifecycleTransaction(tx pgx.Tx, backend Backend) ports.WalletLifecycle {
	return &transactionLifecycle{tx: tx, backend: backend}
}

func NewRestrictionTransaction(tx pgx.Tx, backend Backend) ports.WalletRestrictionCommands {
	return &transactionLifecycle{tx: tx, backend: backend}
}

func (s *Service) RestrictWallet(
	ctx context.Context,
	command ports.RestrictWalletCommand,
) (ports.WalletRestrictionChange, error) {
	return withManagedRestrictionTransaction(ctx, s, func(commands *transactionLifecycle) (ports.WalletRestrictionChange, error) {
		return commands.RestrictWallet(ctx, command)
	})
}

func (s *Service) ReleaseWalletRestriction(
	ctx context.Context,
	command ports.ReleaseWalletRestrictionCommand,
) (ports.WalletRestrictionChange, error) {
	return withManagedRestrictionTransaction(ctx, s, func(commands *transactionLifecycle) (ports.WalletRestrictionChange, error) {
		return commands.ReleaseWalletRestriction(ctx, command)
	})
}

func (c *transactionLifecycle) ResolvePersonalAccount(ctx context.Context, userID int64) (int64, error) {
	if c == nil || c.tx == nil || userID <= 0 {
		return 0, domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return 0, fmt.Errorf("credits wallet: persistence backend not configured")
	}
	accountID, err := c.backend.ResolvePersonalAccount(ctx, c.tx, userID)
	if err != nil {
		return 0, c.backend.MapDatabaseError(err)
	}
	if accountID <= 0 {
		return 0, domain.ErrWalletNotFound
	}
	return accountID, nil
}

func (c *transactionLifecycle) ProvisionAccountWallet(ctx context.Context, billingAccountID int64) error {
	if c == nil || c.tx == nil || billingAccountID <= 0 {
		return domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return fmt.Errorf("credits wallet: persistence backend not configured")
	}
	if err := c.backend.ProvisionAccountWallet(ctx, c.tx, billingAccountID); err != nil {
		return c.backend.MapDatabaseError(err)
	}
	return nil
}

func (c *transactionLifecycle) BindWelcomeIdentityKey(ctx context.Context, keyFingerprint string) error {
	keyFingerprint = strings.TrimSpace(keyFingerprint)
	if c == nil || c.tx == nil || len(keyFingerprint) != 64 {
		return domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return fmt.Errorf("credits wallet: persistence backend not configured")
	}
	if err := c.backend.BindIdentityKey(ctx, c.tx, keyFingerprint); err != nil {
		return c.backend.MapDatabaseError(err)
	}
	return nil
}

func (c *transactionLifecycle) SetAccountWalletFrozen(ctx context.Context, billingAccountID int64, frozen bool) error {
	if c == nil || c.tx == nil || billingAccountID <= 0 {
		return domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return fmt.Errorf("credits wallet: persistence backend not configured")
	}
	if err := c.backend.SetAccountWalletFrozen(ctx, c.tx, billingAccountID, frozen); err != nil {
		return c.backend.MapDatabaseError(err)
	}
	return nil
}

func (c *transactionLifecycle) SetSoleOwnedWalletsFrozen(ctx context.Context, userID int64, frozen bool) error {
	if c == nil || c.tx == nil || userID <= 0 {
		return domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return fmt.Errorf("credits wallet: persistence backend not configured")
	}
	if err := c.backend.SetSoleOwnedWalletsFrozen(ctx, c.tx, userID, frozen); err != nil {
		return c.backend.MapDatabaseError(err)
	}
	return nil
}

func (c *transactionLifecycle) RemoveSharedBillingMemberships(ctx context.Context, userID int64) error {
	if c == nil || c.tx == nil || userID <= 0 {
		return domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return fmt.Errorf("credits wallet: persistence backend not configured")
	}
	if err := c.backend.RemoveSharedBillingMembership(ctx, c.tx, userID); err != nil {
		return c.backend.MapDatabaseError(err)
	}
	return nil
}

func (c *transactionLifecycle) ClaimWelcomeGrant(
	ctx context.Context,
	identityFingerprint string,
	userID int64,
	command ports.GrantCommand,
) (ports.CommandResult, bool, error) {
	identityFingerprint = strings.TrimSpace(identityFingerprint)
	if c == nil || c.tx == nil || len(identityFingerprint) != 64 || userID <= 0 || command.BillingAccountID <= 0 {
		return ports.CommandResult{}, false, domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return ports.CommandResult{}, false, fmt.Errorf("credits wallet: persistence backend not configured")
	}
	var claimed bool
	claimed, err := c.backend.ClaimWelcome(ctx, c.tx, identityFingerprint, userID)
	if err != nil {
		return ports.CommandResult{}, false, c.backend.MapDatabaseError(err)
	}
	if !claimed {
		return ports.CommandResult{}, false, nil
	}
	result, err := c.grantInTx(ctx, command)
	if err != nil {
		return ports.CommandResult{}, false, err
	}
	return result, true, nil
}

// GetWallet reads the Wallet projection on the caller's transaction. It
// returns the backend read unchanged (no command-error mapping), matching
// the pool-owned read path, so ErrWalletNotFound and fail-closed behavior
// are identical in both shapes.
func (c *transactionLifecycle) GetWallet(ctx context.Context, billingAccountID int64) (domain.WalletDetails, error) {
	if c == nil || c.tx == nil || billingAccountID <= 0 {
		return domain.WalletDetails{}, domain.ErrInvalidCommand
	}
	if !backendComplete(c.backend) {
		return domain.WalletDetails{}, fmt.Errorf("credits wallet: persistence backend not configured")
	}
	return c.backend.GetWallet(ctx, c.tx, billingAccountID)
}

func (c *transactionLifecycle) grantInTx(ctx context.Context, command ports.GrantCommand) (ports.CommandResult, error) {
	metadata, err := normalizeGrantForWelcome(command)
	if err != nil {
		return ports.CommandResult{}, err
	}
	return c.backend.ExecuteGrant(ctx, c.tx, command.BillingAccountID, command.Amount.Int64(), metadata)
}

func normalizeGrantForWelcome(command ports.GrantCommand) (domain.CommandMetadata, error) {
	if command.BillingAccountID <= 0 || command.Amount <= 0 || command.Amount.Int64() > domain.MaxCreditAmount {
		return domain.CommandMetadata{}, domain.ErrInvalidCommand
	}
	return commands.NormalizeMetadata(command.Metadata)
}

func (c *transactionLifecycle) RestrictWallet(
	ctx context.Context,
	command ports.RestrictWalletCommand,
) (ports.WalletRestrictionChange, error) {
	command, err := normalizeRestrictWalletCommand(command)
	if err != nil {
		return ports.WalletRestrictionChange{}, err
	}
	if c == nil || c.tx == nil {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits restrictions: transaction not configured")
	}
	if !backendComplete(c.backend) {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits wallet: persistence backend not configured")
	}

	var result ports.WalletRestrictionChange
	result, err = c.backend.RestrictWallet(ctx, c.tx, command)
	if err != nil {
		return ports.WalletRestrictionChange{}, c.backend.MapDatabaseError(err)
	}
	return result, nil
}

func (c *transactionLifecycle) ReleaseWalletRestriction(
	ctx context.Context,
	command ports.ReleaseWalletRestrictionCommand,
) (ports.WalletRestrictionChange, error) {
	command, err := normalizeReleaseWalletRestrictionCommand(command)
	if err != nil {
		return ports.WalletRestrictionChange{}, err
	}
	if c == nil || c.tx == nil {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits restrictions: transaction not configured")
	}
	if !backendComplete(c.backend) {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits wallet: persistence backend not configured")
	}

	var result ports.WalletRestrictionChange
	result, err = c.backend.ReleaseWallet(ctx, c.tx, command)
	if err != nil {
		return ports.WalletRestrictionChange{}, c.backend.MapDatabaseError(err)
	}
	return result, nil
}

func withManagedRestrictionTransaction(
	ctx context.Context,
	service *Service,
	run func(*transactionLifecycle) (ports.WalletRestrictionChange, error),
) (ports.WalletRestrictionChange, error) {
	if service == nil || service.pool == nil {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits restrictions: %w", database.ErrPoolNotConfigured)
	}
	// Ambient guard transaction (control audit atomicity, step 3): same
	// borrower contract as the commands path above. The restriction
	// writes join the guard-owned transaction; the guard alone commits
	// or rolls it back, so this path never begins, commits, or rolls
	// back. With no carrier the legacy owned-transaction path below
	// runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return run(&transactionLifecycle{tx: ambient, backend: service.backend})
	}
	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits restrictions: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dbutil.RollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back Credit Wallet restriction transaction", "err", rollbackErr)
		}
	}()

	result, err := run(&transactionLifecycle{tx: tx, backend: service.backend})
	if err != nil {
		return ports.WalletRestrictionChange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ports.WalletRestrictionChange{}, fmt.Errorf("credits restrictions: commit transaction: %w", err)
	}
	committed = true
	return result, nil
}

func normalizeRestrictWalletCommand(command ports.RestrictWalletCommand) (ports.RestrictWalletCommand, error) {
	command.Type = strings.TrimSpace(command.Type)
	command.Source = strings.TrimSpace(command.Source)
	command.RelatedObject.Type = strings.TrimSpace(command.RelatedObject.Type)
	command.RelatedObject.ID = strings.TrimSpace(command.RelatedObject.ID)
	command.Reason = strings.TrimSpace(command.Reason)
	command.Actor.Type = domain.ActorType(strings.TrimSpace(string(command.Actor.Type)))
	command.Actor.ID = strings.TrimSpace(command.Actor.ID)
	if command.Actor.Type == domain.ActorAdmin {
		command.Reason = ""
	}
	if command.BillingAccountID <= 0 || command.Type == "" || len(command.Type) > 64 ||
		command.Source == "" || len(command.Source) > 64 ||
		command.RelatedObject.Type == "" || len(command.RelatedObject.Type) > 64 ||
		command.RelatedObject.ID == "" || len(command.RelatedObject.ID) > 160 ||
		(command.Actor.Type != domain.ActorAdmin && command.Reason == "") || len(command.Reason) > 1000 ||
		!validRestrictionActor(command.Actor) {
		return ports.RestrictWalletCommand{}, domain.ErrInvalidCommand
	}
	return command, nil
}

func normalizeReleaseWalletRestrictionCommand(
	command ports.ReleaseWalletRestrictionCommand,
) (ports.ReleaseWalletRestrictionCommand, error) {
	command.RestrictionID = strings.TrimSpace(command.RestrictionID)
	command.Reason = strings.TrimSpace(command.Reason)
	command.Actor.Type = domain.ActorType(strings.TrimSpace(string(command.Actor.Type)))
	command.Actor.ID = strings.TrimSpace(command.Actor.ID)
	if command.Actor.Type == domain.ActorAdmin {
		command.Reason = ""
	}
	if len(command.RestrictionID) != 36 || (command.Actor.Type != domain.ActorAdmin && command.Reason == "") || len(command.Reason) > 1000 ||
		!validRestrictionActor(command.Actor) {
		return ports.ReleaseWalletRestrictionCommand{}, domain.ErrInvalidCommand
	}
	return command, nil
}

func validRestrictionActor(actor domain.Actor) bool {
	if actor.ID == "" || len(actor.ID) > 128 {
		return false
	}
	switch actor.Type {
	case domain.ActorAdmin, domain.ActorSystem, domain.ActorService:
		return true
	default:
		return false
	}
}
