package workspacetasks

// Personal payer=actor tracer (#974, part of #973).
//
// Fresh-database proof that a collaborator's Personal Task spend bills
// the actor, never the workspace owner: Quote/Hold/Task rows carry the
// actor's own Personal Billing Account, the ledger Hold Actor is the
// submitter, the owner's account shows zero ledger rows, and terminal
// Fail releases back to the actor's account.
//
// This test is red until the settlement payer switches from the
// workspace-owner account to the actor (fail-closed, no fallback).

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tenancy/funds"

	"github.com/jackc/pgx/v5/pgtype"
)

// seedPersonalCollaborator provisions a second user with their own funded
// Personal account and adds them as a member of the workspace. It returns
// the collaborator's user id and Personal Billing Account id.
func seedPersonalCollaborator(t *testing.T, env *atomicEnv) (userID, accountID int64) {
	t.Helper()
	ctx := context.Background()
	storageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	var collaboratorID int64
	if err := env.pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('payer-actor-collaborator', $1, 'user', 1) RETURNING id`,
		storageUID,
	).Scan(&collaboratorID); err != nil {
		t.Fatalf("seed collaborator user: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'member' FROM workspaces WHERE public_id=$1`,
		env.ws, collaboratorID); err != nil {
		t.Fatalf("seed collaborator membership: %v", err)
	}
	queries := dbsqlc.New(env.pool)
	def, err := funds.EnsureDefaultWorkspaceWithQueries(ctx, queries, collaboratorID)
	if err != nil {
		t.Fatalf("provision collaborator default workspace: %v", err)
	}
	if err := funds.EnsureWorkspaceAccountWithQueries(ctx, queries, def.InternalID, collaboratorID); err != nil {
		t.Fatalf("provision collaborator personal account: %v", err)
	}
	row, err := queries.GetBillingAccountByWorkspaceID(ctx, pgtype.Int8{Int64: def.InternalID, Valid: true})
	if err != nil {
		t.Fatalf("resolve collaborator personal account: %v", err)
	}
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.credits.LifecycleInTx(tx).ProvisionAccountWallet(ctx, row.InternalID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision collaborator wallet: %v", err)
	}
	grantAmount, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := env.credits.GrantsInTx(tx).Grant(ctx, ports.GrantCommand{BillingAccountID: row.InternalID, Amount: grantAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "payer-actor-seed-" + randomAtomicHex(t, 4),
		Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "payer actor seed",
		BusinessReference: domain.BusinessReference{Type: "test", ID: "payer-actor-seed"},
	}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant collaborator funds: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return collaboratorID, row.InternalID
}

func collaboratorWallet(t *testing.T, env *atomicEnv, accountID int64) (available, reserved int64) {
	t.Helper()
	details, err := env.credits.GetWallet(context.Background(), accountID)
	if err != nil {
		t.Fatalf("read collaborator wallet: %v", err)
	}
	return details.Available, details.Reserved
}

