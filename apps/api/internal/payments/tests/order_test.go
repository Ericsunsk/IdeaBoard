package tests

import (
	"errors"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"strings"
	"testing"
	"time"

	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/testdb"
)

func TestCreatePaymentOrderIntentIsDurableIdempotentAndFailClosed(t *testing.T) {
	db := testdb.Open(t)
	repo := paymentpostgres.NewRepository(db)
	configuration := publishTestStripeConfiguration(t, repo, "acct_order", "prod_idea")
	input := paymentsdomain.CreatePaymentOrderIntentInput{
		ID:                   "0198cd29-f457-7e4b-b95e-6cfde41b7111",
		PublicOrderNumber:    "IBP-20260801-ABC123DEF456",
		PublicToken:          "0198cd29-f457-7e4b-b95e-6cfde41b7222",
		UserID:               42,
		ClientIdempotencyKey: "checkout-request-1",
		RequestHash:          "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID:   configuration.CredentialID,
		StripeAccountID:      "acct_order",
		StripeProductID:      "prod_idea",
		IdeaQuantity:         501,
		TermsVersion:         paymentsdomain.CurrentTermsVersion,
		RefundPolicyVersion:  paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL:        "http://localhost:5173",
		ExpiresAt:            time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC),
	}

	first, reused, err := repo.CreatePaymentOrderIntent(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if reused || first.Status != paymentsdomain.PaymentOrderCreating || first.PublicToken != input.PublicToken {
		t.Fatalf("first order=%+v reused=%t", first, reused)
	}

	replayed, reused, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID:                "0198cd29-f457-7e4b-b95e-6cfde41b7333",
		PublicOrderNumber: "IBP-20260801-XYZ123DEF456",
		PublicToken:       "0198cd29-f457-7e4b-b95e-6cfde41b7444",
		UserID:            input.UserID, ClientIdempotencyKey: input.ClientIdempotencyKey,
		RequestHash: input.RequestHash, StripeCredentialID: input.StripeCredentialID, StripeAccountID: input.StripeAccountID,
		StripeProductID: input.StripeProductID, IdeaQuantity: input.IdeaQuantity,
		TermsVersion:        input.TermsVersion,
		RefundPolicyVersion: input.RefundPolicyVersion, ReturnBaseURL: input.ReturnBaseURL, ExpiresAt: input.ExpiresAt,
	})
	if err != nil || !reused || replayed.ID != first.ID || replayed.PublicToken != first.PublicToken {
		t.Fatalf("replayed order=%+v reused=%t error=%v", replayed, reused, err)
	}

	conflict := input
	conflict.RequestHash = "bdb12e222d029f0ef71c1f62dc883f7c9111486712cd2d862397bc562e36fc99"
	if _, _, err := repo.CreatePaymentOrderIntent(t.Context(), conflict); !errors.Is(err, paymentsdomain.ErrCheckoutIdempotencyConflict) {
		t.Fatalf("conflict error=%v, want paymentsdomain.ErrCheckoutIdempotencyConflict", err)
	}

	var count int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE user_id = 42`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("payment order count=%d, want 1", count)
	}
}

func TestCreatePaymentOrderIntentRejectsInvalidStripeCredentialID(t *testing.T) {
	repo := paymentpostgres.NewRepository(testdb.Open(t))
	_, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: "0198cd29-f457-7e4b-b95e-6cfde41b7111", PublicToken: "0198cd29-f457-7e4b-b95e-6cfde41b7222",
	})
	if err == nil || !strings.Contains(err.Error(), "parse Stripe credential ID") {
		t.Fatalf("CreatePaymentOrderIntent() error = %v, want invalid Stripe credential ID", err)
	}
}
