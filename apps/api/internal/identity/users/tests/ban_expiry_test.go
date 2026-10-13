package tests

// Ban deadline and expiry unban tests (issue #895): real PostgreSQL
// transactions + fake Auth manager + controllable clock. The business
// deadline is the fact source; expiry flips status + Wallet and persists
// an unban intent atomically, reusing the issue #888 serialization and
// fencing.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
)

func durableBanDeadline(t *testing.T, f *durableSyncFixture, userID int64) *time.Time {
	t.Helper()
	var deadline *time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT ban_expires_at FROM users WHERE id = $1`, userID).Scan(&deadline); err != nil {
		t.Fatalf("read ban deadline: %v", err)
	}
	return deadline
}

func TestBanExpiryConvergesWithControlledClock(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-expiry", "auth-ban-expiry", "BEXP01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 7}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	deadline := durableBanDeadline(t, f, userID)
	if deadline == nil {
		t.Fatal("ban deadline is NULL, want a finite deadline")
	}
	if until := time.Until(*deadline); until < 6*24*time.Hour || until > 8*24*time.Hour {
		t.Fatalf("ban deadline in %v, want ~7 days", until)
	}

	// Before the deadline nothing expires, even though the ban intent is
	// still converging.
	if expired, err := f.sync.ProcessDueBanExpiries(ctx, time.Now()); err != nil || expired != 0 {
		t.Fatalf("early expiry = %d, err = %v, want 0/<nil>", expired, err)
	}

	// Past the deadline the sweep unbans atomically: business active,
	// Wallet unfrozen, deadline cleared, unban intent persisted.
	future := deadline.Add(time.Hour)
	expired, err := f.sync.ProcessDueBanExpiries(ctx, future)
	if err != nil {
		t.Fatalf("process expiries: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusActive {
		t.Fatalf("business status = %d, want active", status)
	}
	if status := durableWalletStatus(t, f.pool, userID); status != creditsdomain.WalletStatusActive {
		t.Fatalf("wallet status = %q, want active", status)
	}
	if deadline := durableBanDeadline(t, f, userID); deadline != nil {
		t.Fatalf("ban deadline = %v, want cleared after expiry", deadline)
	}
	var unbans int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND op = 'unban' AND status = 'pending'`, userID).Scan(&unbans); err != nil || unbans != 1 {
		t.Fatalf("pending unban intents = %d, err = %v, want 1", unbans, err)
	}

	// The unban intent converges through the normal durable path, while
	// the stale ban intent is superseded without ever pushing.
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge: %v", err)
	}
	assertIdentityAdminEvents(t, f.auth.events, "auth-unban")
}

func TestPermanentBanNeverExpires(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-permanent", "auth-ban-perm", "BPRM01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if deadline := durableBanDeadline(t, f, userID); deadline != nil {
		t.Fatalf("ban deadline = %v, want NULL for a permanent ban", deadline)
	}

	if expired, err := f.sync.ProcessDueBanExpiries(ctx, time.Now().Add(365*24*time.Hour)); err != nil || expired != 0 {
		t.Fatalf("expiry = %d, err = %v, want 0/<nil> a year later", expired, err)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusBanned {
		t.Fatalf("business status = %d, want still banned", status)
	}
}

func TestRebanExtendsDeadlineAndOldSweepSkips(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-reban", "auth-ban-reban", "BRBN01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 1}); err != nil {
		t.Fatalf("first ban: %v", err)
	}
	firstDeadline := durableBanDeadline(t, f, userID)
	if firstDeadline == nil {
		t.Fatal("first deadline is NULL, want finite")
	}

	// Half a day later the admin re-bans for 30 days: the deadline moves
	// forward, the version advances, and a fresh ban intent covers it.
	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 30}); err != nil {
		t.Fatalf("re-ban: %v", err)
	}
	secondDeadline := durableBanDeadline(t, f, userID)
	if secondDeadline == nil || !secondDeadline.After(firstDeadline.Add(24*time.Hour)) {
		t.Fatalf("second deadline = %v, want ~30 days out (first was %v)", secondDeadline, firstDeadline)
	}
	if _, _, version := durableUserRow(t, f.pool, userID); version != 2 {
		t.Fatalf("business version = %d, want 2 (one bump per ban)", version)
	}

	// The old deadline passing no longer unbans: the row carries the
	// newer ban, and the sweep only reads current facts.
	if expired, err := f.sync.ProcessDueBanExpiries(ctx, firstDeadline.Add(time.Hour)); err != nil || expired != 0 {
		t.Fatalf("stale sweep expiry = %d, err = %v, want 0/<nil>", expired, err)
	}
	if _, status, _ := durableUserRow(t, f.pool, userID); status != StatusBanned {
		t.Fatalf("business status = %d, want still banned after stale sweep", status)
	}

	// The extended deadline still expires on its own time.
	if expired, err := f.sync.ProcessDueBanExpiries(ctx, secondDeadline.Add(time.Hour)); err != nil || expired != 1 {
		t.Fatalf("extended sweep expiry = %d, err = %v, want 1/<nil>", expired, err)
	}
}

