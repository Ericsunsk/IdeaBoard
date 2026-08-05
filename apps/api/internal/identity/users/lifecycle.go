package users

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const welcomeGrantCredits int64 = 100

var ErrUserDeletionInProgress = errors.New("user deletion is already in progress")
var ErrUserDeletionBlockedByPaymentState = errors.New("user deletion is blocked by unsettled payment state")

type VerifiedIdentity struct {
	AuthUserID            string `json:"auth_user_id"`
	Username              string `json:"username"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
	Role                  string `json:"role"`
	EnsureActive          bool   `json:"-"`
}

type ProvisionResult struct {
	User           User
	Wallet         credits.WalletBalance
	WelcomeGranted bool
}

type LifecycleService struct {
	pool                  *pgxpool.Pool
	credits               CreditWalletLifecycleService
	welcomeIdentityKey    []byte
	welcomeKeyFingerprint string
}

type DeletionPhase string

const (
	DeletionPendingAuth      DeletionPhase = "pending_auth"
	DeletionPendingStorage   DeletionPhase = "pending_storage"
	DeletionPendingTombstone DeletionPhase = "pending_tombstone"
	DeletionCompleted        DeletionPhase = "completed"
)

type DeletionProgress struct {
	UserID     int64
	AuthUserID string
	StorageUID string
	Phase      DeletionPhase
}

type CreditWalletLifecycleService interface {
	LifecycleInTx(pgx.Tx) credits.WalletLifecycle
	GetWallet(context.Context, int64) (credits.WalletDetails, error)
}

func (s *LifecycleService) InitializeExistingIdentities(
	ctx context.Context,
	identities []VerifiedIdentity,
) ([]ProvisionResult, error) {
	results := make([]ProvisionResult, 0, len(identities))
	for index, identity := range identities {
		result, err := s.ProvisionIdentity(ctx, identity)
		if err != nil {
			return nil, fmt.Errorf("initialize existing identity %d: %w", index, err)
		}
		results = append(results, *result)
	}
	return results, nil
}

func NewLifecycleService(pool *pgxpool.Pool, creditService CreditWalletLifecycleService, welcomeIdentityKey []byte) *LifecycleService {
	keyDigest := sha256.Sum256(append([]byte("ideaboard:welcome-identity-key:v1:"), welcomeIdentityKey...))
	return &LifecycleService{
		pool:                  pool,
		credits:               creditService,
		welcomeIdentityKey:    append([]byte(nil), welcomeIdentityKey...),
		welcomeKeyFingerprint: hex.EncodeToString(keyDigest[:]),
	}
}

func (s *LifecycleService) VerifyWelcomeIdentityKey(ctx context.Context) error {
	if s == nil || s.pool == nil || s.credits == nil || len(s.welcomeIdentityKey) != 32 {
		return fmt.Errorf("identity lifecycle dependencies and a 32-byte Welcome Grant key are required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin Welcome identity key verification: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "Welcome identity key verification")
	if err := s.credits.LifecycleInTx(tx).BindWelcomeIdentityKey(ctx, s.welcomeKeyFingerprint); err != nil {
		return fmt.Errorf("verify permanently bound Welcome identity key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Welcome identity key verification: %w", err)
	}
	return nil
}

func (s *LifecycleService) ProvisionIdentity(ctx context.Context, identity VerifiedIdentity) (*ProvisionResult, error) {
	identity, fingerprint, err := s.normalizeVerifiedIdentity(identity)
	if err != nil {
		return nil, err
	}
	if s.pool == nil || s.credits == nil {
		return nil, fmt.Errorf("identity lifecycle dependencies are not configured")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin identity lifecycle transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back identity provisioning transaction", "err", rollbackErr)
		}
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 5169813340854939127))`, identity.AuthUserID); err != nil {
		return nil, fmt.Errorf("lock identity provisioning: %w", err)
	}
	user, err := findOrCreateIdentityUser(ctx, tx, identity)
	if err != nil {
		return nil, err
	}
	var deletionExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_deletion_requests WHERE user_id = $1)`, user.ID).Scan(&deletionExists); err != nil {
		return nil, fmt.Errorf("check identity deletion state: %w", err)
	}
	if deletionExists {
		return nil, ErrUserDeletionInProgress
	}
	if identity.EnsureActive {
		if err := tx.QueryRow(ctx, `
