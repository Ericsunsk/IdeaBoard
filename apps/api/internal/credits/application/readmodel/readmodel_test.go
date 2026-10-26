package readmodel_test

import (
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/credits/testhelpers"
	"ideaboard/api/internal/testdb"
)

func TestReadModelRejectsInvalidInput(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()

	if _, err := service.GetWallet(ctx, 0); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("GetWallet(0) error = %v, want ErrInvalidCommand", err)
	}
	if _, err := service.ListLedger(ctx, domain.ListLedgerInput{}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("ListLedger(empty) error = %v, want ErrInvalidCommand", err)
	}
	if _, err := service.GetAvailableBalances(ctx, []int64{0}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("GetAvailableBalances([0]) error = %v, want ErrInvalidCommand", err)
	}
	if _, err := service.GetWallet(ctx, 424242); !errors.Is(err, domain.ErrWalletNotFound) {
		t.Fatalf("GetWallet(missing) error = %v, want ErrWalletNotFound", err)
	}
	if _, err := service.GetAvailableBalances(ctx, []int64{424242}); !errors.Is(err, domain.ErrWalletBalanceIntegrity) {
		t.Fatalf("GetAvailableBalances(missing) error = %v, want ErrWalletBalanceIntegrity", err)
	}
}

func TestReadModelReturnsCommittedWalletAndBalances(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()

	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94),
		Amount:           testhelpers.MustCreditAmount(t, 100),
		Metadata:         testhelpers.TestMetadata("readmodel:grant:94", "readmodel-grant-94"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 95),
		Amount:           testhelpers.MustCreditAmount(t, 40),
		Metadata:         testhelpers.TestMetadata("readmodel:grant:95", "readmodel-grant-95"),
	}); err != nil {
		t.Fatal(err)
	}

	wallet, err := service.GetWallet(ctx, testhelpers.MustBillingAccount(t, pool, 94))
	if err != nil {
		t.Fatal(err)
	}
	if wallet.Available != 100 || wallet.Reserved != 0 || wallet.Status != domain.WalletStatusActive {
		t.Fatalf("GetWallet() = %+v, want available=100 reserved=0 status=active", wallet)
	}
	if len(wallet.ActiveRestrictions) != 0 {
		t.Fatalf("GetWallet() restrictions = %+v, want empty", wallet.ActiveRestrictions)
	}

	account94 := testhelpers.MustBillingAccount(t, pool, 94)
	account95 := testhelpers.MustBillingAccount(t, pool, 95)
	balances, err := service.GetAvailableBalances(ctx, []int64{account94, account95})
	if err != nil {
		t.Fatal(err)
	}
	if balances[account94] != 100 || balances[account95] != 40 {
		t.Fatalf("GetAvailableBalances() = %v, want per-account 100/40", balances)
	}

	empty, err := service.GetAvailableBalances(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("GetAvailableBalances(nil) = %v, %v, want empty map", empty, err)
	}
}

func TestReadModelListsLedgerNewestFirstWithKeysetPagination(t *testing.T) {
	pool := testdb.Open(t)
	service := credits.NewService(pool)
	ctx := t.Context()

	if _, err := service.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94),
		Amount:           testhelpers.MustCreditAmount(t, 100),
		Metadata:         testhelpers.TestMetadata("readmodel:ledger:grant", "readmodel-ledger-grant"),
	}); err != nil {
		t.Fatal(err)
	}
	hold, err := service.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94),
		Amount:           testhelpers.MustCreditAmount(t, 30),
		Metadata:         testhelpers.TestMetadata("readmodel:ledger:hold", "readmodel-ledger-hold"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Settlement().Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  testhelpers.MustBillingAccount(t, pool, 94),
		HoldTransactionID: hold.TransactionID,
		Amount:            testhelpers.MustCreditAmount(t, 30),
		Metadata:          testhelpers.TestMetadata("readmodel:ledger:release", "readmodel-ledger-release"),
	}); err != nil {
		t.Fatal(err)
	}

	first, err := service.ListLedger(ctx, domain.ListLedgerInput{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Transactions) != 2 || first.NextBeforeSequence == 0 {
		t.Fatalf("first page = %+v, want 2 transactions with next cursor", first)
	}
	if first.Transactions[0].SequenceID <= first.Transactions[1].SequenceID {
		t.Fatalf("ledger not newest-first: %+v", first.Transactions)
	}
	for _, transaction := range first.Transactions {
		if len(transaction.Postings) != 2 {
			t.Fatalf("transaction %s postings = %+v, want 2 balanced postings", transaction.TransactionID, transaction.Postings)
		}
	}
	if first.Transactions[0].Hold == nil {
		t.Fatalf("release transaction missing hold reference: %+v", first.Transactions[0])
	}

	second, err := service.ListLedger(ctx, domain.ListLedgerInput{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 94), BeforeSequence: first.NextBeforeSequence, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Transactions) != 1 || second.NextBeforeSequence != 0 {
		t.Fatalf("second page = %+v, want final transaction without cursor", second)
	}

	empty, err := service.ListLedger(ctx, domain.ListLedgerInput{BillingAccountID: testhelpers.MustBillingAccount(t, pool, 96)})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Transactions) != 0 {
		t.Fatalf("missing wallet ledger = %+v, want empty page", empty)
	}
}
