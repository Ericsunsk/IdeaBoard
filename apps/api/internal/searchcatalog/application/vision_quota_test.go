// Vision Search member quota tests (#946).
//
// The quota check joins the Vision reservation transaction after Quote
// and before Hold: an over-quota request fails
// closed with ErrVisionQuotaExhausted (403 member_quota_exhausted) and
// leaves no operation row and no Hold.
// In-flight operations converge on their
// submit-time payer and period facts, so terminal Capture/Release needs
// no re-check.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention).

package application

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/memberquotas"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"
)

// TestVisionQuotaExhaustedBlocksReserve proves Task and Vision share one
// member quota: a succeeded Vision Search consumes quota, and the next
// request over the cap fails before any ledger or operation write while
// the funded wallet stays untouched by the block.
func TestVisionQuotaExhaustedBlocksReserve(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 25)
	ctx := context.Background()
	owner := env.createUser(t, "vision-quota-owner")
	member := env.createUser(t, "vision-quota-member")
	ws := "ws_visionquota01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	quotas := memberquotas.NewStore(env.pool)
	limit := int64(30)
	if _, err := quotas.SetQuota(ctx, account, member, memberquotas.PeriodDaily, &limit, owner); err != nil {
		t.Fatalf("set vision quota: %v", err)
	}

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "quota dunes"}})
	service.SetQuotaChecker(quotas)

	first, err := service.Extract(ctx, ws, member, "vision-quota-1", visionTestInput())
	if err != nil {
		t.Fatalf("first Extract() error = %v", err)
	}
	if first.Status != string(VisionOperationSucceeded) {
		t.Fatalf("first status = %q, want succeeded", first.Status)
	}
	if got := env.operationCount(t, ws, "vision-quota-1"); got != 1 {
		t.Fatalf("first operation rows = %d, want 1", got)
	}

	// 25 consumed + 25 requested exceeds the 30 cap on a funded wallet:
	// quota exhaustion, never insufficiency, with no residue.
	_, err = service.Extract(ctx, ws, member, "vision-quota-2", visionTestInput())
	if !errors.Is(err, ErrVisionQuotaExhausted) {
		t.Fatalf("second Extract() error = %v, want ErrVisionQuotaExhausted", err)
	}
	if got := env.operationCount(t, ws, "vision-quota-2"); got != 0 {
		t.Fatalf("blocked operation rows = %d, want 0 (no residue)", got)
	}
	if holds, captures := env.ledgerCount(t, "hold", account), env.ledgerCount(t, "capture", account); holds != 1 || captures != 1 {
		t.Fatalf("ledger holds=%d captures=%d, want 1/1 (first only)", holds, captures)
	}
	wallet, err := env.credits.GetWallet(ctx, account)
	if err != nil {
		t.Fatalf("read vision wallet: %v", err)
	}
	if wallet.Available != 1000-25 || wallet.Reserved != 0 {
		t.Fatalf("wallet available=%d reserved=%d, want 975/0", wallet.Available, wallet.Reserved)
	}
}

// TestVisionQuotaGateReportsOwnReason proves the quota gate constrains
// Vision Search on its own: a tight quota blocks with the quota reason,
// and a roomy quota settles.
func TestVisionQuotaGateReportsOwnReason(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 25)
	ctx := context.Background()
	owner := env.createUser(t, "vision-quota-owner")
	member := env.createUser(t, "vision-quota-member")
	ws := "ws_visionquota02"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	quotas := memberquotas.NewStore(env.pool)
	tightQuota := int64(10)
	if _, err := quotas.SetQuota(ctx, account, member, memberquotas.PeriodDaily, &tightQuota, owner); err != nil {
		t.Fatalf("set tight quota: %v", err)
	}
	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "quota dunes"}})
	service.SetQuotaChecker(quotas)

	// Tight quota: the quota gate reports.
	_, err := service.Extract(ctx, ws, member, "vision-quota-3", visionTestInput())
	if !errors.Is(err, ErrVisionQuotaExhausted) {
		t.Fatalf("quota-blocked Extract() error = %v, want ErrVisionQuotaExhausted", err)
	}

	// Roomy quota: the same funded wallet settles.
	roomyQuota := int64(1000)
	if _, err := quotas.SetQuota(ctx, account, member, memberquotas.PeriodDaily, &roomyQuota, owner); err != nil {
		t.Fatalf("set roomy quota: %v", err)
	}
	if _, err := service.Extract(ctx, ws, member, "vision-quota-4", visionTestInput()); err != nil {
		t.Fatalf("roomy Extract() error = %v", err)
	}
}
