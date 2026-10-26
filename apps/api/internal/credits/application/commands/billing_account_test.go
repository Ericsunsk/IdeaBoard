package commands_test

import (
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"
)

// TestBillingAccountWalletHoldsSpendsAndConvergesWithSeparatedActor pins
// the #916 contract: the Wallet belongs to exactly one Billing Account,
// every Command addresses that account, and Metadata.Actor records the real
// operator (here a plain user) rather than the funds owner.
func TestBillingAccountWalletHoldsSpendsAndConvergesWithSeparatedActor(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()
	accountID := testhelpers.MustBillingAccount(t, pool, 91601)
	const operatorID = "91601"

	grant, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 100),
		Metadata:         testhelpers.TestMetadata("ba-separation:grant", "ba-separation-grant"),
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if grant.Wallet.BillingAccountID != accountID {
		t.Fatalf("Grant() wallet account = %d, want %d", grant.Wallet.BillingAccountID, accountID)
	}

	hold, err := service.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 60),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "ba-separation:hold",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: operatorID},
			Source:            "task_submission",
			Reason:            "operator hold on shared account",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_ba_separation"},
		},
	})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	if hold.Wallet.Available != 40 || hold.Wallet.Reserved != 60 {
		t.Fatalf("Hold() wallet = %+v, want available=40 reserved=60", hold.Wallet)
	}

	capture, err := service.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 25),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "ba-separation:capture",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: operatorID},
			Source:            "task_submission",
			Reason:            "operator partial capture",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_ba_separation"},
		},
	})
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if capture.Wallet.Available != 40 || capture.Wallet.Reserved != 35 {
		t.Fatalf("Capture() wallet = %+v, want available=40 reserved=35", capture.Wallet)
	}

	release, err := service.Settlement().Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 35),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "ba-separation:release",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: operatorID},
			Source:            "task_submission",
			Reason:            "operator release remainder",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_ba_separation"},
		},
	})
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if release.Wallet.Available != 75 || release.Wallet.Reserved != 0 {
		t.Fatalf("Release() wallet = %+v, want available=75 reserved=0", release.Wallet)
	}

	page, err := service.ListLedger(ctx, domain.ListLedgerInput{BillingAccountID: accountID, Limit: 10})
	if err != nil {
		t.Fatalf("ListLedger() error = %v", err)
	}
	if len(page.Transactions) != 4 {
		t.Fatalf("ledger transactions = %d, want 4 (grant/hold/capture/release)", len(page.Transactions))
	}
	// The hold/capture/release facts must carry the operator actor, not an
	// implied funds owner: funds identity and operator stay separated.
	for _, transaction := range page.Transactions {
		if transaction.CommandType == domain.CommandGrant {
			continue
		}
		if transaction.Actor.Type != domain.ActorUser || transaction.Actor.ID != operatorID {
			t.Fatalf("ledger %s actor = %+v, want user operator %s", transaction.TransactionID, transaction.Actor, operatorID)
		}
	}
}

// TestBillingAccountHoldConvergesAfterServiceRestart proves terminal
// settlement reads the funds account from durable state: a fresh Service
// instance (no process-local hold map) Captures the same Hold.
func TestBillingAccountHoldConvergesAfterServiceRestart(t *testing.T) {
	pool := testdb.Open(t)
	first := credits.NewService(pool)
	ctx := t.Context()
	accountID := testhelpers.MustBillingAccount(t, pool, 91602)

	if _, err := first.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 50),
		Metadata:         testhelpers.TestMetadata("ba-restart:grant", "ba-restart-grant"),
	}); err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	hold, err := first.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 50),
		Metadata:         testhelpers.TestMetadata("ba-restart:hold", "ba-restart-hold"),
	})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}

	restarted := credits.NewService(pool)
	capture, err := restarted.Settlement().Capture(ctx, ports.CaptureCommand{
		BillingAccountID:  accountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 50),
		Metadata:          testhelpers.TestMetadata("ba-restart:capture", "ba-restart-capture"),
	})
	if err != nil {
		t.Fatalf("Capture(after restart) error = %v", err)
	}
	if capture.Wallet.Available != 0 || capture.Wallet.Reserved != 0 {
		t.Fatalf("Capture() wallet = %+v, want available=0 reserved=0", capture.Wallet)
	}
	if capture.Replayed {
		t.Fatal("fresh Capture() was reported as replayed")
	}
}

