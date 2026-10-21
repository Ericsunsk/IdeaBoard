package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool    *pgxpool.Pool
	credits ports.CreditWalletLifecycleService
}

func NewService(pool *pgxpool.Pool, creditService ports.CreditWalletLifecycleService) *Service {
	return &Service{
		pool:    pool,
		credits: creditService,
	}
}

func (s *Service) SetUserStatus(ctx context.Context, userID int64, status int16) error {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 || (status != domain.StatusBanned && status != domain.StatusActive) {
		return fmt.Errorf("invalid user status lifecycle command")
	}
	// Ambient guard transaction (control audit atomicity, step 3): the
	// status flip plus the wallet freeze join the borrowed guard
	// transaction. The guard began it and the guard commits or rolls it
	// back, so this path never commits or rolls back. With no carrier
	// the legacy owned-transaction path below runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		return s.setUserStatusTx(ctx, ambient, userID, status)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin user status transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "user status")
	if err := s.setUserStatusTx(ctx, tx, userID, status); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit user status transaction: %w", err)
	}
	return nil
}

// setUserStatusTx locks the user row, flips the status, and aligns the
// wallet freeze inside the caller's transaction (owned or borrowed).
func (s *Service) setUserStatusTx(ctx context.Context, tx pgx.Tx, userID int64, status int16) error {
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return database.ErrNotFound
		}
		return fmt.Errorf("lock user status lifecycle: %w", err)
	}
	if status == domain.StatusActive {
		var deletionExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_deletion_requests WHERE user_id = $1)`, userID).Scan(&deletionExists); err != nil {
			return fmt.Errorf("check pending user deletion: %w", err)
		}
		if deletionExists {
			return domain.ErrUserDeletionInProgress
		}
	}
	result, err := tx.Exec(ctx, `UPDATE users SET status = $2, updated_at = now() WHERE id = $1`, userID, status)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	if result.RowsAffected() != 1 {
		return database.ErrNotFound
	}
	// Ban/unban touches only solely-owned non-organization account wallets;
	// shared and organization funds never freeze because one member was
	// deactivated (#916). Existing Holds still converge via Capture/Release.
	if err := s.credits.LifecycleInTx(tx).SetSoleOwnedWalletsFrozen(ctx, userID, status == domain.StatusBanned); err != nil {
		return fmt.Errorf("update Billing Account Wallet status: %w", err)
	}
	return nil
}

func (s *Service) DeleteUser(
	ctx context.Context,
	userID int64,
	authDeleter ports.AuthUserDeleter,
	uploadCleaner ports.UserUploadCleaner,
) error {
	// Durable saga order: the initialization above commits business
	// deactivate + Wallet freeze + durable deletion request first; only
	// then do the external Auth -> Media Storage calls run, each followed
	// by its own durable phase advance, ending with the business
	// tombstone. No step joins the Control guard ambient transaction, so
	// an external success followed by a guard audit/commit failure keeps
	// its progress and the retry converges without restoring active.
	progress, err := s.beginUserDeletion(ctx, userID)
	if err != nil {
		return err
	}

	for progress.Phase != ports.DeletionCompleted {
		switch progress.Phase {
		case ports.DeletionPendingAuth:
			if authDeleter == nil {
				return domain.ErrUserDeletionAuthUnavailable
			}
			if err := authDeleter.DeleteUser(ctx, progress.AuthUserID); err != nil && !isAuthIdentityAlreadyGone(err) {
				return fmt.Errorf("delete Auth user: %w", err)
			}
			progress.Phase, err = s.advanceDeletion(ctx, progress.UserID, ports.DeletionPendingAuth, ports.DeletionPendingStorage)
			if err != nil {
				return fmt.Errorf("record Auth deletion: %w", err)
			}
		case ports.DeletionPendingStorage:
			if uploadCleaner == nil {
				return domain.ErrUserDeletionStorageUnavailable
			}
			if err := uploadCleaner.DeleteUserUploads(ctx, progress.UserID, progress.StorageUID); err != nil {
				return fmt.Errorf("delete user uploads: %w", err)
			}
			progress.Phase, err = s.advanceDeletion(ctx, progress.UserID, ports.DeletionPendingStorage, ports.DeletionPendingTombstone)
			if err != nil {
				return fmt.Errorf("record user upload cleanup: %w", err)
			}
		case ports.DeletionPendingTombstone:
			progress.Phase, err = s.tombstoneUser(ctx, progress.UserID)
			if err != nil {
				return fmt.Errorf("tombstone user: %w", err)
			}
		default:
			return fmt.Errorf("unsupported user deletion phase %q", progress.Phase)
		}
	}
	return nil
}

// isAuthIdentityAlreadyGone reports whether an Auth delete failure means
// the identity is already absent from the Auth service. Auth answers a
// missing user with a 404 not_found envelope, which the client maps to
// ErrAuthUserNotFound; for the pending_auth phase that is a converged
// state, not a failure. Deletion must keep converging idempotently
// (Auth -> Media Storage -> business tombstone): a prior attempt can
// succeed on the Auth side and still lose the phase advance, so a retry
// only ever sees 404. Treating it as done is what lets Storage cleanup
// and the PII tombstone run instead of stranding a frozen account with
// its media and personal data retained.
func isAuthIdentityAlreadyGone(err error) bool {
	return errors.Is(err, autherrors.ErrAuthUserNotFound)
}

func (s *Service) beginUserDeletion(ctx context.Context, userID int64) (*ports.DeletionProgress, error) {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 {
		return nil, fmt.Errorf("invalid user deletion command")
	}
	// Durable deletion saga: the initialization (business deactivate +
	// Wallet freeze + durable deletion request) always commits in its own
	// transaction BEFORE any external Auth/Media call. It deliberately
	// never joins the Control guard ambient transaction: the guard's
	// audit/commit failure must not roll the initialization back to
	// active, and the saga must not hold a DB transaction open across
	// external Auth/Storage I/O. Each later phase advances in its own
	// durable transaction for the same reason.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin user deletion transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "begin user deletion")

	progress, err := beginUserDeletionTx(ctx, s.credits, tx, userID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit user deletion initialization: %w", err)
	}
	return progress, nil
}

// beginUserDeletionTx records the deletion progress and freezes the user
// and wallet rows inside the caller's owned transaction.
func beginUserDeletionTx(ctx context.Context, credits ports.CreditWalletLifecycleService, tx pgx.Tx, userID int64) (*ports.DeletionProgress, error) {
	progress, err := loadOrCreateDeletionProgress(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET status = 0, updated_at = now() WHERE id = $1`, userID); err != nil {
		return nil, fmt.Errorf("freeze user during deletion: %w", err)
	}
	if err := credits.LifecycleInTx(tx).SetSoleOwnedWalletsFrozen(ctx, userID, true); err != nil {
		return nil, fmt.Errorf("freeze Billing Account Wallets during deletion: %w", err)
	}
	return progress, nil
}

