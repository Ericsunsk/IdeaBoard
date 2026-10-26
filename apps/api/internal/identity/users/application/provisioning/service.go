package provisioning

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/identity/trust"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const welcomeGrantCredits int64 = 100

type Service struct {
	pool                  *pgxpool.Pool
	credits               ports.CreditWalletLifecycleService
	welcomeIdentityKey    []byte
	welcomeKeyFingerprint string
}

func NewService(
	pool *pgxpool.Pool,
	creditService ports.CreditWalletLifecycleService,
	welcomeIdentityKey []byte,
) *Service {
	keyDigest := sha256.Sum256(append([]byte("ideaboard:welcome-identity-key:v1:"), welcomeIdentityKey...))
	return &Service{
		pool:                  pool,
		credits:               creditService,
		welcomeIdentityKey:    append([]byte(nil), welcomeIdentityKey...),
		welcomeKeyFingerprint: hex.EncodeToString(keyDigest[:]),
	}
}

func (s *Service) VerifyWelcomeIdentityKey(ctx context.Context) error {
	if s == nil || s.pool == nil || s.credits == nil || len(s.welcomeIdentityKey) != 32 {
		return fmt.Errorf("identity lifecycle dependencies and a 32-byte Welcome Grant key are required")
	}
	scope, err := dbutil.Begin(ctx, s.pool, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin Welcome identity key verification: %w", err)
	}
	if scope.Owned() {
		defer rollbackIdentityTransaction(ctx, scope.Tx(), "Welcome identity key verification")
	}
	if err := s.credits.LifecycleInTx(scope.Tx()).BindWelcomeIdentityKey(ctx, s.welcomeKeyFingerprint); err != nil {
		return fmt.Errorf("verify permanently bound Welcome identity key: %w", err)
	}
	if err := scope.Commit(ctx); err != nil {
		return fmt.Errorf("commit Welcome identity key verification: %w", err)
	}
	return nil
}

func (s *Service) ProvisionIdentity(ctx context.Context, identity ports.VerifiedIdentity) (*ports.ProvisionResult, error) {
	if s == nil {
		return nil, fmt.Errorf("identity lifecycle dependencies are not configured")
	}
	identity, fingerprint, err := s.normalizeVerifiedIdentity(identity)
	if err != nil {
		return nil, err
	}
	if s.pool == nil || s.credits == nil {
		return nil, fmt.Errorf("identity lifecycle dependencies are not configured")
	}

	scope, err := dbutil.Begin(ctx, s.pool, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin identity lifecycle transaction: %w", err)
	}
	if !scope.Owned() {
		user, grant, granted, err := s.provisionIdentityTx(ctx, scope.Tx(), identity, fingerprint)
		if err != nil {
			return nil, err
		}
		if granted {
			return &ports.ProvisionResult{User: *user, Wallet: grant, WelcomeGranted: true}, nil
		}
		return s.provisionResultWithoutGrant(ctx, scope.Tx(), user)
	}

	tx := scope.Tx()
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back identity provisioning transaction", "err", rollbackErr)
		}
	}()

	user, grant, granted, err := s.provisionIdentityTx(ctx, tx, identity, fingerprint)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit identity lifecycle transaction: %w", err)
	}
	wallet := grant
	if !granted {
		readTx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("begin provisioned wallet read: %w", err)
		}
		result, readErr := s.provisionResultWithoutGrant(ctx, readTx, user)
		_ = readTx.Rollback(ctx)
		if readErr != nil {
			return nil, readErr
		}
		return result, nil
	}
	return &ports.ProvisionResult{User: *user, Wallet: wallet, WelcomeGranted: granted}, nil
}

// provisionResultWithoutGrant reads the already-settled wallet for a
// replayed provisioning (the welcome fingerprint was claimed before, so
// no new grant was minted). Account resolution joins the given transaction
// (owned or borrowed); the wallet read runs on the pool against committed
// state. Under a borrowed guard transaction the wallet row pre-exists in
// every reachable replay shape (a fresh wallet plus an already-claimed
// fingerprint would require a fingerprint collision), so the pool read
// observes it.
func (s *Service) provisionResultWithoutGrant(ctx context.Context, tx pgx.Tx, user *domain.User) (*ports.ProvisionResult, error) {
	lifecycle := s.credits.LifecycleInTx(tx)
	personalAccountID, err := lifecycle.ResolvePersonalAccount(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("resolve personal Billing Account: %w", err)
	}
	walletDetails, err := s.credits.GetWallet(ctx, personalAccountID)
	if err != nil {
		return nil, fmt.Errorf("read provisioned Credit Wallet: %w", err)
	}
	return &ports.ProvisionResult{User: *user, Wallet: walletDetails.WalletBalance, WelcomeGranted: false}, nil
}

