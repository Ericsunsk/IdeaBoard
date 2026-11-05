package vision

import (
	"context"
	"errors"
	"testing"

	creditsdomain "ideaboard/api/internal/credits/domain"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"
)

// Fresh DB closing assertions for #981 (part of #973), Vision side.
//
// The tracer (#977) pins single-actor success; this file closes the
// cross-member and fail-closed corners: A/B each extract on their own
// payer with zero cross pollution, underfunded-then-funded retry on the
// same actor payer, and closed-Organization Extract refused with no
// operation row.
//
// Tests SKIP without API_TEST_DATABASE_DSN (house convention).

func TestVisionClosingCrossMemberIsolation(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-close-owner")
	member := env.createUser(t, "vision-close-member")
	ws := "ws_visionclose01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})

	var actorAccount, ownerAccount int64
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, member).Scan(&actorAccount); err != nil {
		t.Fatalf("resolve actor account: %v", err)
	}
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, owner).Scan(&ownerAccount); err != nil {
		t.Fatalf("resolve owner account: %v", err)
	}
	env.fundAccount(t, actorAccount, 1000)
	env.fundAccount(t, ownerAccount, 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "close dunes"}})
	ownerGot, err := service.Extract(ctx, ws, owner, "vision-close-owner-1", visionTestInput())
	if err != nil {
		t.Fatalf("owner Extract: %v", err)
	}
	memberGot, err := service.Extract(ctx, ws, member, "vision-close-member-1", visionTestInput())
	if err != nil {
		t.Fatalf("member Extract: %v", err)
	}
	if ownerRow := env.operationRow(t, ownerGot.OperationID); ownerRow.account != ownerAccount || ownerRow.userID != owner {
		t.Fatalf("owner operation = (user %d, account %d), want (%d,%d)", ownerRow.userID, ownerRow.account, owner, ownerAccount)
	}
	if memberRow := env.operationRow(t, memberGot.OperationID); memberRow.account != actorAccount || memberRow.userID != member {
		t.Fatalf("member operation = (user %d, account %d), want (%d,%d)", memberRow.userID, memberRow.account, member, actorAccount)
	}
	if env.ledgerCount(t, "hold", ownerAccount) != 1 || env.ledgerCount(t, "capture", ownerAccount) != 1 {
		t.Fatal("owner ledger must show exactly own hold+capture")
	}
	if env.ledgerCount(t, "hold", actorAccount) != 1 || env.ledgerCount(t, "capture", actorAccount) != 1 {
		t.Fatal("actor ledger must show exactly own hold+capture")
	}
	if env.ledgerCount(t, "release", ownerAccount) != 0 || env.ledgerCount(t, "release", actorAccount) != 0 {
		t.Fatal("cross-member test must not release")
	}
}

func TestVisionClosingInsufficientThenFundedRetry(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-close-retry-owner")
	member := env.createUser(t, "vision-close-retry-member")
	ws := "ws_visionclose02"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	actorAccount := env.personalAccountFor(t, member)
	ownerAccount := env.personalAccountFor(t, owner)
	env.fundAccount(t, actorAccount, 3)
	env.fundAccount(t, ownerAccount, 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "retry dunes"}})
	if _, err := service.Extract(ctx, ws, member, "vision-close-retry-1", visionTestInput()); !errors.Is(err, creditsdomain.ErrInsufficientCredits) {
		t.Fatalf("underfunded Extract err = %v, want insufficient-credits", err)
	}
	if env.operationCount(t, ws, "vision-close-retry-1") != 0 {
		t.Fatal("underfunded call persisted an operation")
	}
	if env.ledgerCount(t, "hold", actorAccount) != 0 || env.ledgerCount(t, "hold", ownerAccount) != 0 {
		t.Fatal("underfunded call moved a wallet")
	}

	env.fundAccount(t, actorAccount, 1000)
	got, err := service.Extract(ctx, ws, member, "vision-close-retry-2", visionTestInput())
	if err != nil {
		t.Fatalf("funded retry Extract: %v", err)
	}
	row := env.operationRow(t, got.OperationID)
	if row.account != actorAccount || row.userID != member {
		t.Fatalf("retry operation = (user %d, account %d), want actor (%d,%d)", row.userID, row.account, member, actorAccount)
	}
	if env.ledgerCount(t, "hold", ownerAccount) != 0 || env.ledgerCount(t, "capture", ownerAccount) != 0 {
		t.Fatal("retry moved the owner's account")
	}
}

func TestVisionClosingOrgClosedRefusesExtract(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-close-org-owner")
	env.createOrg(t, "org_visionclose01", map[int64]string{owner: "owner"})
	env.createOrgWorkspace(t, "org_visionclose01", "ws_visioncloseorg01", map[int64]string{owner: "owner"})
	shared := env.billingAccountFor(t, "ws_visioncloseorg01")
	env.fundAccount(t, shared, 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "closed dunes"}})
	if _, err := service.Extract(ctx, "ws_visioncloseorg01", owner, "vision-close-org-1", visionTestInput()); err != nil {
		t.Fatalf("open org Extract: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE organizations SET closed_at = now() WHERE public_id = 'org_visionclose01'`); err != nil {
		t.Fatalf("close org: %v", err)
	}
	if _, err := service.Extract(ctx, "ws_visioncloseorg01", owner, "vision-close-org-2", visionTestInput()); !errors.Is(err, ErrVisionWorkspaceNotFound) {
		t.Fatalf("closed-org Extract err = %v, want workspace-not-found", err)
	}
	if env.operationCount(t, "ws_visioncloseorg01", "vision-close-org-2") != 0 {
		t.Fatal("closed-org call persisted an operation")
	}
}
