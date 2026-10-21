package credits

import (
	"context"

	creditsservice "ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	usersports "ideaboard/api/internal/identity/users/ports"

	"github.com/jackc/pgx/v5"
)

// Adapter translates the Credits lifecycle contract into the Identity-owned seam.
type Adapter struct {
	service *creditsservice.Service
}

func New(service *creditsservice.Service) *Adapter {
	return &Adapter{service: service}
}

func (a *Adapter) LifecycleInTx(tx pgx.Tx) usersports.WalletLifecycle {
	return &lifecycleAdapter{lifecycle: a.service.LifecycleInTx(tx)}
}

func (a *Adapter) GetWallet(ctx context.Context, billingAccountID int64) (usersports.WalletDetails, error) {
	wallet, err := a.service.GetWallet(ctx, billingAccountID)
	if err != nil {
		return usersports.WalletDetails{}, err
	}
	return usersports.WalletDetails{WalletBalance: usersports.WalletBalance{
		BillingAccountID: wallet.BillingAccountID,
		PublicID:         wallet.PublicID,
		DisplayName:      wallet.DisplayName,
		Available:        wallet.Available,
		Reserved:         wallet.Reserved,
	}}, nil
}

type lifecycleAdapter struct {
	lifecycle creditsports.WalletLifecycle
}

func (a *lifecycleAdapter) BindWelcomeIdentityKey(ctx context.Context, fingerprint string) error {
	return a.lifecycle.BindWelcomeIdentityKey(ctx, fingerprint)
}

func (a *lifecycleAdapter) ResolvePersonalAccount(ctx context.Context, userID int64) (int64, error) {
	return a.lifecycle.ResolvePersonalAccount(ctx, userID)
}

func (a *lifecycleAdapter) ProvisionAccountWallet(ctx context.Context, billingAccountID int64) error {
	return a.lifecycle.ProvisionAccountWallet(ctx, billingAccountID)
}

func (a *lifecycleAdapter) SetAccountWalletFrozen(ctx context.Context, billingAccountID int64, frozen bool) error {
	return a.lifecycle.SetAccountWalletFrozen(ctx, billingAccountID, frozen)
}

func (a *lifecycleAdapter) SetSoleOwnedWalletsFrozen(ctx context.Context, userID int64, frozen bool) error {
	return a.lifecycle.SetSoleOwnedWalletsFrozen(ctx, userID, frozen)
}

func (a *lifecycleAdapter) RemoveSharedBillingMemberships(ctx context.Context, userID int64) error {
	return a.lifecycle.RemoveSharedBillingMemberships(ctx, userID)
}

func (a *lifecycleAdapter) ClaimWelcomeGrant(
	ctx context.Context,
	fingerprint string,
	userID int64,
	command usersports.WelcomeGrantCommand,
) (usersports.WelcomeGrantResult, bool, error) {
	amount, err := creditsdomain.NewCreditAmount(command.Amount)
	if err != nil {
		return usersports.WelcomeGrantResult{}, false, err
	}
	result, granted, err := a.lifecycle.ClaimWelcomeGrant(ctx, fingerprint, command.UserID, creditsports.GrantCommand{
		BillingAccountID: command.BillingAccountID,
		Amount:           amount,
		Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey:    command.IdempotencyKey,
			Actor:             creditsdomain.Actor{Type: creditsdomain.ActorType(command.ActorType), ID: command.ActorID},
			Source:            command.Source,
			Reason:            command.Reason,
			BusinessReference: creditsdomain.BusinessReference{Type: command.BusinessReferenceType, ID: command.BusinessReferenceID},
		},
	})
	if err != nil {
		return usersports.WelcomeGrantResult{}, false, err
	}
	return usersports.WelcomeGrantResult{Wallet: usersports.WalletBalance{
		BillingAccountID: result.Wallet.BillingAccountID,
		PublicID:         result.Wallet.PublicID,
		DisplayName:      result.Wallet.DisplayName,
		Available:        result.Wallet.Available,
		Reserved:         result.Wallet.Reserved,
	}}, granted, nil
}

var _ usersports.CreditWalletLifecycleService = (*Adapter)(nil)