func (s *Service) provisionIdentityTx(ctx context.Context, tx pgx.Tx, identity ports.VerifiedIdentity, fingerprint string) (*domain.User, ports.WalletBalance, bool, error) {
	zeroWallet := ports.WalletBalance{}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 5169813340854939127))`, identity.AuthUserID); err != nil {
		return nil, zeroWallet, false, fmt.Errorf("lock identity provisioning: %w", err)
	}
	user, err := findOrCreateIdentityUser(ctx, tx, identity)
	if err != nil {
		return nil, zeroWallet, false, err
	}
	if err := ensureIdentityNotDeleting(ctx, tx, user.ID); err != nil {
		return nil, zeroWallet, false, err
	}
	if identity.EnsureActive {
		if err := activateIdentityUser(ctx, tx, user, identity); err != nil {
			return nil, zeroWallet, false, err
		}
	}
	lifecycle := s.credits.LifecycleInTx(tx)
	if err := lifecycle.BindWelcomeIdentityKey(ctx, s.welcomeKeyFingerprint); err != nil {
		return nil, zeroWallet, false, fmt.Errorf("verify permanently bound Welcome identity key: %w", err)
	}
	personalAccountID, err := lifecycle.ResolvePersonalAccount(ctx, user.ID)
	if err != nil {
		return nil, zeroWallet, false, fmt.Errorf("resolve personal Billing Account: %w", err)
	}
	if err := lifecycle.ProvisionAccountWallet(ctx, personalAccountID); err != nil {
		return nil, zeroWallet, false, fmt.Errorf("provision Billing Account Wallet: %w", err)
	}
	grant, granted, err := lifecycle.ClaimWelcomeGrant(ctx, fingerprint, user.ID, ports.WelcomeGrantCommand{
		UserID:                user.ID,
		BillingAccountID:      personalAccountID,
		Amount:                welcomeGrantCredits,
		IdempotencyKey:        "welcome:" + fingerprint,
		ActorType:             "system",
		ActorID:               "identity-lifecycle",
		Source:                "welcome_grant",
		Reason:                "initial Welcome Grant",
		BusinessReferenceType: "welcome_claim",
		BusinessReferenceID:   fingerprint,
	})
	if err != nil {
		return nil, zeroWallet, false, fmt.Errorf("claim Welcome Grant: %w", err)
	}
	if err := lifecycle.SetAccountWalletFrozen(ctx, personalAccountID, user.Status != 1); err != nil {
		return nil, zeroWallet, false, fmt.Errorf("align Billing Account Wallet status: %w", err)
	}
	return user, grant.Wallet, granted, nil
}

func ensureIdentityNotDeleting(ctx context.Context, tx pgx.Tx, userID int64) error {
	var deletionExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_deletion_requests WHERE user_id = $1)`, userID).Scan(&deletionExists); err != nil {
		return fmt.Errorf("check identity deletion state: %w", err)
	}
	if deletionExists {
		return domain.ErrUserDeletionInProgress
	}
	return nil
}

func activateIdentityUser(ctx context.Context, tx pgx.Tx, user *domain.User, identity ports.VerifiedIdentity) error {
	if err := tx.QueryRow(ctx, `
UPDATE users SET display_name = $2, role = $3, status = 1, updated_at = now()
WHERE id = $1
RETURNING id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at`, user.ID, identity.DisplayName, identity.Role).Scan(&user.ID, &user.PublicID, &user.DisplayName, &user.AuthUserID, &user.StorageUID, &user.Avatar, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return fmt.Errorf("activate provisioned identity: %w", err)
	}
	return nil
}

func (s *Service) InitializeExistingIdentities(
	ctx context.Context,
	identities []ports.VerifiedIdentity,
) ([]ports.ProvisionResult, error) {
	results := make([]ports.ProvisionResult, 0, len(identities))
	for index, identity := range identities {
		result, err := s.ProvisionIdentity(ctx, identity)
		if err != nil {
			return nil, fmt.Errorf("initialize existing identity %d: %w", index, err)
		}
		results = append(results, *result)
	}
	return results, nil
}

func (s *Service) normalizeVerifiedIdentity(identity ports.VerifiedIdentity) (ports.VerifiedIdentity, string, error) {
	normalized, err := trust.NormalizeProvisioningIdentity(identity)
	if err != nil || len(s.welcomeIdentityKey) != 32 {
		if err != nil {
			return ports.VerifiedIdentity{}, "", fmt.Errorf("trusted canonical registration identity: %w", err)
		}
		return ports.VerifiedIdentity{}, "", fmt.Errorf("a 32-byte Welcome Grant key is required")
	}
	identity = normalized
	digest := hmac.New(sha256.New, s.welcomeIdentityKey)
	_, _ = digest.Write([]byte(identity.ClaimIdentity))
	return identity, hex.EncodeToString(digest.Sum(nil)), nil
}

func findOrCreateIdentityUser(ctx context.Context, tx pgx.Tx, identity ports.VerifiedIdentity) (*domain.User, error) {
	user := &domain.User{}
	var authUserID *string
	err := tx.QueryRow(ctx, `
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users WHERE auth_user_id = $1
FOR UPDATE`, identity.AuthUserID).Scan(
		&user.ID, &user.PublicID, &user.DisplayName, &authUserID, &user.StorageUID, &user.Avatar,
		&user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == nil {
		user.AuthUserID = authUserID
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find local identity: %w", err)
	}
	storageUID, err := domain.NewStorageUID()
	if err != nil {
		return nil, fmt.Errorf("allocate storage uid: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users_used_uids (storage_uid) VALUES ($1)`, storageUID); err != nil {
		return nil, fmt.Errorf("reserve storage uid: %w", err)
	}
	err = tx.QueryRow(ctx, `
INSERT INTO users (display_name, auth_user_id, storage_uid, avatar, role, status)
VALUES ($1, $2, $3, $4, $5, 1)
RETURNING id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at`,
		identity.DisplayName, identity.AuthUserID, storageUID, identity.Avatar, identity.Role,
	).Scan(
		&user.ID, &user.PublicID, &user.DisplayName, &authUserID, &user.StorageUID, &user.Avatar,
		&user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create local identity: %w", err)
	}
	user.AuthUserID = authUserID
	return user, nil
}

func rollbackIdentityTransaction(ctx context.Context, tx pgx.Tx, operation string) {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("failed to roll back identity provisioning transaction", "operation", operation, "err", err)
	}
}
