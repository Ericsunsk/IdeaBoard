package tests

// Control audit atomicity, step 3: the identity lifecycle ports join
// the ambient guard transaction instead of committing in their own.
//
// What is proven here, against the isolated fixture database:
//   - SetUserStatus (ban) on a carrier context flips the user row and
//     the wallet freeze inside the carrier: visible from the still-open
//     carrier, invisible on the pool, then durable after the carrier
//     commits.
//   - A carrier rollback leaves the user active and the wallet
//     unfrozen (the step-2 gap: the lifecycle commit used to survive
//     on its own).
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing lifecycle suite is that proof; it is not duplicated
//     here).

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// carrierStatusQuerier is satisfied by both pgx.Tx (same-transaction
// reads) and *pgxpool.Pool (outside reads).
type carrierStatusQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func provisionCarrierLifecycleUser(t *testing.T, pool *pgxpool.Pool, service *LifecycleService, authUserID, email string) int64 {
	t.Helper()
	result, err := service.ProvisionIdentity(context.Background(), VerifiedIdentity{
		AuthUserID: authUserID, DisplayName: "carrier-user",
		ClaimIdentity: email, ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity() error = %v", err)
	}
	if result.User.ID <= 0 || result.User.Status != 1 {
		t.Fatalf("provisioned user = %+v, want active user", result.User)
	}
	return result.User.ID
}

func readCarrierUserStatus(t *testing.T, ctx context.Context, querier carrierStatusQuerier, userID int64) int16 {
	t.Helper()
	var status int16
	if err := querier.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil {
		t.Fatalf("read user status: %v", err)
	}
	return status
}

func TestSetUserStatusJoinsAmbientCarrierTx(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	userID := provisionCarrierLifecycleUser(t, pool, service, "auth-carrier-ban-1", "carrier-ban-1@example.com")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := service.SetUserStatus(carrier, userID, StatusBanned); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SetUserStatus(banned) on carrier error = %v", err)
	}
	// Same-transaction visibility: the flip must be readable from
	// INSIDE the still-open carrier ...
	if got := readCarrierUserStatus(t, carrier, tx, userID); got != StatusBanned {
		_ = tx.Rollback(ctx)
		t.Fatalf("user status inside carrier = %d, want banned", got)
	}
	// ... and invisible outside it before the carrier commits.
	if got := readCarrierUserStatus(t, ctx, pool, userID); got != StatusActive {
		_ = tx.Rollback(ctx)
		t.Fatalf("user status on pool before commit = %d, want active", got)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if got := readCarrierUserStatus(t, ctx, pool, userID); got != StatusBanned {
		t.Fatalf("user status after carrier commit = %d, want banned", got)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(userID, "carrier:ban-frozen")); !errors.Is(err, domain.ErrWalletFrozen) {
		t.Fatalf("Hold(frozen wallet) error = %v, want ErrWalletFrozen", err)
	}
}

func TestSetUserStatusRollsBackWithAmbientCarrier(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	userID := provisionCarrierLifecycleUser(t, pool, service, "auth-carrier-ban-2", "carrier-ban-2@example.com")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	if err := service.SetUserStatus(carrier, userID, StatusBanned); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SetUserStatus(banned) on carrier error = %v", err)
	}
	// The business fails after the lifecycle write: the guard rolls the
	// whole carrier back, lifecycle flip and wallet freeze included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if got := readCarrierUserStatus(t, ctx, pool, userID); got != StatusActive {
		t.Fatalf("user status after carrier rollback = %d, want active", got)
	}
	if _, err := creditService.Hold(ctx, lifecycleHold(userID, "carrier:rollback-active")); err != nil {
		t.Fatalf("Hold(after carrier rollback) error = %v, want success (wallet unfrozen)", err)
	}
}
