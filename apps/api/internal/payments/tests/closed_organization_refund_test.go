package tests

import (
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/tenancy"
	tenancypostgres "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

// Closure ends access and spending, not obligations to refund when required
// by law or an express purchase promise. Both refund entry points must settle
// through the original payer without reactivating or transferring the pool.
func TestClosedOrganizationExceptionRefund(t *testing.T) {
	for _, source := range []string{"admin", "dashboard"} {
		for _, succeeded := range []bool{true, false} {
			name := source + "/failed"
			if succeeded {
				name = source + "/succeeded"
			}
			t.Run(name, func(t *testing.T) {
				db := testdb.Open(t)
				repo := newTestPaymentStores(db)
				tenancyStore := tenancypostgres.NewStore(db, nil)
				org, err := tenancyStore.CreateOrganization(t.Context(), 42, "refund-obligation")
				if err != nil {
					t.Fatal(err)
				}
				var accountID int64
				if err := db.QueryRow(t.Context(), `SELECT internal_id FROM billing_accounts WHERE organization_id = $1`, org.InternalID).Scan(&accountID); err != nil {
					t.Fatal(err)
				}
				now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
				order := createCreditedRefundOrderForAccount(t, repo, accountID, now.Add(-30*24*time.Hour))
				personalID := paymentBillingAccount(t, db, 42)
				var personalTransactionsBefore int
				if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE billing_account_id = $1`, personalID).Scan(&personalTransactionsBefore); err != nil {
					t.Fatal(err)
				}
				if _, err := tenancyStore.CloseOrganization(t.Context(), org.PublicID, 42, "permanent closure"); err != nil {
					t.Fatal(err)
				}

				var refund paymentsdomain.PaymentRefund
				if source == "admin" {
					input := paymentrefund.CreatePendingPaymentRefundInput{
						ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
						IdempotencyKey: "closed-org-exception", Now: now,
					}
					var replayed bool
					refund, replayed, err = repo.CreatePendingPaymentRefund(t.Context(), input)
					if err != nil || replayed {
						t.Fatalf("Admin exception refund = %+v, replayed = %v, error = %v", refund, replayed, err)
					}
					if replay, replayed, err := repo.CreatePendingPaymentRefund(t.Context(), input); err != nil || !replayed || replay.ID != refund.ID {
						t.Fatalf("Admin replay = %+v, replayed = %v, error = %v", replay, replayed, err)
					}
				} else {
					provider := paymentports.StripeRefund{ID: "re_closed_org", ChargeID: "ch_refund", AmountMinor: 105, Currency: "usd", Status: "pending"}
					refund, err = repo.CreateExternalPaymentRefund(t.Context(), provider, now)
					if err != nil {
						t.Fatalf("Dashboard exception refund: %v", err)
					}
					if replay, err := repo.CreateExternalPaymentRefund(t.Context(), provider, now); err != nil || replay.ID != refund.ID {
						t.Fatalf("Dashboard replay = %+v, error = %v", replay, err)
					}
				}
				service := credits.NewService(db)
				wallet, err := service.GetWallet(t.Context(), accountID)
				if err != nil || wallet.Status != domain.WalletStatusFrozen || wallet.Available != 400 || wallet.Reserved != 100 {
					t.Fatalf("reserved closed Wallet = %+v, error = %v", wallet, err)
				}
				if succeeded {
					refund, err = repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now.Add(time.Minute))
					if err == nil {
						_, err = repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now.Add(2*time.Minute))
					}
				} else {
					refund, err = repo.MarkPaymentRefundFailed(t.Context(), refund.ID, "provider rejected", now.Add(time.Minute))
				}
				if err != nil {
					t.Fatalf("settle exception refund: %v", err)
				}
				wantAvailable := int64(500)
				if succeeded {
					wantAvailable = 400
				}
				wallet, err = service.GetWallet(t.Context(), accountID)
				if err != nil || wallet.Status != domain.WalletStatusFrozen || wallet.Available != wantAvailable || wallet.Reserved != 0 {
					t.Fatalf("settled closed Wallet = %+v, error = %v", wallet, err)
				}
				amount, err := domain.NewCreditAmount(1)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.Settlement().Hold(t.Context(), ports.HoldCommand{BillingAccountID: accountID, Amount: amount, Metadata: domain.CommandMetadata{
					IdempotencyKey: "closed-org-spending", Actor: domain.Actor{Type: domain.ActorService, ID: "test"}, Source: "test", Reason: "spending remains forbidden", BusinessReference: domain.BusinessReference{Type: "test", ID: "closed-org"},
				}}); !errors.Is(err, domain.ErrWalletFrozen) {
					t.Fatalf("spending on closed pool = %v, want frozen", err)
				}
				if _, err := tenancyStore.CreateOrganizationWorkspace(t.Context(), org.PublicID, 42, "blocked"); !errors.Is(err, tenancy.ErrOrganizationClosed) {
					t.Fatalf("closed Organization access = %v", err)
				}
				var personalTransactions, holds int
				var ledgerTotal int64
				if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE billing_account_id = $1`, personalID).Scan(&personalTransactions); err != nil || personalTransactions != personalTransactionsBefore {
					t.Fatalf("Personal transactions = %d, want %d, error = %v", personalTransactions, personalTransactionsBefore, err)
				}
				if err := db.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE billing_account_id = $1 AND command_type = 'hold'`, accountID).Scan(&holds); err != nil || holds != 1 {
					t.Fatalf("refund holds = %d, error = %v", holds, err)
				}
				if err := db.QueryRow(t.Context(), `SELECT COALESCE(sum(postings.amount), 0)::bigint FROM credit_postings postings JOIN credit_transactions transactions USING (transaction_id) WHERE transactions.billing_account_id = $1`, accountID).Scan(&ledgerTotal); err != nil || ledgerTotal != 0 {
					t.Fatalf("Ledger total = %d, error = %v", ledgerTotal, err)
				}
			})
		}
	}
}
