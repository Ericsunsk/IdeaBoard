package tests

// Role repair revalidation tests (issue #896): real PostgreSQL + fake
// Auth reader/manager. Every test scans real drift first, then mutates
// the CURRENT facts, then repairs: only a repair whose pre-push facts
// still match may push, only demotions auto-repair, and resolution is
// recorded only after a confirmed repair.

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/application/reconciliation"
)

func newRoleRepairFixture(t *testing.T, manager *fakeIdentityAdminAuth) (*durableSyncFixture, *fakeAuthStateReader, *reconciliation.Reconciler) {
	t.Helper()
	f := newDurableSyncFixture(t)
	reader := &fakeAuthStateReader{states: make(map[string]authidentity.AuthUserState)}
	linkFixtureUsers(t, f.pool, reader)
	reconciler := reconciliation.NewReconciler(reconciliation.ReconcilerDeps{
		DB:        f.pool,
		Users:     NewUserRepository(f.pool),
		Auth:      reader,
		Manager:   manager,
		Lifecycle: &recordingUserLifecycle{},
	})
	return f, reader, reconciler
}

func scanRoleDrift(t *testing.T, reconciler *reconciliation.Reconciler) int64 {
	t.Helper()
	slot := time.Date(2026, 10, 4, 3, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	runID := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, slot)
	if err := reconciler.ExecuteReconciliation(context.Background(), runID); err != nil {
		t.Fatalf("execute reconciliation: %v", err)
	}
	return runID
}

func openRoleFindings(t *testing.T, reconciler *reconciliation.Reconciler, runID int64) []roleFindingStatus {
	t.Helper()
	findings, err := reconciler.ListRunFindings(context.Background(), runID)
	if err != nil {
		t.Fatalf("list run findings: %v", err)
	}
	var out []roleFindingStatus
	for _, finding := range findings {
		if finding.FindingType == "role_mismatch" {
			out = append(out, roleFindingStatus{Status: finding.Status})
		}
	}
	return out
}

type roleFindingStatus struct {
	Status string
}

