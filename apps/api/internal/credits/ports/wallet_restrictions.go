package ports

import (
	"context"
	"ideaboard/api/internal/credits/domain"
)

type WalletRestrictionCommands interface {
	RestrictWallet(context.Context, RestrictWalletCommand) (WalletRestrictionChange, error)
	ReleaseWalletRestriction(context.Context, ReleaseWalletRestrictionCommand) (WalletRestrictionChange, error)
}

type RestrictWalletCommand struct {
	BillingAccountID int64
	Type             string
	Source           string
	RelatedObject    domain.BusinessReference
	Reason           string
	Actor            domain.Actor
}

type ReleaseWalletRestrictionCommand struct {
	RestrictionID string
	Reason        string
	Actor         domain.Actor
}

type WalletRestrictionChange struct {
	RestrictionID string `json:"restriction_id"`
	Replayed      bool   `json:"replayed"`
}

// merged from internal/credits/ports/wallet_lifecycle.go (structural consolidation; symbols unchanged).
// The Wallet belongs to exactly one Billing Account (#916); userID-style
// wallet identity is gone. Actor identity travels on each Command's Metadata.
type WalletLifecycle interface {
	BindWelcomeIdentityKey(context.Context, string) error
	// ResolvePersonalAccount ensures and returns the caller's deterministic
	// initial personal Billing Account (Default-workspace account).
	ResolvePersonalAccount(context.Context, int64) (int64, error)
	ProvisionAccountWallet(context.Context, int64) error
	SetAccountWalletFrozen(context.Context, int64, bool) error
	// SetSoleOwnedWalletsFrozen freezes only non-organization accounts the
	// user solely owns; shared/org funds never freeze on ban/delete.
	SetSoleOwnedWalletsFrozen(context.Context, int64, bool) error
	// RemoveSharedBillingMemberships detaches a deleted user from accounts
	// that keep other managers.
	RemoveSharedBillingMemberships(context.Context, int64) error
	ClaimWelcomeGrant(context.Context, string, int64, GrantCommand) (CommandResult, bool, error)
}

// merged from internal/credits/ports/wallet_reader.go (structural consolidation; symbols unchanged).
type WalletReader interface {
	// ResolvePersonalAccount ensures and returns the deterministic initial
	// personal Billing Account for one user (#916).
	ResolvePersonalAccount(context.Context, int64) (int64, error)
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
	ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error)
}
