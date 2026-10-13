package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/authidentity"
	"ideaboard/api/internal/identity/users/application/reconciliation"
)

// TestIdentityAdminReconciliationDriftMatrix covers the four drift kinds:
// role mismatch, ban-state mismatch, missing Auth identity, and missing
// local auth link. Every kind produces a finding, and the dry-run phase
// modifies no user data and no Auth data.
func TestIdentityAdminReconciliationDriftMatrix(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	auth := &fakeAuthStateReader{states: make(map[string]authidentity.AuthUserState)}
	linkFixtureUsers(t, pool, auth)

	roleID, _ := seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-role-drift", authUserID: reconAuthID("auth-recon-role"),
		storageUID: "RCROLE", role: "user", status: StatusActive,
	})
	banID, _ := seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-ban-drift", authUserID: reconAuthID("auth-recon-ban"),
		storageUID: "RCBAND", role: "user", status: StatusActive,
	})
	missingID, missingPublicID := seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-auth-gone", authUserID: reconAuthID("auth-recon-gone"),
		storageUID: "RCGONE", role: "user", status: StatusActive,
	})
	linkID, linkPublicID := seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-no-link", authUserID: nil,
		storageUID: "RCNOLK", role: "user", status: StatusActive,
	})

	banExpires := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)
	auth.states["auth-recon-role"] = reconAuthState("auth-recon-role", "admin", false)
	// Ban drift: local active, Auth banned (with metadata the local
	// row cannot store: alert-only, recorded in details).
	auth.states["auth-recon-ban"] = authidentity.AuthUserState{
		AuthUserID: "auth-recon-ban", Role: "user", Banned: true,
		BanExpiresAt: &banExpires,
	}
	// "auth-recon-gone" is absent: Auth 404 while the local row links.
	reconciler := newTestReconciler(pool, auth, false)

	slot := time.Date(2026, 10, 4, 3, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	runID := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, slot)
	if err := reconciler.ExecuteReconciliation(ctx, runID); err != nil {
		t.Fatalf("execute reconciliation: %v", err)
	}

	run, err := reconciler.GetReconciliationRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "completed" || run.FindingCount != 4 {
		t.Fatalf("run = %+v, want completed with 4 findings", run)
	}
	if run.AttemptCount != 1 {
		t.Fatalf("run attempt count = %d, want 1", run.AttemptCount)
	}

	findings, err := reconciler.ListRunFindings(ctx, runID)
	if err != nil {
		t.Fatalf("list run findings: %v", err)
	}
	byType := make(map[string]map[string]any)
	for _, finding := range findings {
		if finding.Status != "open" {
			t.Fatalf("finding %q status = %q, want open", finding.FindingType, finding.Status)
		}
		if finding.Details["dry_run"] != true || finding.Details["applied"] != false || finding.Details["winner"] != "local" {
			t.Fatalf("finding %q details = %v, want dry_run=true applied=false winner=local", finding.FindingType, finding.Details)
		}
		byType[finding.FindingType] = finding.Details
	}
	for _, want := range []string{"role_mismatch", "ban_state_mismatch", "auth_identity_missing", "local_auth_link_missing"} {
		if _, ok := byType[want]; !ok {
			t.Fatalf("missing finding type %q in %v", want, findings)
		}
	}
	if byType["role_mismatch"]["local_role"] != "user" || byType["role_mismatch"]["auth_role"] != "admin" {
		t.Fatalf("role mismatch details = %v, want local=user auth=admin", byType["role_mismatch"])
	}
	if byType["role_mismatch"]["remediation"] != "push_to_auth" {
		t.Fatalf("role mismatch remediation = %v, want push_to_auth", byType["role_mismatch"]["remediation"])
	}

	if byType["ban_state_mismatch"]["auth_ban_expires"] != "2026-10-11T03:00:00Z" {
		t.Fatalf("ban mismatch details = %v, want Auth ban expiry recorded", byType["ban_state_mismatch"])
	}
	if byType["ban_state_mismatch"]["remediation"] != "push_unban_to_auth" {
		t.Fatalf("ban mismatch remediation = %v, want push_unban_to_auth", byType["ban_state_mismatch"]["remediation"])
	}
	if byType["auth_identity_missing"]["auth_found"] != false || byType["auth_identity_missing"]["remediation"] != "manual_review" {
		t.Fatalf("auth-missing details = %v, want manual review", byType["auth_identity_missing"])
	}
	if byType["local_auth_link_missing"]["remediation"] != "manual_review" {
		t.Fatalf("link-missing details = %v, want manual review", byType["local_auth_link_missing"])
	}

	// Dry-run evidence: no local user row changed...
	for _, id := range []int64{roleID, banID, missingID, linkID} {
		if role, status := mustReconUserRow(t, pool, id); role != "user" || status != StatusActive {
			t.Fatalf("user %d changed to role=%q status=%d, dry-run must not modify users", id, role, status)
		}
	}
	// ...and the finding subjects reference the drifted local rows.
	subjects := make(map[string]bool)
	for _, finding := range findings {
		subjects[finding.SubjectID] = true
	}
	for _, publicID := range []string{missingPublicID, linkPublicID} {
		if !subjects[publicID] {
			t.Fatalf("subjects = %v, want local public id %s referenced", subjects, publicID)
		}
	}
	if len(auth.calls) == 0 {
		t.Fatal("auth reader was never called")
	}
	for _, call := range auth.calls {
		if len(call) > 100 {
			t.Fatalf("auth batch size = %d, want at most 100 per contract", len(call))
		}
	}
}