UPDATE users SET username = $2, role = $3, status = 1, updated_at = now()
WHERE id = $1
RETURNING id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at`,
			user.ID, identity.Username, identity.Role,
		).Scan(&user.ID, &user.Username, &user.AuthUserID, &user.StorageUID, &user.Avatar,
			&user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("activate provisioned identity: %w", err)
		}
	}
	lifecycle := s.credits.LifecycleInTx(tx)
	if err := lifecycle.BindWelcomeIdentityKey(ctx, s.welcomeKeyFingerprint); err != nil {
		return nil, fmt.Errorf("verify permanently bound Welcome identity key: %w", err)
	}
	if err := lifecycle.ProvisionWallet(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("provision Credit Wallet: %w", err)
	}
	amount, _ := credits.NewCreditAmount(welcomeGrantCredits)
	grant, granted, err := lifecycle.ClaimWelcomeGrant(ctx, fingerprint, credits.GrantCommand{
		UserID: user.ID,
		Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey:    "welcome:" + fingerprint,
			Actor:             credits.Actor{Type: credits.ActorSystem, ID: "identity-lifecycle"},
			Source:            "welcome_grant",
			Reason:            "initial Welcome Grant",
			BusinessReference: credits.BusinessReference{Type: "welcome_claim", ID: fingerprint},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claim Welcome Grant: %w", err)
	}
	if err := lifecycle.SetWalletFrozen(ctx, user.ID, user.Status != 1); err != nil {
		return nil, fmt.Errorf("align Credit Wallet status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit identity lifecycle transaction: %w", err)
	}
	wallet := grant.Wallet
	if !granted {
		walletDetails, err := s.credits.GetWallet(ctx, user.ID)
		if err != nil {
			return nil, fmt.Errorf("read provisioned Credit Wallet: %w", err)
		}
		wallet = walletDetails.WalletBalance
	}
	return &ProvisionResult{User: *user, Wallet: wallet, WelcomeGranted: granted}, nil
}

func (s *LifecycleService) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 || (status != 0 && status != 1) {
		return fmt.Errorf("invalid user status lifecycle command")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user status transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "user status")
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("lock user status lifecycle: %w", err)
	}
	if status == 1 {
		var deletionExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_deletion_requests WHERE user_id = $1)`, userID).Scan(&deletionExists); err != nil {
			return fmt.Errorf("check pending user deletion: %w", err)
		}
		if deletionExists {
			return ErrUserDeletionInProgress
		}
	}
	result, err := tx.Exec(ctx, `UPDATE users SET status = $2, updated_at = now() WHERE id = $1`, userID, status)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	if result.RowsAffected() != 1 {
		return database.ErrNotFound
	}
	if err := s.credits.LifecycleInTx(tx).SetWalletFrozen(ctx, userID, status == 0); err != nil {
		return fmt.Errorf("update Credit Wallet status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user status transaction: %w", err)
	}
	return nil
}

