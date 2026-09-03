package ports

import (
	"context"
)

type WalletLifecycle interface {
	BindWelcomeIdentityKey(context.Context, string) error
	ProvisionWallet(context.Context, int64) error
	SetWalletFrozen(context.Context, int64, bool) error
	ClaimWelcomeGrant(context.Context, string, GrantCommand) (CommandResult, bool, error)
}