func loadOrCreateDeletionProgress(ctx context.Context, tx pgx.Tx, userID int64) (*ports.DeletionProgress, error) {
	progress := &ports.DeletionProgress{UserID: userID}
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
	err := tx.QueryRow(ctx, `
SELECT COALESCE(auth_user_id, ''), COALESCE(storage_uid, ''), phase
FROM user_deletion_requests
WHERE user_id = $1
FOR UPDATE`, userID).Scan(&progress.AuthUserID, &progress.StorageUID, &progress.Phase)
	if err == nil {
		return progress, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("load user deletion request: %w", err)
	}
	progress.AuthUserID, progress.StorageUID = currentAuthUserID, currentStorageUID
	if strings.TrimSpace(progress.AuthUserID) == "" {
		return nil, fmt.Errorf("user deletion requires an auth identity")
	}
	var hasUnsettledPaymentState bool
	if err := tx.QueryRow(ctx, `SELECT has_unsettled_payment_state($1)`, userID).Scan(&hasUnsettledPaymentState); err != nil {
		return nil, fmt.Errorf("check unsettled payment state before user deletion: %w", err)
	}
	if hasUnsettledPaymentState {
		return nil, domain.ErrUserDeletionBlockedByPaymentState
	}
	progress.Phase = ports.DeletionPendingAuth
	if _, err := tx.Exec(ctx, `
INSERT INTO user_deletion_requests (user_id, auth_user_id, storage_uid, phase)
VALUES ($1, $2, $3, $4)`, userID, progress.AuthUserID, progress.StorageUID, progress.Phase); err != nil {
		return nil, fmt.Errorf("record pending user deletion: %w", err)
	}
	return progress, nil
}

func (s *Service) advanceDeletion(ctx context.Context, userID int64, from, to ports.DeletionPhase) (ports.DeletionPhase, error) {
	if s == nil || s.pool == nil || userID <= 0 {
		return "", fmt.Errorf("invalid user deletion transition")
	}
	if (from != ports.DeletionPendingAuth || to != ports.DeletionPendingStorage) &&
		(from != ports.DeletionPendingStorage || to != ports.DeletionPendingTombstone) {
		return "", fmt.Errorf("invalid user deletion phase transition %s -> %s", from, to)
	}
	// Durable saga phase advance: each Auth/Media step commits its phase
	// row in its own statement on the pool, independent of the Control
	// guard ambient transaction. An external success followed by a guard
	// audit/commit failure therefore keeps its durable progress and the
	// retry converges instead of rolling back to the previous phase.
	return advanceDeletionPhase(ctx, s.pool, userID, from, to)
}

type deletionPhaseExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func advanceDeletionPhase(ctx context.Context, store deletionPhaseExecutor, userID int64, from, to ports.DeletionPhase) (ports.DeletionPhase, error) {
	result, err := store.Exec(ctx, `
UPDATE user_deletion_requests
SET phase = $3, updated_at = now()
WHERE user_id = $1 AND phase = $2`, userID, from, to)
	if err != nil {
		return "", fmt.Errorf("advance user deletion phase: %w", err)
	}
	if result.RowsAffected() == 1 {
		return to, nil
	}
	var current ports.DeletionPhase
	if err := store.QueryRow(ctx, `SELECT phase FROM user_deletion_requests WHERE user_id = $1`, userID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", database.ErrNotFound
		}
		return "", fmt.Errorf("load user deletion phase: %w", err)
	}
	if current == to || current == ports.DeletionCompleted {
		return current, nil
	}
	return "", fmt.Errorf("user deletion phase is %s, expected %s", current, from)
}

func (s *Service) tombstoneUser(ctx context.Context, userID int64) (ports.DeletionPhase, error) {
	if s == nil || s.pool == nil || s.credits == nil || userID <= 0 {
		return "", fmt.Errorf("invalid user tombstone command")
	}
	// Durable saga final step: the PII tombstone (anonymize user, keep
	// Wallet frozen, clear deletion PII, mark completed) always commits in
	// its own transaction. It never joins the Control guard ambient
	// transaction, so a guard audit/commit failure after the tombstone
	// cannot resurrect personal data or an active Wallet; the retry
	// replays idempotently from the completed phase.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin user tombstone transaction: %w", err)
	}
	defer rollbackIdentityTransaction(ctx, tx, "user tombstone")
	phase, err := tombstoneUserTx(ctx, s.credits, tx, userID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit user tombstone transaction: %w", err)
	}
	return phase, nil
}

// tombstoneUserTx anonymizes the user, freezes the wallet, and completes
// the deletion request inside the owned tombstone transaction.
// A completed tombstone replays idempotently.
func tombstoneUserTx(ctx context.Context, credits ports.CreditWalletLifecycleService, tx pgx.Tx, userID int64) (ports.DeletionPhase, error) {
	phase, err := loadTombstonePhase(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	if phase == ports.DeletionCompleted {
		return ports.DeletionCompleted, nil
	}
	if phase != ports.DeletionPendingTombstone {
		return "", fmt.Errorf("user deletion is not ready for tombstone: %s", phase)
	}
	tombstoneStorageUID, err := reserveTombstoneStorageUID(ctx, tx)
	if err != nil {
		return "", err
	}
	if err := anonymizeTombstoneUser(ctx, tx, userID, tombstoneStorageUID); err != nil {
		return "", err
	}
	if err := credits.LifecycleInTx(tx).SetSoleOwnedWalletsFrozen(ctx, userID, true); err != nil {
		return "", fmt.Errorf("freeze Billing Account Wallets: %w", err)
	}
	if err := credits.LifecycleInTx(tx).RemoveSharedBillingMemberships(ctx, userID); err != nil {
		return "", fmt.Errorf("detach shared Billing Account memberships: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE user_deletion_requests
SET auth_user_id = NULL,
    storage_uid = NULL,
    phase = 'completed',
    completed_at = now(),
    updated_at = now()
WHERE user_id = $1`, userID); err != nil {
		return "", fmt.Errorf("complete user deletion request: %w", err)
	}
	return ports.DeletionCompleted, nil
}

func loadTombstonePhase(ctx context.Context, tx pgx.Tx, userID int64) (ports.DeletionPhase, error) {
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", database.ErrNotFound
		}
		return "", fmt.Errorf("lock user for tombstone: %w", err)
	}
	var phase ports.DeletionPhase
	if err := tx.QueryRow(ctx, `SELECT phase FROM user_deletion_requests WHERE user_id = $1 FOR UPDATE`, userID).Scan(&phase); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", database.ErrNotFound
		}
		return "", fmt.Errorf("load user deletion request: %w", err)
	}
	return phase, nil
}

func anonymizeTombstoneUser(ctx context.Context, tx pgx.Tx, userID int64, tombstoneStorageUID string) error {
	result, err := tx.Exec(ctx, `
UPDATE users
SET display_name = '已注销用户',
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
	return nil
}

func reserveTombstoneStorageUID(ctx context.Context, tx pgx.Tx) (string, error) {
	for range 32 {
		storageUID, err := domain.NewStorageUID()
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