// TestPersonalCollaboratorSubmitBillsActorNotOwner proves the payer is
// the actor: every billing row names the collaborator's Personal account,
// the ledger Hold Actor is the collaborator, and the owner's account
// carries no ledger rows at all.
func TestPersonalCollaboratorSubmitBillsActorNotOwner(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "payer-actor-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("collaborator Create: %v", err)
	}
	_ = env.service.StagedEvents()

	if created.OwnerUserID != collaboratorID {
		t.Fatalf("task submitter = %d, want the actor %d", created.OwnerUserID, collaboratorID)
	}
	if created.BillingAccountID != collaboratorAccount {
		t.Fatalf("task billing account = %d, want the actor account %d (owner account %d must not be billed)",
			created.BillingAccountID, collaboratorAccount, env.accountID)
	}
	if created.BillingSnapshot.BillingAccountID != collaboratorAccount {
		t.Fatalf("snapshot billing account = %d, want the actor account %d",
			created.BillingSnapshot.BillingAccountID, collaboratorAccount)
	}
	stored, err := env.store.GetTask(ctx, env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BillingAccountID != collaboratorAccount || stored.OwnerUserID != collaboratorID {
		t.Fatalf("stored triple = (%d, %d), want (%d, %d)",
			stored.BillingAccountID, stored.OwnerUserID, collaboratorAccount, collaboratorID)
	}
	holdID := created.BillingSnapshot.HoldTransactionID
	if holdID == "" {
		t.Fatal("snapshot carries no hold transaction id")
	}
	var holdAccount int64
	var holdActorType, holdActorID string
	if err := env.pool.QueryRow(ctx,
		`SELECT billing_account_id, actor_type, actor_id FROM credit_transactions WHERE transaction_id = $1`,
		holdID).Scan(&holdAccount, &holdActorType, &holdActorID); err != nil {
		t.Fatalf("read hold ledger row: %v", err)
	}
	if holdAccount != collaboratorAccount {
		t.Fatalf("hold billing account = %d, want the actor account %d", holdAccount, collaboratorAccount)
	}
	if holdActorType != string(domain.ActorUser) || holdActorID != fmt.Sprint(collaboratorID) {
		t.Fatalf("hold actor = (%q, %q), want (user, %d)", holdActorType, holdActorID, collaboratorID)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, env.accountID); got != 0 {
		t.Fatalf("owner ledger rows = %d, want 0 (owner pays nothing)", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 10 {
		t.Fatalf("actor wallet = (%d, %d), want (990, 10)", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
}

// TestPersonalSkipsQuotaCheck proves Personal scopes bill with no
// quota concept: tight caps on both the actor's and the owner's accounts
// still let the actor's over-cap submit settle. Pre-#974 this submit fails
// with ErrMemberQuotaExhausted against the owner-pooled account; without
// the settlement skip it would fail against the actor's own cap.
func TestPersonalSkipsQuotaCheck(t *testing.T) {
	env := openAtomicEnv(t, 10)
	quotas := memberquotas.NewStore(env.pool)
	env.serviceDeps.Quotas = quotas
	env.service = NewService(env.serviceDeps)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)
	tight := int64(15)
	setDailyQuota(t, quotas, env.accountID, collaboratorID, tight)
	setDailyQuota(t, quotas, collaboratorAccount, collaboratorID, tight)

	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 2, Resolution: "1k",
	}, "payer-actor-skip-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("over-cap Personal Create err = %v, want success (no quota on Personal)", err)
	}
	_ = env.service.StagedEvents()
	if created.BillingAccountID != collaboratorAccount {
		t.Fatalf("task billing account = %d, want the actor account %d", created.BillingAccountID, collaboratorAccount)
	}
	if created.BillingSnapshot.TotalCredits != 20 {
		t.Fatalf("snapshot total = %d, want 20 (over the skipped 15 cap)", created.BillingSnapshot.TotalCredits)
	}
}

// TestPersonalCollaboratorFailReleasesToActor proves terminal Fail
// converges on the actor's account: the Hold releases back to the
// collaborator and the owner's account stays untouched.
func TestPersonalCollaboratorFailReleasesToActor(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	created, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "payer-actor-fail-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("collaborator Create: %v", err)
	}
	_ = env.service.StagedEvents()

	failed, err := env.service.Fail(ctx, env.ws, created.ID, "provider exploded")
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if failed.BillingAccountID != collaboratorAccount {
		t.Fatalf("failed billing account = %d, want the actor account %d", failed.BillingAccountID, collaboratorAccount)
	}
	_ = env.service.StagedEvents()

	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, env.accountID); got != 0 {
		t.Fatalf("owner ledger rows after fail = %d, want 0", got)
	}
	if got := env.ledgerCount(t, `command_type = 'release' AND billing_account_id = $1`, collaboratorAccount); got != 1 {
		t.Fatalf("actor release ledger rows = %d, want 1", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 1000 || reserved != 0 {
		t.Fatalf("actor wallet after fail = (%d, %d), want (1000, 0)", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet after fail = (%d, %d), want (1000, 0)", available, reserved)
	}
}