func (s *LifecycleService) BeginUserDeletion(ctx context.Context, userID int64) (*DeletionProgress, error) {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 {
		return nil, fmt.Errorf("invalid user deletion command")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin user deletion transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "begin user deletion")

	progress := &DeletionProgress{UserID: userID}
	var currentAuthUserID, currentStorageUID string
	if err := tx.QueryRow(ctx, `
SELECT COALESCE(auth_user_id, ''), storage_uid
FROM users
WHERE id = $1
FOR UPDATE`, userID).Scan(&currentAuthUserID, &currentStorageUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("lock user for deletion: %w", err)
	}
	err = tx.QueryRow(ctx, `
SELECT COALESCE(auth_user_id, ''), COALESCE(storage_uid, ''), phase
FROM user_deletion_requests
WHERE user_id = $1
FOR UPDATE`, userID).Scan(&progress.AuthUserID, &progress.StorageUID, &progress.Phase)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load user deletion request: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		progress.AuthUserID = currentAuthUserID
		progress.StorageUID = currentStorageUID
		if strings.TrimSpace(progress.AuthUserID) == "" {
			return nil, fmt.Errorf("user deletion requires an auth identity")
		}
		var hasUnsettledPaymentState bool
		if err := tx.QueryRow(ctx, `SELECT has_unsettled_payment_state($1)`, userID).Scan(&hasUnsettledPaymentState); err != nil {
			return nil, fmt.Errorf("check unsettled payment state before user deletion: %w", err)
		}
		if hasUnsettledPaymentState {
			return nil, ErrUserDeletionBlockedByPaymentState
		}
		progress.Phase = DeletionPendingAuth
		if _, err := tx.Exec(ctx, `
INSERT INTO user_deletion_requests (user_id, auth_user_id, storage_uid, phase)
VALUES ($1, $2, $3, $4)`, userID, progress.AuthUserID, progress.StorageUID, progress.Phase); err != nil {
			return nil, fmt.Errorf("record pending user deletion: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET status = 0, updated_at = now() WHERE id = $1`, userID); err != nil {
		return nil, fmt.Errorf("freeze user during deletion: %w", err)
	}
	if err := s.credits.LifecycleInTx(tx).SetWalletFrozen(ctx, userID, true); err != nil {
		return nil, fmt.Errorf("freeze Credit Wallet during deletion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit pending user deletion: %w", err)
	}
	return progress, nil
}

func (s *LifecycleService) AdvanceUserDeletion(ctx context.Context, userID int64, from, to DeletionPhase) error {
	if s == nil || s.pool == nil || userID <= 0 {
		return fmt.Errorf("invalid user deletion transition")
	}
	if (from != DeletionPendingAuth || to != DeletionPendingStorage) &&
		(from != DeletionPendingStorage || to != DeletionPendingTombstone) {
		return fmt.Errorf("invalid user deletion phase transition %s -> %s", from, to)
	}
	result, err := s.pool.Exec(ctx, `
UPDATE user_deletion_requests
SET phase = $3, updated_at = now()
WHERE user_id = $1 AND phase = $2`, userID, from, to)
	if err != nil {
		return fmt.Errorf("advance user deletion phase: %w", err)
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	var current DeletionPhase
	if err := s.pool.QueryRow(ctx, `SELECT phase FROM user_deletion_requests WHERE user_id = $1`, userID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("load user deletion phase: %w", err)
	}
	if current == to || current == DeletionCompleted {
		return nil
	}
	return fmt.Errorf("user deletion phase is %s, expected %s", current, from)
}

func (s *LifecycleService) TombstoneUser(ctx context.Context, userID int64) error {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 {
		return fmt.Errorf("invalid user tombstone command")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user tombstone transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "user tombstone")
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("lock user for tombstone: %w", err)
	}
	var phase DeletionPhase
	if err := tx.QueryRow(ctx, `SELECT phase FROM user_deletion_requests WHERE user_id = $1 FOR UPDATE`, userID).Scan(&phase); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("load user deletion request: %w", err)
	}
	if phase == DeletionCompleted {
		return nil
	}
	if phase != DeletionPendingTombstone {
		return fmt.Errorf("user deletion is not ready for tombstone: %s", phase)
	}
	tombstoneStorageUID, err := reserveTombstoneStorageUID(ctx, tx)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
UPDATE users
SET username = 'deleted-user-' || lower($2),
    auth_user_id = NULL,
    storage_uid = $2,
    avatar = '',
    role = 'user',
    status = 0,
    updated_at = now()
WHERE id = $1`, userID, tombstoneStorageUID)
	if err != nil {
		return fmt.Errorf("anonymize user tombstone: %w", err)
	}
	if result.RowsAffected() != 1 {
		return database.ErrNotFound
	}
	if err := s.credits.LifecycleInTx(tx).SetWalletFrozen(ctx, userID, true); err != nil {
		return fmt.Errorf("freeze Credit Wallet: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE user_deletion_requests
SET auth_user_id = NULL,
    storage_uid = NULL,
    phase = 'completed',
    completed_at = now(),
    updated_at = now()
WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("complete user deletion request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user tombstone transaction: %w", err)
	}
	return nil
}

func reserveTombstoneStorageUID(ctx context.Context, tx pgx.Tx) (string, error) {
	for range 32 {
		storageUID, err := NewStorageUID()
		if err != nil {
			return "", err
		}
		result, err := tx.Exec(ctx, `
INSERT INTO users_used_uids (storage_uid)
VALUES ($1)
ON CONFLICT (storage_uid) DO NOTHING`, storageUID)
		if err != nil {
			return "", fmt.Errorf("reserve tombstone storage uid: %w", err)
		}
		if result.RowsAffected() == 1 {
			return storageUID, nil
		}
	}
	return "", fmt.Errorf("allocate unique tombstone storage uid after 32 attempts")
}

func rollbackIdentityTransaction(ctx context.Context, tx pgx.Tx, operation string) {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.Warn("failed to roll back identity lifecycle transaction", "operation", operation, "err", err)
	}
}

func (s *LifecycleService) normalizeVerifiedIdentity(identity VerifiedIdentity) (VerifiedIdentity, string, error) {
	identity.AuthUserID = strings.TrimSpace(identity.AuthUserID)
	identity.Username = strings.TrimSpace(identity.Username)
	identity.ClaimIdentity = strings.ToLower(strings.TrimSpace(identity.ClaimIdentity))
	identity.ClaimIdentityKind = strings.TrimSpace(identity.ClaimIdentityKind)
	identity.Role = strings.ToLower(strings.TrimSpace(identity.Role))
	if identity.Role == "" {
		identity.Role = "user"
	}
	if identity.AuthUserID == "" || identity.Username == "" || identity.ClaimIdentity == "" ||
		identity.ClaimIdentityKind != "trusted_canonical_registration" || !identity.ClaimIdentityVerified ||
		(identity.Role != "user" && identity.Role != "admin") || len(s.welcomeIdentityKey) != 32 {
		return VerifiedIdentity{}, "", fmt.Errorf("trusted canonical registration identity and a 32-byte Welcome Grant key are required")
	}
	digest := hmac.New(sha256.New, s.welcomeIdentityKey)
	_, _ = digest.Write([]byte(identity.ClaimIdentity))
	return identity, hex.EncodeToString(digest.Sum(nil)), nil
}

func findOrCreateIdentityUser(ctx context.Context, tx pgx.Tx, identity VerifiedIdentity) (*User, error) {
	user := &User{}
	var authUserID *string
	err := tx.QueryRow(ctx, `
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users WHERE auth_user_id = $1
FOR UPDATE`, identity.AuthUserID).Scan(
		&user.ID, &user.Username, &authUserID, &user.StorageUID, &user.Avatar,
		&user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == nil {
		user.AuthUserID = authUserID
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find local identity: %w", err)
	}
	storageUID, err := NewStorageUID()
	if err != nil {
		return nil, fmt.Errorf("allocate storage uid: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users_used_uids (storage_uid) VALUES ($1)`, storageUID); err != nil {
		return nil, fmt.Errorf("reserve storage uid: %w", err)
	}
	err = tx.QueryRow(ctx, `
INSERT INTO users (username, auth_user_id, storage_uid, role, status)
VALUES ($1, $2, $3, $4, 1)
RETURNING id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at`,
		identity.Username, identity.AuthUserID, storageUID, identity.Role,
	).Scan(
		&user.ID, &user.Username, &authUserID, &user.StorageUID, &user.Avatar,
		&user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create local identity: %w", err)
	}
	user.AuthUserID = authUserID
	return user, nil
}
