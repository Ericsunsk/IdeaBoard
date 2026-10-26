package tests

// Billing Account refund/dispute convergence (#920).
//
// External top-up refunds converge on the order's fixed payer Billing Account:
// PurchaseRefundHold, PurchaseRefund, PaymentDisputeDebit/Restore and the
// related Restrictions all address order.BillingAccountID, never the
// operator's personal account. Member exits and management changes never move
// the obligation; duplicate notifications never double-settle; insufficient
// Ideas follow the existing policy without touching other accounts.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	platformevents "ideaboard/api/internal/events"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentcredentials "ideaboard/api/internal/payments/application/credentials"
	"ideaboard/api/internal/payments/application/hints"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"
)

func createCreditedOrderForPayer(t *testing.T, pool *pgxpool.Pool, repo *testPaymentStores, payer, operator int64, publicOrderNumber, chargeID string, quantity int64, paidAt time.Time) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		configuration = publishTestStripeConfiguration(t, repo, "acct_920", "prod_920")
	}
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(),
		UserID: operator, BillingAccountID: payer,
		ClientIdempotencyKey: "920-" + publicOrderNumber, RequestHash: "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID: configuration.CredentialID, StripeAccountID: "acct_920", StripeProductID: "prod_920",
		IdeaQuantity:        quantity,
		TermsVersion:        paymentsdomain.CurrentTermsVersion,
		RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL:       "https://app.example.com/user/boards",
		ExpiresAt:           paidAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_920_" + publicOrderNumber[len(publicOrderNumber)-4:],
		StripeSessionID: "cs_920_" + publicOrderNumber[len(publicOrderNumber)-4:],
		CheckoutURL:     "https://checkout.stripe.example/920", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	paid, _, err := repo.RecordPaidPaymentOrder(t.Context(), paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_920_" + chargeID, StripeChargeID: chargeID, IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: quantity, PaidTaxMinor: 25, PaidTotalMinor: quantity + 25, PaidCurrency: "usd",
		ReceiptURL: "https://pay.stripe.com/receipts/920", PaidAt: paidAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	credited, err := repo.CreditPaidPaymentOrder(t.Context(), paid.ID, paidAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return credited.Order
}

func walletAvailable(t *testing.T, pool *pgxpool.Pool, accountID int64) int64 {
	t.Helper()
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), accountID)
	if err != nil {
		t.Fatalf("get wallet %d: %v", accountID, err)
	}
	return wallet.Available
}

func walletReserved(t *testing.T, pool *pgxpool.Pool, accountID int64) int64 {
	t.Helper()
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), accountID)
	if err != nil {
		t.Fatalf("get wallet %d: %v", accountID, err)
	}
	return wallet.Reserved
}

func TestBillingAccountRefundSettlesOnPayerNotOperator(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	operatorBefore := walletAvailable(t, pool, paymentBillingAccount(t, pool, 42))

	order := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920A00000100", "ch_920_payer", 500, now.Add(-time.Hour))
	if order.BillingAccountID != payer || order.UserID != 42 {
		t.Fatalf("order attribution account=%d user=%d, want %d/42", order.BillingAccountID, order.UserID, payer)
	}

	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "920-payer-refund-1", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := walletReserved(t, pool, payer); got != 200 {
		t.Fatalf("payer reserved = %d, want 200", got)
	}
	if got := walletAvailable(t, pool, paymentBillingAccount(t, pool, 42)); got != operatorBefore {
		t.Fatalf("operator available moved to %d, want %d", got, operatorBefore)
	}
	// Funds fact lands on the payer account.
	var holdAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, refund.CreditHoldTransactionID).Scan(&holdAccount); err != nil {
		t.Fatal(err)
	}
	if holdAccount != payer {
		t.Fatalf("refund hold account = %d, want payer %d", holdAccount, payer)
	}

	settled, err := repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now.Add(time.Minute))
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("settle refund=%+v err=%v", settled, err)
	}
	var refundAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, settled.CreditTransactionID).Scan(&refundAccount); err != nil {
		t.Fatal(err)
	}
	if refundAccount != payer {
		t.Fatalf("purchase_refund account = %d, want payer %d", refundAccount, payer)
	}
	if got := walletAvailable(t, pool, payer); got != 300 {
		t.Fatalf("payer available = %d, want 300", got)
	}
	if got := walletAvailable(t, pool, paymentBillingAccount(t, pool, 42)); got != operatorBefore {
		t.Fatalf("operator available after settle = %d, want %d", got, operatorBefore)
	}
}

