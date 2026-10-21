package postgres

import (
	"context"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/jackc/pgx/v5"
)

func ResolvePersonalAccount(ctx context.Context, tx pgx.Tx, userID int64) (int64, error) {
	var accountID int64
	err := tx.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID)
	return accountID, err
}

func ProvisionAccountWallet(ctx context.Context, tx pgx.Tx, billingAccountID int64) error {
	_, err := tx.Exec(ctx, `SELECT provision_billing_account_wallet($1)`, billingAccountID)
	return err
}

func BindIdentityKey(ctx context.Context, tx pgx.Tx, fingerprint string) error {
	_, err := tx.Exec(ctx, `SELECT bind_credit_welcome_identity_key($1)`, fingerprint)
	return err
}

func SetAccountWalletFrozen(ctx context.Context, tx pgx.Tx, billingAccountID int64, frozen bool) error {
	status := domain.WalletStatusActive
	if frozen {
		status = domain.WalletStatusFrozen
	}
	_, err := tx.Exec(ctx, `SELECT set_billing_account_wallet_status($1, $2)`, billingAccountID, string(status))
	return err
}

func SetSoleOwnedWalletsFrozen(ctx context.Context, tx pgx.Tx, userID int64, frozen bool) error {
	_, err := tx.Exec(ctx, `SELECT freeze_sole_owned_billing_wallets($1, $2)`, userID, frozen)
	return err
}

func RemoveSharedBillingMembership(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, `SELECT remove_shared_billing_memberships_for_user($1)`, userID)
	return err
}

func ClaimWelcome(ctx context.Context, tx pgx.Tx, fingerprint string, userID int64) (bool, error) {
	var claimed bool
	err := tx.QueryRow(ctx, `SELECT claim_credit_welcome($1, $2)`, fingerprint, userID).Scan(&claimed)
	return claimed, err
}

func ExecuteGrant(ctx context.Context, tx pgx.Tx, billingAccountID, amount int64, metadata domain.CommandMetadata) (ports.CommandResult, error) {
	return ExecuteCreditCommand(ctx, tx, domain.CommandGrant, billingAccountID, amount, "", metadata)
}

func RestrictWallet(ctx context.Context, tx pgx.Tx, command ports.RestrictWalletCommand) (ports.WalletRestrictionChange, error) {
	var result ports.WalletRestrictionChange
	err := tx.QueryRow(ctx, `
SELECT restriction_id, replayed
FROM create_credit_wallet_restriction($1, $2, $3, $4, $5, $6, $7, $8)`,
		command.BillingAccountID, command.Type, command.Source, command.RelatedObject.Type,
		command.RelatedObject.ID, command.Reason, command.Actor.Type, command.Actor.ID,
	).Scan(&result.RestrictionID, &result.Replayed)
	return result, err
}

func ReleaseWallet(ctx context.Context, tx pgx.Tx, command ports.ReleaseWalletRestrictionCommand) (ports.WalletRestrictionChange, error) {
	var result ports.WalletRestrictionChange
	err := tx.QueryRow(ctx, `
SELECT restriction_id, replayed
FROM release_credit_wallet_restriction($1, $2, $3, $4)`,
		command.RestrictionID, command.Actor.Type, command.Actor.ID, command.Reason,
	).Scan(&result.RestrictionID, &result.Replayed)
	return result, err
}