func TestBanExpirySweepIsIdempotentUnderConcurrency(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-dup", "auth-ban-dup", "BDUP01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 1}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	deadline := durableBanDeadline(t, f, userID)
	future := deadline.Add(time.Hour)

	var wg sync.WaitGroup
	results := make([]int, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n, err := f.sync.ProcessDueBanExpiries(ctx, future)
			results[i] = n
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent sweep %d: %v", i, err)
		}
	}
	if results[0]+results[1] != 1 {
		t.Fatalf("expiry results = %v, want exactly one unban total", results)
	}
	var unbans int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM identity_auth_sync_intents WHERE user_id = $1 AND op = 'unban'`, userID).Scan(&unbans); err != nil || unbans != 1 {
		t.Fatalf("unban intents = %d, err = %v, want 1", unbans, err)
	}
}

func TestBanExpiryAuthErrorRetriesConvergence(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-authern", "auth-ban-aerr", "BAER01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 1}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	deadline := durableBanDeadline(t, f, userID)

	// Auth is down when the unban pushes: the business unban already
	// committed (business-first), the intent stays pending for retry.
	f.auth.unbanErrors = []error{errors.New("auth service unavailable")}
	if expired, err := f.sync.ProcessDueBanExpiries(ctx, deadline.Add(time.Hour)); err != nil || expired != 1 {
		t.Fatalf("expiry = %d, err = %v, want 1/<nil>", expired, err)
	}
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge during outage: %v", err)
	}
	if got := durableIntentCount(t, f.pool, userID, "pending"); got != 1 {
		t.Fatalf("pending intents = %d, want 1 durable retry", got)
	}

	f.auth.unbanErrors = nil
	f.sync.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	if _, err := f.sync.ConvergeDue(ctx); err != nil {
		t.Fatalf("converge after recovery: %v", err)
	}
	if got := durableIntentCount(t, f.pool, userID, "done"); got != 1 {
		t.Fatalf("done intents = %d, want 1", got)
	}
}

func TestBanExpirySkipsDeletionAndTombstone(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()

	deletionID, _ := f.seedUser(t, "ban-del", "auth-ban-del", "BDEL01", "user", StatusBanned)
	if _, err := f.pool.Exec(ctx, `UPDATE users SET ban_expires_at = now() - interval '1 hour' WHERE id = $1`, deletionID); err != nil {
		t.Fatalf("seed expired deadline: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_deletion_requests (user_id, auth_user_id, storage_uid, phase) VALUES ($1, 'auth-ban-del', 'BDEL01', 'pending_storage')`, deletionID); err != nil {
		t.Fatalf("seed deletion request: %v", err)
	}

	tombID, _ := f.seedUser(t, "ban-tomb", "auth-ban-tomb", "BTMB01", "user", StatusBanned)
	if _, err := f.pool.Exec(ctx, `UPDATE users SET ban_expires_at = now() - interval '1 hour', auth_user_id = NULL WHERE id = $1`, tombID); err != nil {
		t.Fatalf("seed tombstoned row: %v", err)
	}

	if expired, err := f.sync.ProcessDueBanExpiries(ctx, time.Now()); err != nil || expired != 0 {
		t.Fatalf("expiry = %d, err = %v, want 0/<nil> (lifecycle-owned rows stand down)", expired, err)
	}
	for _, id := range []int64{deletionID, tombID} {
		if _, status, _ := durableUserRow(t, f.pool, id); status != StatusBanned {
			t.Fatalf("user %d status = %d, want still banned", id, status)
		}
		if got := durableIntentCount(t, f.pool, id, "pending"); got != 0 {
			t.Fatalf("user %d pending intents = %d, want 0", id, got)
		}
	}
}

func TestManualUnbanClearsDeadline(t *testing.T) {
	f := newDurableSyncFixture(t)
	ctx := context.Background()
	userID, publicID := f.seedUser(t, "ban-clear", "auth-ban-clear", "BCLR01", "user", StatusActive)

	if _, err := f.commands.Ban(ctx, identityAdminActor(t), publicID, IdentityAdminBanInput{ExpiresInDays: 7}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, err := f.commands.Unban(ctx, identityAdminActor(t), publicID); err != nil {
		t.Fatalf("manual unban: %v", err)
	}
	if deadline := durableBanDeadline(t, f, userID); deadline != nil {
		t.Fatalf("ban deadline = %v, want cleared by manual unban", deadline)
	}
	if expired, err := f.sync.ProcessDueBanExpiries(ctx, time.Now().Add(30*24*time.Hour)); err != nil || expired != 0 {
		t.Fatalf("expiry = %d, err = %v, want 0/<nil> long after", expired, err)
	}
}