func TestRoleRepairResolvesConfirmedDemotion(t *testing.T) {
	manager := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, manager)
	ctx := context.Background()
	_, _ = f.seedUser(t, "repair-ok", "auth-repair-ok", "RRPO01", "user", StatusActive)
	reader.states["auth-repair-ok"] = reconAuthState("auth-repair-ok", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	if err := reconciler.RepairRoleDemotions(ctx, runID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	assertIdentityAdminEvents(t, manager.events, "auth-role:user")
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "resolved" {
		t.Fatalf("role findings = %+v, want one resolved", findings)
	}
}

func TestRoleRepairSkipsLegalPromotion(t *testing.T) {
	manager := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, manager)
	ctx := context.Background()
	_, publicID := f.seedUser(t, "repair-promo", "auth-repair-promo", "RRPR01", "user", StatusActive)
	reader.states["auth-repair-promo"] = reconAuthState("auth-repair-promo", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	// Legal promotion commits after the scan: business is now admin with
	// its own sync intent. The stale demotion finding must not push.
	outcome, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin")
	if err != nil {
		t.Fatalf("legal promotion: %v", err)
	}
	if !outcome.SyncPending {
		t.Fatal("promotion outcome is not sync-pending")
	}
	if err := reconciler.RepairRoleDemotions(ctx, runID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if len(*manager.events) != 0 {
		t.Fatalf("repair pushed %v after a legal promotion, want no Auth I/O", *manager.events)
	}
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "open" {
		t.Fatalf("role findings = %+v, want one still open", findings)
	}
}

func TestRoleRepairSkipsAuthIdentityChange(t *testing.T) {
	manager := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, manager)
	ctx := context.Background()
	userID, _ := f.seedUser(t, "repair-link", "auth-repair-link", "RRLK01", "user", StatusActive)
	reader.states["auth-repair-link"] = reconAuthState("auth-repair-link", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	// The business row links to a new Auth identity after the scan: the
	// repair must not push to the stale identity.
	if _, err := f.pool.Exec(ctx, `UPDATE users SET auth_user_id = 'auth-repair-link-new' WHERE id = $1`, userID); err != nil {
		t.Fatalf("relink user: %v", err)
	}
	if err := reconciler.RepairRoleDemotions(ctx, runID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if len(*manager.events) != 0 {
		t.Fatalf("repair pushed %v to a stale identity, want no Auth I/O", *manager.events)
	}
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "open" {
		t.Fatalf("role findings = %+v, want one still open", findings)
	}
}

func TestRoleRepairResolvesWithoutPushWhenAlreadyConverged(t *testing.T) {
	manager := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, manager)
	ctx := context.Background()
	_, _ = f.seedUser(t, "repair-conv", "auth-repair-conv", "RRCV01", "user", StatusActive)
	reader.states["auth-repair-conv"] = reconAuthState("auth-repair-conv", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	// Auth converged out-of-band (retry, manual push): no second push,
	// but the finding resolves on confirmed agreement.
	reader.states["auth-repair-conv"] = reconAuthState("auth-repair-conv", "user", false)
	if err := reconciler.RepairRoleDemotions(ctx, runID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if len(*manager.events) != 0 {
		t.Fatalf("repair pushed %v for converged drift, want no Auth I/O", *manager.events)
	}
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "resolved" {
		t.Fatalf("role findings = %+v, want one resolved", findings)
	}
}

func TestRoleRepairLeavesOpenOnConcurrentPromotionAndHeals(t *testing.T) {
	inner := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, inner)
	ctx := context.Background()
	_, publicID := f.seedUser(t, "repair-race", "auth-repair-race", "RRRA01", "user", StatusActive)
	reader.states["auth-repair-race"] = reconAuthState("auth-repair-race", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	// A legal promotion commits between the repair's pre-push check and
	// its push (simulated deterministically inside the push call). The
	// repair must not resolve: the new business state owns convergence.
	racing := &racePromotionManager{inner: inner, promote: func() {
		if _, err := f.commands.SetRole(ctx, identityAdminActor(t), publicID, "admin"); err != nil {
			t.Errorf("concurrent legal promotion: %v", err)
		}
	}}
	reconciler = reconciliation.NewReconciler(reconciliation.ReconcilerDeps{
		DB:        f.pool,
		Users:     NewUserRepository(f.pool),
		Auth:      reader,
		Manager:   racing,
		Lifecycle: &recordingUserLifecycle{},
	})
	if err := reconciler.RepairRoleDemotions(ctx, runID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "open" {
		t.Fatalf("role findings = %+v, want one still open after the race", findings)
	}

	// The promotion's own durable intent heals the stale push: both
	// sides converge on admin without operator intervention. The stale
	// demote push landed on the repair manager; the heal lands on the
	// fixture Auth client that owns the promotion intent.
	if got := *inner.events; len(got) != 1 || got[0] != "auth-role:user" {
		t.Fatalf("repair pushes = %v, want exactly the one stale demote", got)
	}
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge after race: %v", err)
	}
	if role, _, _ := durableUserRow(t, f.pool, mustRepairUserID(t, f, publicID)); role != "admin" {
		t.Fatalf("business role = %q, want admin", role)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-role:admin")
}

type racePromotionManager struct {
	inner   *fakeIdentityAdminAuth
	promote func()
	fired   bool
}

func (m *racePromotionManager) DeleteUser(ctx context.Context, authUserID string) error {
	return m.inner.DeleteUser(ctx, authUserID)
}

func (m *racePromotionManager) SetUserRole(ctx context.Context, authUserID, role string) error {
	if !m.fired {
		m.fired = true
		m.promote()
	}
	return m.inner.SetUserRole(ctx, authUserID, role)
}

func (m *racePromotionManager) BanUser(ctx context.Context, authUserID string, expires int) error {
	return m.inner.BanUser(ctx, authUserID, expires)
}

func (m *racePromotionManager) UnbanUser(ctx context.Context, authUserID string) error {
	return m.inner.UnbanUser(ctx, authUserID)
}

func (m *racePromotionManager) RevokeUserSessions(ctx context.Context, authUserID string) error {
	return m.inner.RevokeUserSessions(ctx, authUserID)
}

func mustRepairUserID(t *testing.T, f *durableSyncFixture, publicID string) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(), `SELECT id FROM users WHERE public_id::text = $1`, publicID).Scan(&id); err != nil {
		t.Fatalf("resolve user id: %v", err)
	}
	return id
}

func TestRoleRepairAuthReadErrorRetries(t *testing.T) {
	manager := &fakeIdentityAdminAuth{events: &[]string{}}
	f, reader, reconciler := newRoleRepairFixture(t, manager)
	ctx := context.Background()
	_, _ = f.seedUser(t, "repair-rerr", "auth-repair-rerr", "RRRE01", "user", StatusActive)
	reader.states["auth-repair-rerr"] = reconAuthState("auth-repair-rerr", "admin", false)
	runID := scanRoleDrift(t, reconciler)

	reader.err = errors.New("auth service unavailable")
	if err := reconciler.RepairRoleDemotions(ctx, runID); err == nil {
		t.Fatal("repair during Auth outage succeeded, want retryable error")
	}
	findings := openRoleFindings(t, reconciler, runID)
	if len(findings) != 1 || findings[0].Status != "open" {
		t.Fatalf("role findings = %+v, want one still open", findings)
	}
}
