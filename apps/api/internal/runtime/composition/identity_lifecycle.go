package composition

import (
	"context"

	"ideaboard/api/internal/identity/users/application/lifecycle"
	"ideaboard/api/internal/identity/users/application/provisioning"
	"ideaboard/api/internal/identity/users/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// identityLifecycle composes the Identity application services used by runtime wiring.
type identityLifecycle struct {
	provisioning *provisioning.Service
	lifecycle    *lifecycle.Service
}

func newIdentityLifecycle(
	pool *pgxpool.Pool,
	credits ports.CreditWalletLifecycleService,
	welcomeIdentityKey []byte,
) *identityLifecycle {
	return &identityLifecycle{
		provisioning: provisioning.NewService(pool, credits, welcomeIdentityKey),
		lifecycle:    lifecycle.NewService(pool, credits),
	}
}

func (s *identityLifecycle) VerifyWelcomeIdentityKey(ctx context.Context) error {
	return s.provisioning.VerifyWelcomeIdentityKey(ctx)
}

func (s *identityLifecycle) ProvisionIdentity(ctx context.Context, identity ports.VerifiedIdentity) (*ports.ProvisionResult, error) {
	return s.provisioning.ProvisionIdentity(ctx, identity)
}

func (s *identityLifecycle) InitializeExistingIdentities(ctx context.Context, identities []ports.VerifiedIdentity) ([]ports.ProvisionResult, error) {
	return s.provisioning.InitializeExistingIdentities(ctx, identities)
}

func (s *identityLifecycle) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	return s.lifecycle.SetUserStatus(ctx, userID, status)
}

func (s *identityLifecycle) DeleteUser(
	ctx context.Context,
	userID int64,
	authDeleter ports.AuthUserDeleter,
	uploadCleaner ports.UserUploadCleaner,
) error {
	return s.lifecycle.DeleteUser(ctx, userID, authDeleter, uploadCleaner)
}
