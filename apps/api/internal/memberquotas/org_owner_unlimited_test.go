// Organization Owner short-circuit focused integration test (#953).
//
// The settlement check (Task submit / Vision reserve) short-circuits
// Organization Owners to unlimited before locking the quota row: a stale
// capped row plus over-cap Task consumption never gates an Owner, while
// the same cap still gates Admin/Member. Focused on the short-circuit
// core only: no cross-window or refund assertions.
//
// PostgreSQL integration test: SKIPs without API_TEST_DATABASE_DSN
// (house convention via testdb.Open).

package memberquotas

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
)

// orgQuotaFixture is one Organization scope on a shared Organization
// Billing Account: owner/admin/member memberships plus one Organization
// Workspace for Task attribution.
type orgQuotaFixture struct {
	env       *quotaEnv
	account   int64
	workspace int64
	owner     int64
	admin     int64
	member    int64
}

func openOrgQuotaFixture(t *testing.T) *orgQuotaFixture {
	t.Helper()
	e := openQuotaEnv(t)
	ctx := context.Background()

	uidC := fmt.Sprintf("Q%04dC", quotaTestSeq)
	var userC int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO users (display_name, storage_uid, role, status) VALUES ('quota-user-c', $1, 'user', 1) RETURNING id`, uidC).Scan(&userC); err != nil {
		t.Fatalf("seed quota org admin user: %v", err)
	}
	var orgID int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO organizations (public_id, name) VALUES ($1, 'quota-org') RETURNING internal_id`, fmt.Sprintf("org_quota%04d", quotaTestSeq)).Scan(&orgID); err != nil {
		t.Fatalf("seed quota organization: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'admin'), ($1, $4, 'member')`, orgID, e.userA, userC, e.userB); err != nil {
		t.Fatalf("seed quota org memberships: %v", err)
	}
	var account int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id, organization_id) VALUES ($1, NULL, $2) RETURNING internal_id`, fmt.Sprintf("ba_orgquota%04d", quotaTestSeq), orgID).Scan(&account); err != nil {
		t.Fatalf("seed quota org billing account: %v", err)
	}
	var wsID int64
	if err := e.pool.QueryRow(ctx, `INSERT INTO workspaces (public_id, organization_id, owner_user_id, name) VALUES ($1, $2, NULL, 'quota-org-ws') RETURNING internal_id`, fmt.Sprintf("ws_orgquota%04d", quotaTestSeq), orgID).Scan(&wsID); err != nil {
		t.Fatalf("seed quota org workspace: %v", err)
	}
	grant, err := creditsdomain.NewCreditAmount(100000)
	if err != nil {
		t.Fatalf("quota org grant amount: %v", err)
	}
	if _, err := e.credits.Grant(ctx, creditsports.GrantCommand{BillingAccountID: account, Amount: grant, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: fmt.Sprintf("quota-org-seed-%d", quotaTestSeq),
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "quota org seed",
		BusinessReference: creditsdomain.BusinessReference{Type: "test", ID: fmt.Sprintf("quota-org-seed-%d", quotaTestSeq)},
	}}); err != nil {
		t.Fatalf("grant quota org funds: %v", err)
	}
	return &orgQuotaFixture{env: e, account: account, workspace: wsID, owner: e.userA, admin: userC, member: e.userB}
}

// consumeTasks records two 30-credit Task consumptions (pending Holds +
// snapshots, 60 total) for userID on the Organization account.
func (f *orgQuotaFixture) consumeTasks(t *testing.T, userID int64, tag string) {
	t.Helper()
	now := time.Now().UTC()
	for i, key := range []string{tag + "-1", tag + "-2"} {
		taskID := fmt.Sprintf("task_orgq%04d_%s_%d", quotaTestSeq, tag, i)
		holdTx := f.env.hold(t, nil, f.account, taskID, userID, 30, "quota-org-hold-"+key)
		f.env.insertTaskSnapshot(t, f.env.pool, f.account, f.workspace, taskID, userID, now, holdTx, 30)
	}
}

func (f *orgQuotaFixture) checkTx(t *testing.T, userID, amount int64) error {
	t.Helper()
	tx, err := f.env.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return f.env.store.CheckQuotaInTx(context.Background(), tx, f.account, userID, amount, time.Now().UTC())
}

// TestOrgOwnerShortCircuitUnlimited proves the #953 Owner short-circuit
// against real Task facts: the Owner carries a stale capped daily row and
// 60/50 consumed, yet both the settlement-transaction check and the quote
// preflight allow. The same cap still gates Admin and Member.
func TestOrgOwnerShortCircuitUnlimited(t *testing.T) {
	f := openOrgQuotaFixture(t)
	ctx := context.Background()
	cap := int64(50)
	for _, userID := range []int64{f.owner, f.admin, f.member} {
		if _, err := f.env.store.SetQuota(ctx, f.account, userID, PeriodDaily, &cap, f.owner); err != nil {
			t.Fatalf("set org quota for %d: %v", userID, err)
		}
	}
	f.consumeTasks(t, f.owner, "owner")
	f.consumeTasks(t, f.admin, "admin")
	f.consumeTasks(t, f.member, "member")

	// The Owner's capped row survives (the allow below comes from the
	// short-circuit, not from a missing row).
	period, limit, found, err := f.env.store.GetQuotaRow(ctx, f.account, f.owner)
	if err != nil || !found || period != PeriodDaily || limit == nil || *limit != 50 {
		t.Fatalf("owner quota row = (%q, %v, %v, %v), want (daily, 50, true, nil)", period, limit, found, err)
	}
	if err := f.checkTx(t, f.owner, 1); err != nil {
		t.Fatalf("owner settlement check: %v, want nil (short-circuit)", err)
	}
	if err := f.env.store.CheckQuotaForQuote(ctx, f.account, f.owner, 1, time.Now().UTC()); err != nil {
		t.Fatalf("owner quote check: %v, want nil (short-circuit)", err)
	}
	// Admin and Member share the same 60/50 over-cap shape and fail
	// closed: the short-circuit is Owner-only.
	if err := f.checkTx(t, f.admin, 1); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("admin settlement check = %v, want ErrQuotaExhausted", err)
	}
	if err := f.checkTx(t, f.member, 1); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("member settlement check = %v, want ErrQuotaExhausted", err)
	}
}
