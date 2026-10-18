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
	UserID        int64
	Type          string
	Source        string
	RelatedObject domain.BusinessReference
	Reason        string
	Actor         domain.Actor
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
type WalletLifecycle interface {
	BindWelcomeIdentityKey(context.Context, string) error
	ProvisionWallet(context.Context, int64) error
	SetWalletFrozen(context.Context, int64, bool) error
	ClaimWelcomeGrant(context.Context, string, GrantCommand) (CommandResult, bool, error)
}

// merged from internal/credits/ports/wallet_reader.go (structural consolidation; symbols unchanged).
type WalletReader interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
	ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error)
}
