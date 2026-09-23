package tests

import (
	"errors"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/testdb"
)

func TestPaidPaymentOrderAndPurchaseCreditConvergeAtomicallyOnce(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedAmount, err := domain.NewCreditAmount(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creditService.Grant(t.Context(), ports.GrantCommand{
		UserID: 42, Amount: seedAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "payment-convergence-seed", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "42"},
			Source: "admin_console", Reason: "seed payment convergence Wallet",
			BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: "payment-convergence-seed"},
		},
	}); err != nil {
		t.Fatalf("seed Wallet: %v", err)
	}

	repo := paymentpostgres.NewRepository(db)
	order := createBoundPaymentOrder(t, repo)
	facts := paidCheckoutFacts(order)
	creditedAt := time.Date(2026, 8, 1, 12, 10, 0, 0, time.UTC)

	const workers = 4
	results := make(chan paymentsdomain.PaymentConvergenceResult, workers)
	errorsSeen := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, creditedAt)
			results <- result
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("ConvergePaidPaymentOrder() error = %v", err)
		}
	}
	var firstCount, replayCount int
	for result := range results {
		if result.Order.Status != paymentsdomain.PaymentOrderCredited || result.AvailableIdeas != 502 {
			t.Fatalf("convergence result = %+v", result)
		}
		if result.Replayed {
			replayCount++
		} else {
			firstCount++
		}
	}
	if firstCount != 1 || replayCount != workers-1 {
		t.Fatalf("first=%d replay=%d", firstCount, replayCount)
	}

	var purchaseCount int
	if err := db.QueryRow(t.Context(), `
SELECT COUNT(*) FROM credit_transactions
WHERE command_type = 'purchase_credit' AND business_reference_id = $1`, order.ID).Scan(&purchaseCount); err != nil {
		t.Fatal(err)
	}
	if purchaseCount != 1 {
		t.Fatalf("purchase ledger transactions = %d, want 1", purchaseCount)
	}
}

func TestPaidPaymentOrderFactConflictRollsBackWithoutCredit(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedAmount, _ := domain.NewCreditAmount(1)
	if _, err := creditService.Grant(t.Context(), ports.GrantCommand{
		UserID: 42, Amount: seedAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "payment-conflict-seed", Actor: domain.Actor{Type: domain.ActorAdmin, ID: "42"},
			Source: "admin_console", Reason: "seed payment convergence Wallet",
			BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: "payment-conflict-seed"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	repo := paymentpostgres.NewRepository(db)
	order := createBoundPaymentOrder(t, repo)
	facts := paidCheckoutFacts(order)
	facts.RequestHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	if _, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, time.Now()); !errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) {
		t.Fatalf("ConvergePaidPaymentOrder() error = %v, want idempotency conflict", err)
	}
	var purchaseCount int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'purchase_credit'`).Scan(&purchaseCount); err != nil {
		t.Fatal(err)
	}
	if purchaseCount != 0 {
		t.Fatalf("purchase ledger transactions = %d, want 0", purchaseCount)
	}
	var status string
	if err := db.QueryRow(t.Context(), `SELECT status FROM payment_orders WHERE id = $1`, order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(paymentsdomain.PaymentOrderCheckoutOpen) {
		t.Fatalf("payment order status = %q", status)
	}
}

func createBoundPaymentOrder(t *testing.T, repo *paymentpostgres.Repository) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration := publishTestStripeConfiguration(t, repo, "acct_payment", "prod_idea")
	input := paymentsdomain.CreatePaymentOrderIntentInput{
		ID: "0198cd29-f457-7e4b-b95e-6cfde41b8111", PublicOrderNumber: "IBP-20260801-ABC123DEF456",
		PublicToken: "0198cd29-f457-7e4b-b95e-6cfde41b8222", UserID: 42,
		ClientIdempotencyKey: "payment-convergence-checkout", RequestHash: "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID: configuration.CredentialID, StripeAccountID: "acct_payment", StripeProductID: "prod_idea", IdeaQuantity: 501,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "http://localhost:5173",
		ExpiresAt:     time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC),
	}
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	order, err = repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_payment", StripeSessionID: "cs_test_payment",
		CheckoutURL: "https://checkout.stripe.com/c/pay/cs_test_payment", ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func paidCheckoutFacts(order paymentsdomain.PaymentOrder) paymentsdomain.PaidCheckoutFacts {
	return paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: order.StripeAccountID, StripeSessionID: order.StripeCheckoutSessionID,
		ExpiresAt:         order.ExpiresAt,
		PublicOrderNumber: order.PublicOrderNumber, PaymentOrderID: order.ID, RequestHash: order.RequestHash,
		StripeCustomerID: order.StripeCustomerID, StripePaymentIntentID: "pi_payment", StripeChargeID: "ch_payment",
		IdeaQuantity: order.IdeaQuantity, PaidSubtotalMinor: 501, PaidTaxMinor: 1, PaidTotalMinor: 502,
		PaidCurrency: "usd", ReceiptURL: "https://pay.stripe.com/receipts/payment", PaidAt: time.Date(2026, 8, 1, 12, 5, 0, 0, time.UTC),
	}
}