// TestBillingAccountCommandsReplayIdempotentlyAndDetectConflicts keeps the
// idempotency and conflict invariants on the account contract: same key +
// same hash replays, same key + different hash conflicts, and a foreign
// account cannot settle another account's Hold.
func TestBillingAccountCommandsReplayIdempotentlyAndDetectConflicts(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()
	accountID := testhelpers.MustBillingAccount(t, pool, 91603)
	otherAccountID := testhelpers.MustBillingAccount(t, pool, 91604)

	metadata := testhelpers.TestMetadata("ba-idem:grant", "ba-idem-grant")
	first, err := service.Administrative().Grant(ctx, ports.GrantCommand{BillingAccountID: accountID, Amount: testhelpers.MustCreditAmount(t, 30), Metadata: metadata})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	replay, err := service.Administrative().Grant(ctx, ports.GrantCommand{BillingAccountID: accountID, Amount: testhelpers.MustCreditAmount(t, 30), Metadata: metadata})
	if err != nil {
		t.Fatalf("Grant(replay) error = %v", err)
	}
	if !replay.Replayed || replay.TransactionID != first.TransactionID {
		t.Fatalf("replay = %+v, want replay of %s", replay, first.TransactionID)
	}
	conflictMetadata := testhelpers.TestMetadata("ba-idem:grant", "ba-idem-grant")
	conflictMetadata.Source = "different-source"
	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{BillingAccountID: accountID, Amount: testhelpers.MustCreditAmount(t, 30), Metadata: conflictMetadata}); err == nil {
		t.Fatal("Grant(conflicting hash) succeeded, want idempotency conflict")
	}

	hold, err := service.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: accountID,
		Amount:           testhelpers.MustCreditAmount(t, 10),
		Metadata:         testhelpers.TestMetadata("ba-idem:hold", "ba-idem-hold"),
	})
	if err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	if _, err := service.Settlement().Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  otherAccountID,
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 10),
		Metadata:          testhelpers.TestMetadata("ba-idem:cross-release", "ba-idem-cross-release"),
	}); err == nil {
		t.Fatal("Release(foreign account hold) succeeded, want hold isolation")
	}
}

// TestPersonalBillingAccountsAreDeterministicAndIsolated pins the migration
// contract: repeated resolution returns the same personal account, distinct
// users never share one, and the personal account is never
// organization-owned (personal funds never inject into an organization).
func TestPersonalBillingAccountsAreDeterministicAndIsolated(t *testing.T) {
	pool := testdb.Open(t)
	ctx := t.Context()

	first := testhelpers.MustBillingAccount(t, pool, 91605)
	second := testhelpers.MustBillingAccount(t, pool, 91605)
	if first != second {
		t.Fatalf("personal account unstable: %d != %d", first, second)
	}
	other := testhelpers.MustBillingAccount(t, pool, 91606)
	if other == first {
		t.Fatalf("distinct users share personal account %d", first)
	}
	var organizationID *int64
	var workspaceID int64
	if err := pool.QueryRow(ctx, `SELECT organization_id, workspace_id FROM billing_accounts WHERE internal_id = $1`, first).Scan(&organizationID, &workspaceID); err != nil {
		t.Fatalf("read personal account error = %v", err)
	}
	if organizationID != nil {
		t.Fatalf("personal account %d is organization-owned", first)
	}
	if workspaceID <= 0 {
		t.Fatalf("personal account %d has no workspace", first)
	}
	var isDefault bool
	var ownerID int64
	if err := pool.QueryRow(ctx, `SELECT is_default, owner_user_id FROM workspaces WHERE internal_id = $1`, workspaceID).Scan(&isDefault, &ownerID); err != nil {
		t.Fatalf("read personal workspace error = %v", err)
	}
	if !isDefault || ownerID != 91605 {
		t.Fatalf("personal workspace default=%t owner=%d, want default owned by 91605", isDefault, ownerID)
	}
}
