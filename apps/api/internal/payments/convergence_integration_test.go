package payments

import (
	"errors"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
)

func TestPaidPaymentOrderAndPurchaseCreditConvergeAtomicallyOnce(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedAmount, err := credits.NewCreditAmount(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creditService.Grant(t.Context(), credits.GrantCommand{
		UserID: 42, Amount: seedAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "payment-convergence-seed", Actor: credits.Actor{Type: credits.ActorAdmin, ID: "42"},
			Source: "admin_console", Reason: "seed payment convergence Wallet",
			BusinessReference: credits.BusinessReference{Type: "admin_adjustment", ID: "payment-convergence-seed"},
		},
	}); err != nil {
		t.Fatalf("seed Wallet: %v", err)
	}

	repo := NewRepository(db)
	order := createBoundPaymentOrder(t, repo)
	facts := paidCheckoutFacts(order)
	creditedAt := time.Date(2026, 8, 1, 12, 10, 0, 0, time.UTC)

	const workers = 4
	results := make(chan PaymentConvergenceResult, workers)
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
		if result.Order.Status != PaymentOrderCredited || result.AvailableIdeas != 502 {
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
	if err := db.QueryRowContext(t.Context(), `
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
	seedAmount, _ := credits.NewCreditAmount(1)
	if _, err := creditService.Grant(t.Context(), credits.GrantCommand{
		UserID: 42, Amount: seedAmount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "payment-conflict-seed", Actor: credits.Actor{Type: credits.ActorAdmin, ID: "42"},
			Source: "admin_console", Reason: "seed payment convergence Wallet",
			BusinessReference: credits.BusinessReference{Type: "admin_adjustment", ID: "payment-conflict-seed"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	order := createBoundPaymentOrder(t, repo)
	facts := paidCheckoutFacts(order)
	facts.RequestHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	if _, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, time.Now()); !errors.Is(err, ErrCheckoutIdempotencyConflict) {
		t.Fatalf("ConvergePaidPaymentOrder() error = %v, want idempotency conflict", err)
	}
	var purchaseCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'purchase_credit'`).Scan(&purchaseCount); err != nil {
		t.Fatal(err)
	}
	if purchaseCount != 0 {
		t.Fatalf("purchase ledger transactions = %d, want 0", purchaseCount)
	}
	var status string
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM payment_orders WHERE id = $1`, order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(PaymentOrderCheckoutOpen) {
		t.Fatalf("payment order status = %q", status)
	}
}

func createBoundPaymentOrder(t *testing.T, repo *Repository) PaymentOrder {
	t.Helper()
	input := CreatePaymentOrderIntentInput{
		ID: "0198cd29-f457-7e4b-b95e-6cfde41b8111", PublicOrderNumber: "IBP-20260801-ABC123DEF456",
		PublicToken: "0198cd29-f457-7e4b-b95e-6cfde41b8222", UserID: 42,
		ClientIdempotencyKey: "payment-convergence-checkout", RequestHash: "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeAccountID: "acct_payment", StripeProductID: "prod_idea", IdeaQuantity: 501,
		TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "http://localhost:5173",
		ExpiresAt:     time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC),
	}
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	order, err = repo.BindPaymentOrderCheckout(t.Context(), BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_payment", StripeSessionID: "cs_test_payment",
		CheckoutURL: "https://checkout.stripe.com/c/pay/cs_test_payment", ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func paidCheckoutFacts(order PaymentOrder) PaidCheckoutFacts {
	return PaidCheckoutFacts{
		StripeAccountID: order.StripeAccountID, StripeSessionID: order.StripeCheckoutSessionID,
		ExpiresAt:         order.ExpiresAt,
		PublicOrderNumber: order.PublicOrderNumber, PaymentOrderID: order.ID, RequestHash: order.RequestHash,
		StripeCustomerID: order.StripeCustomerID, StripePaymentIntentID: "pi_payment", StripeChargeID: "ch_payment",
		IdeaQuantity: order.IdeaQuantity, PaidSubtotalMinor: 501, PaidTaxMinor: 1, PaidTotalMinor: 502,
		PaidCurrency: "usd", ReceiptURL: "https://pay.stripe.com/receipts/payment", PaidAt: time.Date(2026, 8, 1, 12, 5, 0, 0, time.UTC),
	}
}