// TestIdentityAdminReconciliationNoDrift proves a converged pair stays
// silent: matching role/ban (including an Auth-side ban expiry on a
// consistently banned user, which is alert-only metadata, not drift)
// yields no findings.
func TestIdentityAdminReconciliationNoDrift(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	auth := &fakeAuthStateReader{states: make(map[string]authidentity.AuthUserState)}
	linkFixtureUsers(t, pool, auth)

	seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-clean-admin", authUserID: reconAuthID("auth-recon-clean-admin"),
		storageUID: "RCCLNA", role: "admin", status: StatusActive,
	})
	seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-clean-banned", authUserID: reconAuthID("auth-recon-clean-banned"),
		storageUID: "RCCLNB", role: "user", status: StatusBanned,
	})

	banExpires := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)
	auth.states["auth-recon-clean-admin"] = reconAuthState("auth-recon-clean-admin", "admin", false)
	auth.states["auth-recon-clean-banned"] = authidentity.AuthUserState{
		AuthUserID: "auth-recon-clean-banned", Role: "user", Banned: true,
		BanExpiresAt: &banExpires,
	}
	reconciler := newTestReconciler(pool, auth, false)

	runID := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC))
	if err := reconciler.ExecuteReconciliation(ctx, runID); err != nil {
		t.Fatalf("execute reconciliation: %v", err)
	}
	run, err := reconciler.GetReconciliationRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "completed" || run.FindingCount != 0 {
		t.Fatalf("run = %+v, want completed with 0 findings", run)
	}
	findings, err := reconciler.ListRunFindings(ctx, runID)
	if err != nil {
		t.Fatalf("list run findings: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none without drift", findings)
	}
}

// TestIdentityAdminReconciliationIsIdempotent proves re-scans never
// duplicate findings: re-executing a completed run is a no-op, and a new
// scheduled run observing the same drift refreshes the same rows instead
// of inserting new ones.
func TestIdentityAdminReconciliationIsIdempotent(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	auth := &fakeAuthStateReader{states: make(map[string]authidentity.AuthUserState)}
	linkFixtureUsers(t, pool, auth)

	seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-idem", authUserID: reconAuthID("auth-recon-idem"),
		storageUID: "RCIDEM", role: "user", status: StatusActive,
	})
	auth.states["auth-recon-idem"] = reconAuthState("auth-recon-idem", "admin", false)
	reconciler := newTestReconciler(pool, auth, false)
	firstSlot := time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC)
	firstRun := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, firstSlot)
	if err := reconciler.ExecuteReconciliation(ctx, firstRun); err != nil {
		t.Fatalf("execute first run: %v", err)
	}
	countAfterFirst, err := reconciler.CountFindings(ctx)
	if err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if countAfterFirst != 1 {
		t.Fatalf("finding count = %d, want 1", countAfterFirst)
	}

	// Re-executing the completed run is a no-op.
	if err := reconciler.ExecuteReconciliation(ctx, firstRun); err != nil {
		t.Fatalf("re-execute completed run: %v", err)
	}
	countAfterRerun, err := reconciler.CountFindings(ctx)
	if err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if countAfterRerun != countAfterFirst {
		t.Fatalf("finding count after rerun = %d, want %d (no duplication)", countAfterRerun, countAfterFirst)
	}
	firstFindings, err := reconciler.ListRunFindings(ctx, firstRun)
	if err != nil {
		t.Fatalf("list first run findings: %v", err)
	}
	if len(firstFindings) != 1 {
		t.Fatalf("first run findings = %d, want 1", len(firstFindings))
	}

	// A new scheduled run observing the same drift refreshes the same row.
	secondRun := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, firstSlot.Add(24*time.Hour))
	if err := reconciler.ExecuteReconciliation(ctx, secondRun); err != nil {
		t.Fatalf("execute second run: %v", err)
	}
	countAfterSecond, err := reconciler.CountFindings(ctx)
	if err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if countAfterSecond != countAfterFirst {
		t.Fatalf("finding count after second run = %d, want %d (dedup by fingerprint)", countAfterSecond, countAfterFirst)
	}
	secondFindings, err := reconciler.ListRunFindings(ctx, secondRun)
	if err != nil {
		t.Fatalf("list second run findings: %v", err)
	}
	if len(secondFindings) != 1 || secondFindings[0].Fingerprint != firstFindings[0].Fingerprint {
		t.Fatalf("second run findings = %+v, want the same fingerprinted finding", secondFindings)
	}
	if secondFindings[0].OccurrenceCount != firstFindings[0].OccurrenceCount+1 {
		t.Fatalf("occurrence count = %d, want re-observation recorded", secondFindings[0].OccurrenceCount)
	}
	if secondFindings[0].LastSeenRunID != secondRun {
		t.Fatalf("last seen run = %d, want %d", secondFindings[0].LastSeenRunID, secondRun)
	}
}

