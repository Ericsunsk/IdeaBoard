package vision

import (
	"context"
	"testing"

	searchdomain "ideaboard/api/internal/searchcatalog/domain"
)

// TestVisionPersonalPayerIsActor is the #977 red test (Fresh DB):
// Personal workspace member consumption bills the actor's own Personal
// account; the owner's account sees zero ledger movement, and the
// operation row converges reserve -> settlement on the actor's account.
func TestVisionPersonalPayerIsActor(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-payer-owner")
	member := env.createUser(t, "vision-payer-member")
	ws := "ws_visionpayer01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})

	var actorAccount, ownerAccount int64
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, member).Scan(&actorAccount); err != nil {
		t.Fatalf("resolve actor personal account: %v", err)
	}
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, owner).Scan(&ownerAccount); err != nil {
		t.Fatalf("resolve owner personal account: %v", err)
	}
	if actorAccount == ownerAccount {
		t.Fatal("test setup collides: actor and owner share a personal account")
	}
	env.fundAccount(t, actorAccount, 1000)
	env.fundAccount(t, ownerAccount, 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "actor dunes"}})
	got, err := service.Extract(ctx, ws, member, "vision-payer-1", visionTestInput())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Status != string(VisionOperationSucceeded) {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	row := env.operationRow(t, got.OperationID)
	if row.userID != member {
		t.Fatalf("operation user = %d, want actor %d", row.userID, member)
	}
	if row.account != actorAccount {
		t.Fatalf("operation payer = %d, want actor account %d (owner %d must see zero movement)", row.account, actorAccount, ownerAccount)
	}
	if row.status != "succeeded" {
		t.Fatalf("operation status = %q, want succeeded", row.status)
	}
	if env.ledgerCount(t, "hold", actorAccount) != 1 || env.ledgerCount(t, "capture", actorAccount) != 1 {
		t.Fatalf("actor ledger hold=%d capture=%d, want 1/1",
			env.ledgerCount(t, "hold", actorAccount), env.ledgerCount(t, "capture", actorAccount))
	}
	if env.ledgerCount(t, "hold", ownerAccount) != 0 || env.ledgerCount(t, "capture", ownerAccount) != 0 || env.ledgerCount(t, "release", ownerAccount) != 0 {
		t.Fatalf("owner ledger hold=%d capture=%d release=%d, want 0/0/0",
			env.ledgerCount(t, "hold", ownerAccount), env.ledgerCount(t, "capture", ownerAccount), env.ledgerCount(t, "release", ownerAccount))
	}
	wallet, err := env.credits.GetWallet(ctx, actorAccount)
	if err != nil {
		t.Fatalf("read actor wallet: %v", err)
	}
	if wallet.Available != 1000-7 || wallet.Reserved != 0 {
		t.Fatalf("actor wallet available=%d reserved=%d, want 993/0", wallet.Available, wallet.Reserved)
	}
	ownerWallet, err := env.credits.GetWallet(ctx, ownerAccount)
	if err != nil {
		t.Fatalf("read owner wallet: %v", err)
	}
	if ownerWallet.Available != 1000 || ownerWallet.Reserved != 0 {
		t.Fatalf("owner wallet available=%d reserved=%d, want 1000/0", ownerWallet.Available, ownerWallet.Reserved)
	}
}