func TestBillingAccountRefundFailedReleasesOnPayer(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")

	order := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920B00000100", "ch_920_fail", 500, now.Add(-time.Hour))
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "920-payer-fail-1", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := repo.MarkPaymentRefundFailed(t.Context(), refund.ID, "provider rejected", now.Add(time.Minute))
	if err != nil || failed.Status != paymentsdomain.PaymentRefundFailed {
		t.Fatalf("failed refund=%+v err=%v", failed, err)
	}
	if got := walletAvailable(t, pool, payer); got != 500 {
		t.Fatalf("payer available after release = %d, want 500", got)
	}
	if got := walletReserved(t, pool, payer); got != 0 {
		t.Fatalf("payer reserved after release = %d, want 0", got)
	}
	// Duplicate failure converges idempotently without a second release.
	again, err := repo.MarkPaymentRefundFailed(t.Context(), refund.ID, "provider rejected", now.Add(2*time.Minute))
	if err != nil || again.Status != paymentsdomain.PaymentRefundFailed {
		t.Fatalf("duplicate fail=%+v err=%v", again, err)
	}
}

func TestBillingAccountRefundCumulativeCapAndDuplicateExternalIdempotent(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 14, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	order := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920C00000100", "ch_920_cap", 500, now.Add(-time.Hour))

	first, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
		IdempotencyKey: "920-cap-1", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundSucceeded(t.Context(), first.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Cumulative cap: only 200 remain.
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 201,
		IdempotencyKey: "920-cap-over", Now: now.Add(2 * time.Minute),
	}); !errors.Is(err, paymentcontracts.ErrRefundAmountExceeds) {
		t.Fatalf("over-cap error = %v, want ErrRefundAmountExceeds", err)
	}
	second, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "920-cap-2", Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Same key + same quantity replays; same key + different quantity conflicts.
	if _, replayed, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "920-cap-2", Now: now.Add(3 * time.Minute),
	}); err != nil || !replayed {
		t.Fatalf("replay replayed=%v err=%v", replayed, err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 199,
		IdempotencyKey: "920-cap-2", Now: now.Add(3 * time.Minute),
	}); !errors.Is(err, paymentcontracts.ErrRefundIdempotency) {
		t.Fatalf("idempotency conflict err = %v", err)
	}
	_ = second

	// Dashboard duplicate delivery replays the durable fact without a second hold.
	provider := paymentports.StripeRefund{ID: "re_920_dup", ChargeID: "ch_920_cap", AmountMinor: 50, Currency: "usd", Status: "succeeded"}
	_ = provider
	disputeOrder := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920C00000200", "ch_920_dup", 500, now.Add(-time.Hour))
	_ = disputeOrder
	extProvider := paymentports.StripeRefund{ID: "re_920_ext", ChargeID: "ch_920_dup", AmountMinor: 100, Currency: "usd", Status: "succeeded"}
	ext, err := repo.CreateExternalPaymentRefund(t.Context(), extProvider, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	dup, err := repo.CreateExternalPaymentRefund(t.Context(), extProvider, now.Add(5*time.Minute))
	if err != nil || dup.ID != ext.ID {
		t.Fatalf("duplicate external id=%q want %q err=%v", dup.ID, ext.ID, err)
	}
	var holds int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM credit_transactions WHERE business_reference_type = 'payment_refund' AND business_reference_id = $1 AND command_type = 'hold'`, ext.ID).Scan(&holds); err != nil || holds != 1 {
		t.Fatalf("hold rows after duplicate=%d err=%v", holds, err)
	}
	var holdAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, ext.CreditHoldTransactionID).Scan(&holdAccount); err != nil {
		t.Fatal(err)
	}
	if holdAccount != payer {
		t.Fatalf("external hold account = %d, want payer %d", holdAccount, payer)
	}
}

func TestBillingAccountDisputeDebitRestoreOnPayer(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 15, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	operatorAccount := paymentBillingAccount(t, pool, 42)
	operatorBefore := walletAvailable(t, pool, operatorAccount)

	_ = createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920D00000100", "ch_920_dispute", 500, now.Add(-time.Hour))
	dispute, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_920_payer", Status: "under_review", ChargeID: "ch_920_dispute", PaymentIntentID: "pi_920_dispute",
		AmountMinor: 525, Currency: "usd", CreatedAt: now,
	}, now)
	if err != nil || !changed {
		t.Fatalf("dispute=%+v changed=%v err=%v", dispute, changed, err)
	}
	if dispute.DebitedIdeaQuantity != 500 || dispute.UnrecoveredCredits != 0 || dispute.RestrictionID == "" {
		t.Fatalf("open dispute=%+v", dispute)
	}
	// Debit + restriction land on the payer account.
	var debitAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, dispute.DebitTransactionID).Scan(&debitAccount); err != nil {
		t.Fatal(err)
	}
	if debitAccount != payer {
		t.Fatalf("dispute debit account = %d, want payer %d", debitAccount, payer)
	}
	var restrictionAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_wallet_restrictions WHERE restriction_id = $1::uuid`, dispute.RestrictionID).Scan(&restrictionAccount); err != nil {
		t.Fatal(err)
	}
	if restrictionAccount != payer {
		t.Fatalf("restriction account = %d, want payer %d", restrictionAccount, payer)
	}
	if got := walletAvailable(t, pool, payer); got != 0 {
		t.Fatalf("payer available after debit = %d, want 0", got)
	}
	if got := walletAvailable(t, pool, operatorAccount); got != operatorBefore {
		t.Fatalf("operator available moved to %d, want %d", got, operatorBefore)
	}
	// Duplicate delivery replays without a second debit.
	replayed, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_920_payer", Status: "under_review", ChargeID: "ch_920_dispute", PaymentIntentID: "pi_920_dispute",
		AmountMinor: 525, Currency: "usd", CreatedAt: now,
	}, now.Add(time.Minute))
	if err != nil || changed || replayed.DebitTransactionID != dispute.DebitTransactionID {
		t.Fatalf("dispute replay=%+v changed=%v err=%v", replayed, changed, err)
	}
	// Late win restores exactly this dispute's debit on the payer account.
	won, changed, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_920_payer", Status: "won", ChargeID: "ch_920_dispute", PaymentIntentID: "pi_920_dispute",
		AmountMinor: 525, Currency: "usd", CreatedAt: now,
	}, now.Add(2*time.Minute))
	if err != nil || !changed || won.Status != paymentsdomain.PaymentDisputeWon || won.RestoreTransactionID == "" {
		t.Fatalf("won dispute=%+v changed=%v err=%v", won, changed, err)
	}
	var restoreAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, won.RestoreTransactionID).Scan(&restoreAccount); err != nil {
		t.Fatal(err)
	}
	if restoreAccount != payer {
		t.Fatalf("restore account = %d, want payer %d", restoreAccount, payer)
	}
	if got := walletAvailable(t, pool, payer); got != 500 {
		t.Fatalf("payer available after restore = %d, want 500", got)
	}
}

