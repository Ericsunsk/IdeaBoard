package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ideaboard/api/internal/billingoverview"
	identitypg "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/memberquotas"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The snapshot and production identity/display adapters share one pool.
// Every identity projection must borrow the snapshot transaction rather
// than request another connection while holding the only available one.
func TestV1BillingOverviewIdentityProjectionsJoinSnapshotPG(t *testing.T) {
	db := testdb.Open(t)
	config := db.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, id := range []int64{11, 12} {
		if _, err := pool.Exec(t.Context(), `UPDATE users SET public_id = $1 WHERE id = $2`, testUUIDForUser(id), id); err != nil {
			t.Fatal(err)
		}
	}
	fx, quotas := newQuotaTestFixture()
	ws := v1WorkspaceFixture(t, t.Context(), fx.store, "overview-snapshot", 11)
	fx.dir.ensure(11)
	fx.dir.ensure(12)
	if _, err := fx.store.AddWorkspaceMember(t.Context(), ws.PublicID, 12, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	account := fx.mustAccount(t, t.Context(), ws)
	fx.seedWallet(account, 500, 0)
	fx.accounts.roles = map[int64]map[int64]string{account: {11: tenancy.BillingRoleOwner}}
	if _, err := fx.accounts.InitiateBillingTransferInvite(t.Context(), account, 11, 12, "snapshot-invite"); err != nil {
		t.Fatal(err)
	}
	quotas.period[quotaKey{account, 12}] = memberquotas.PeriodUnlimited
	users := identitypg.NewRepository(pool)
	fx.deps.Users = runtimehttp.NewUsersRepositoryDirectory(users)
	fx.deps.MemberDisplay = NewMemberDisplayService(users, nil)
	fx.deps.Snapshot = billingoverview.PostgresSnapshot(pool)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+ws.PublicID+"/billing-overview", nil).WithContext(ctx)
	testsupport.SetTestUser(req, 11)
	recorder := httptest.NewRecorder()
	fx.engine().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("overview with one pool connection = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := pool.Stat().CanceledAcquireCount(); got != 0 {
		t.Fatalf("snapshot projection requested a second connection: canceled acquisitions = %d", got)
	}
	var envelope struct {
		Data v1BillingOverviewDTO `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	payload := envelope.Data
	if len(payload.BillingMembers) != 1 || payload.BillingMembers[0].UserID != quotaUserPublicID(11) {
		t.Fatalf("billing members = %+v", payload.BillingMembers)
	}
	if len(payload.LiveInvites) != 1 || payload.LiveInvites[0].ToUserID != quotaUserPublicID(12) {
		t.Fatalf("transfer invites = %+v", payload.LiveInvites)
	}
	if len(payload.Quotas) != 1 || payload.Quotas[0].UserID != quotaUserPublicID(12) {
		t.Fatalf("quotas = %+v", payload.Quotas)
	}
}
