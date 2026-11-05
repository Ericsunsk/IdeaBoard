// Organization-only quota enforcement tests (#975, part of #973).
//
// MemberQuotas is Org-only: the settlement and quote checks enforce the
// (Organization Billing Account, User) cap with the Owner short-circuit and
// the retained role matrix. Personal workspaces carry no quota concept, so
// any check against a Personal (non-Organization) Billing Account fails
// closed with ErrPersonalQuotaUnsupported: never silently unlimited, never
// member_quota_exhausted.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention via testdb.Open).
package memberquotas

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

// openPersonalQuotaEnv seeds one Personal workspace: a holder-owned
// workspace whose billing account carries no organization_id. Quota rows
// may exist on it (e.g. written before the Org-only shrink), but the
// checks must never enforce them.
func openPersonalQuotaEnv(t *testing.T) (pool *pgxpool.Pool, account, user int64) {
	t.Helper()
	ctx := context.Background()
	pool = testdb.Open(t)
	quotaTestSeq++

	storageUID := fmt.Sprintf("QP%04d", quotaTestSeq)
	if err := pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('quota-personal-user', $1, 'user', 1) RETURNING id`, storageUID).Scan(&user); err != nil {
		t.Fatalf("seed personal quota user: %v", err)
	}
	ws := fmt.Sprintf("ws_personalquota%04d", quotaTestSeq)
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'personal-quota-ws')`, ws, user); err != nil {
		t.Fatalf("seed personal quota workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, (SELECT internal_id FROM workspaces WHERE public_id = $2)) RETURNING internal_id`, fmt.Sprintf("ba_personalquota%04d", quotaTestSeq), ws).Scan(&account); err != nil {
		t.Fatalf("seed personal quota billing account: %v", err)
	}
	return pool, account, user
}

// TestOrgCheckRejectsPersonalAccount proves the Org-only fence: a tight
// Personal cap never reports member_quota_exhausted and never passes
// silently; both the settlement-transaction check and the quote preflight
// fail closed with ErrPersonalQuotaUnsupported.
func TestOrgCheckRejectsPersonalAccount(t *testing.T) {
	pool, account, user := openPersonalQuotaEnv(t)
	ctx := context.Background()
	store := NewStore(pool)

	tight := int64(15)
	if _, err := store.SetQuota(ctx, account, user, PeriodDaily, &tight, user); err != nil {
		t.Fatalf("seed tight personal quota: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := store.CheckQuotaInTx(ctx, tx, account, user, 20, time.Now().UTC()); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("personal settlement check = %v, want ErrPersonalQuotaUnsupported (never member_quota_exhausted, never nil)", err)
	}
	if err := store.CheckQuotaForQuote(ctx, account, user, 20, time.Now().UTC()); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("personal quote check = %v, want ErrPersonalQuotaUnsupported (never member_quota_exhausted, never nil)", err)
	}
	if err := store.CheckQuotaForQuote(ctx, account, user, 20, time.Now().UTC()); errors.Is(err, ErrQuotaExhausted) {
		t.Fatal("personal quote check reported member_quota_exhausted, want ErrPersonalQuotaUnsupported only")
	}

	// A nil-store (unwired pool) Personal quote fails closed the same way:
	// with no pool the Organization scope cannot be proven, so it never
	// silently allows and never reports member_quota_exhausted.
	nilStore := NewStore(nil)
	if err := nilStore.CheckQuotaForQuote(ctx, account, user, 20, time.Now().UTC()); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("nil-store personal quote check = %v, want ErrPersonalQuotaUnsupported", err)
	}
	if err := nilStore.CheckQuotaForQuote(ctx, account, user, 20, time.Now().UTC()); errors.Is(err, ErrQuotaExhausted) {
		t.Fatal("nil-store personal quote check reported member_quota_exhausted, want ErrPersonalQuotaUnsupported only")
	}
	// The nil-store settlement check joins the same caller tx, so it hits
	// the same Org-only fence (never silently unlimited).
	if err := nilStore.CheckQuotaInTx(ctx, tx, account, user, 20, time.Now().UTC()); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("nil-store personal settlement check = %v, want ErrPersonalQuotaUnsupported", err)
	}
}

// TestQuoteNilStoreFailClosedWithoutDB proves the quote preflight needs no
// database to fail closed: without a pool the Organization scope cannot be
// proven, so every check reports the Organization-only error (never a
// silent allow, never member_quota_exhausted), and malformed ids report
// not-found (never exhausted).
func TestQuoteNilStoreFailClosedWithoutDB(t *testing.T) {
	store := NewStore(nil)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CheckQuotaForQuote(ctx, 7001, 3, 20, now); !errors.Is(err, ErrPersonalQuotaUnsupported) {
		t.Fatalf("nil-store quote check = %v, want ErrPersonalQuotaUnsupported", err)
	}
	if err := store.CheckQuotaForQuote(ctx, 7001, 3, 20, now); errors.Is(err, ErrQuotaExhausted) {
		t.Fatal("nil-store quote check reported member_quota_exhausted, want ErrPersonalQuotaUnsupported only")
	}
	for _, tc := range [][2]int64{{0, 3}, {7001, 0}, {-1, 3}} {
		if err := store.CheckQuotaForQuote(ctx, tc[0], tc[1], 20, now); !errors.Is(err, ErrQuotaNotFound) {
			t.Fatalf("nil-store quote check ids=(%d,%d) = %v, want ErrQuotaNotFound", tc[0], tc[1], err)
		}
	}
	// A nil settlement transaction fails closed without touching quota math.
	if err := store.CheckQuotaInTx(ctx, nil, 7001, 3, 20, now); err == nil || errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("nil-tx settlement check = %v, want a fail-closed error (never nil, never member_quota_exhausted)", err)
	}
}