func TestBillingAccountRefundDisputeSurviveMemberExit(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 16, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")

	order := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920E00000100", "ch_920_exit", 500, now.Add(-time.Hour))
	// Operator 42 leaves before settlement.
	if _, err := pool.Exec(t.Context(), `DELETE FROM billing_account_members WHERE billing_account_id = $1 AND user_id = 42`, payer); err != nil {
		t.Fatal(err)
	}
	// Refund still settles on the fixed payer account.
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), paymentrefund.CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "920-exit-refund", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now.Add(time.Minute))
	if err != nil || settled.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("settle after exit=%+v err=%v", settled, err)
	}
	if got := walletAvailable(t, pool, payer); got != 400 {
		t.Fatalf("payer available after exit settle = %d, want 400", got)
	}
	// Dispute still debits the fixed payer account after exit.
	dispute, _, err := repo.ConvergePaymentDispute(t.Context(), paymentports.StripeDispute{
		ID: "dp_920_exit", Status: "under_review", ChargeID: "ch_920_exit", PaymentIntentID: "pi_920_exit",
		AmountMinor: 105, Currency: "usd", CreatedAt: now.Add(2 * time.Minute),
	}, now.Add(2*time.Minute))
	if err != nil || dispute.DebitedIdeaQuantity != 100 {
		t.Fatalf("dispute after exit=%+v err=%v", dispute, err)
	}
	var debitAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE transaction_id = $1`, dispute.DebitTransactionID).Scan(&debitAccount); err != nil {
		t.Fatal(err)
	}
	if debitAccount != payer {
		t.Fatalf("post-exit debit account = %d, want payer %d", debitAccount, payer)
	}
	// Hints still resolve to remaining managers, never the exited operator.
	managers, err := repo.ListBillingAccountManagerIDs(t.Context(), payer)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range managers {
		if id == 42 {
			t.Fatalf("exited operator still in manager audience: %v", managers)
		}
	}
	if len(managers) == 0 {
		t.Fatalf("no remaining managers for payer %d", payer)
	}
	// Payer lookup for hints still resolves the fixed account.
	if accountID, err := repo.GetPaymentOrderPayerAccount(t.Context(), order.ID); err != nil || accountID != payer {
		t.Fatalf("order payer lookup = %d, %v, want %d", accountID, err, payer)
	}
	for _, orderID := range []string{"", uuid.NewString()} {
		if accountID, err := repo.GetPaymentOrderPayerAccount(t.Context(), orderID); err == nil || accountID != 0 {
			t.Fatalf("missing order payer lookup = %d, %v, want error without fallback", accountID, err)
		}
	}
}

func TestBillingAccountExternalRefundShortageTouchesNoOtherAccount(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 17, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	otherAccount := paymentBillingAccount(t, pool, 42)
	otherBefore := walletAvailable(t, pool, otherAccount)

	_ = createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920F00000100", "ch_920_short", 500, now.Add(-time.Hour))
	spendAmount, err := domain.NewCreditAmount(400)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credits.NewService(pool).Administrative().Deduct(t.Context(), ports.DeductionCommand{BillingAccountID: payer, Amount: spendAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "920-shortage-spend", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "42"}, Source: "admin", Reason: "consume Ideas", BusinessReference: domain.BusinessReference{Type: "test", ID: "920-shortage"},
	}}); err != nil {
		t.Fatalf("spend: %v", err)
	}
	if _, err := repo.CreateExternalPaymentRefund(t.Context(), paymentports.StripeRefund{ID: "re_920_short", ChargeID: "ch_920_short", AmountMinor: 300, Currency: "usd", Status: "succeeded"}, now); !errors.Is(err, domain.ErrInsufficientCredits) {
		t.Fatalf("short dashboard refund err = %v, want ErrInsufficientCredits", err)
	}
	if got := walletAvailable(t, pool, otherAccount); got != otherBefore {
		t.Fatalf("other account moved to %d, want %d", got, otherBefore)
	}
	if _, err := repo.RecordPaymentRefundAnomaly(t.Context(), paymentports.StripeRefund{ID: "re_920_short", ChargeID: "ch_920_short", AmountMinor: 300, Currency: "usd", Status: "succeeded"}, "manual recovery", now); err != nil {
		t.Fatal(err)
	}
}

func TestBillingAccountRefundHintsReachOnlyCurrentManagers(t *testing.T) {
	pool := testdb.Open(t)
	repo := newTestPaymentStores(pool)
	now := time.Date(2026, 8, 6, 18, 0, 0, 0, time.UTC)
	ensurePaymentsTestUser(t, pool, 43)
	ensurePaymentsTestUser(t, pool, 44)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	grantBillingAccountManager(t, pool, payer, 44, "admin")

	order := createCreditedOrderForPayer(t, pool, repo, payer, 42, "IBP-20260806-920G00000100", "ch_920_hints", 500, now.Add(-time.Hour))

	resolver, err := paymentcredentials.NewResolver(repo)
	if err != nil {
		t.Fatal(err)
	}
	events := NewEventHub()
	gateway := &refundGatewayFake{createResponse: paymentports.StripeRefund{ID: "re_920_hints", Status: "succeeded"}}
	service, err := paymentrefund.NewService(repo, gateway, hints.NewPublisher(events, repo), func() time.Time { return now }, resolver)
	if err != nil {
		t.Fatal(err)
	}

	operatorStream, unsubscribeOperator := subscribePaymentUserEvents(t, events, 42)
	defer unsubscribeOperator()
	managerStream, unsubscribeManager := subscribePaymentUserEvents(t, events, 43)
	defer unsubscribeManager()
	exitedStream, unsubscribeExited := subscribePaymentUserEvents(t, events, 44)
	defer unsubscribeExited()

	// Manager 44 exits before the refund: hints must follow the current
	// audience, never the stale membership.
	if _, err := pool.Exec(t.Context(), `DELETE FROM billing_account_members WHERE billing_account_id = $1 AND user_id = 44`, payer); err != nil {
		t.Fatal(err)
	}
	refund, err := service.CreatePaymentRefund(context.Background(), paymentcontracts.CreatePaymentRefundCommand{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100, IdempotencyKey: "920-hints-1",
	})
	if err != nil || refund.Status != paymentsdomain.PaymentRefundSucceeded {
		t.Fatalf("CreatePaymentRefund=%+v err=%v", refund, err)
	}
	// Reservation + settlement each fan out once to the two current managers.
	assertHint := func(name string, stream <-chan platformevents.UserEvent) {
		t.Helper()
		select {
		case raw := <-stream:
			event := decodePaymentUserEvent(t, raw)
			if event.BillingAccountID != payer || event.Status != paymentports.PaymentEventRefundUpdated {
				t.Fatalf("%s event=%+v want payer %d refund_updated", name, event, payer)
			}
		default:
			t.Fatalf("%s received no refund hint", name)
		}
		// Second hint (reservation vs settlement) carries the same account.
		select {
		case raw := <-stream:
			event := decodePaymentUserEvent(t, raw)
			if event.BillingAccountID != payer {
				t.Fatalf("%s second event=%+v want payer %d", name, event, payer)
			}
		default:
			t.Fatalf("%s received only one refund hint", name)
		}
	}
	assertHint("operator", operatorStream)
	assertHint("manager", managerStream)
	select {
	case raw := <-exitedStream:
		t.Fatalf("exited member observed refund hint: %+v", raw)
	default:
	}
}