// TestIdentityAdminReconciliationFailsClosedOnAutoRemediate proves the
// opt-in gate: enabling AutoRemediate refuses to run instead of repairing.
func TestIdentityAdminReconciliationFailsClosedOnAutoRemediate(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-gated", authUserID: reconAuthID("auth-recon-gated"),
		storageUID: "RCGATE", role: "user", status: StatusActive,
	})
	auth := &fakeAuthStateReader{states: map[string]authidentity.AuthUserState{
		"auth-recon-gated": reconAuthState("auth-recon-gated", "admin", false),
	}}
	gated := newTestReconciler(pool, auth, true)

	// Schedule with an ungated reconciler so the run row exists, then
	// execute with the gate on: the run must stay queued and untouched.
	ungated := newTestReconciler(pool, auth, false)
	runID := reconRunForSlot(t, ungated, &fakeRunDispatcher{}, time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC))
	if err := gated.ExecuteReconciliation(ctx, runID); !errors.Is(err, reconciliation.ErrAutoRemediationNotSupported) {
		t.Fatalf("execute with AutoRemediate = %v, want ErrAutoRemediationNotSupported", err)
	}
	run, err := ungated.GetReconciliationRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "queued" {
		t.Fatalf("run status = %q, want queued (gate fired before any state change)", run.Status)
	}
	open, err := ungated.ListOpenFindings(ctx)
	if err != nil {
		t.Fatalf("list open findings: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("open findings = %+v, want none after gated refusal", open)
	}
	if len(auth.calls) != 0 {
		t.Fatalf("auth calls = %d, want none (gate fired before the scan)", len(auth.calls))
	}
}

// TestIdentityAdminReconciliationAuthFailureAborts proves an Auth outage
// fails the run instead of persisting misleading findings: without Auth
// data every linked user would misreport as missing.
func TestIdentityAdminReconciliationAuthFailureAborts(t *testing.T) {
	pool := newUserRepoTestDB(t)
	ctx := context.Background()

	seedReconUser(t, pool, reconSeedUser{
		displayName: "recon-outage", authUserID: reconAuthID("auth-recon-outage"),
		storageUID: "RCOUTG", role: "user", status: StatusActive,
	})
	auth := &fakeAuthStateReader{err: errors.New("auth service unavailable")}
	reconciler := newTestReconciler(pool, auth, false)

	runID := reconRunForSlot(t, reconciler, &fakeRunDispatcher{}, time.Date(2026, 10, 4, 3, 0, 0, 0, time.UTC))
	if err := reconciler.ExecuteReconciliation(ctx, runID); err == nil || !strings.Contains(err.Error(), "read auth states") {
		t.Fatalf("execute during Auth outage = %v, want auth read error", err)
	}
	open, err := reconciler.ListOpenFindings(ctx)
	if err != nil {
		t.Fatalf("list open findings: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("open findings = %+v, want none after aborted scan", open)
	}
	if err := reconciler.FailReconciliation(ctx, runID, errors.New("auth service unavailable")); err != nil {
		t.Fatalf("fail run: %v", err)
	}
	run, err := reconciler.GetReconciliationRun(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "failed" {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
}
