package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"

	payments "ideaboard/api/internal/payments"
	paymentports "ideaboard/api/internal/payments/ports"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

func adminPaymentUserPublicID(t *testing.T, db *pgxpool.Pool, userID int64) uuid.UUID {
	t.Helper()
	var publicID uuid.UUID
	if err := db.QueryRow(t.Context(), `SELECT public_id FROM users WHERE id = $1`, userID).Scan(&publicID); err != nil {
		t.Fatalf("load payment user public identity: %v", err)
	}
	return publicID
}

func TestAdminPaymentOrderDetailAssociatesRefundAndDisputeWebhookFacts(t *testing.T) {
	db := testdb.Open(t)
	repo := paymentpostgres.NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_admin_read", "prod_admin_read")
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(nil, nil), testStripeGateway{}, testCheckoutGateway{}, testWalletReader{}, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-EVENTS000001", 501)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_events", StripeSessionID: "cs_events",
		CheckoutURL: "https://checkout.stripe.example/events", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("BindPaymentOrderCheckout() error = %v", err)
	}
	paidAt := time.Now().UTC().Truncate(time.Second)
	if _, _, err := repo.RecordPaidPaymentOrder(t.Context(), paymentsdomain.PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_events", StripeChargeID: "ch_events", IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: bound.BaseAmountMinor, PaidTaxMinor: 0, PaidTotalMinor: bound.BaseAmountMinor,
		PaidCurrency: "usd", ReceiptURL: "https://pay.stripe.com/receipts/events", PaidAt: paidAt,
	}); err != nil {
		t.Fatalf("RecordPaidPaymentOrder() error = %v", err)
	}
	processedAt := paidAt.Add(time.Minute)
	for _, event := range []paymentports.StripeWebhookEvent{
		{ID: "evt_refund_events", Type: "refund.updated", ObjectID: "re_events_12345678", PaymentIntentID: "pi_events", Livemode: false, CreatedAt: processedAt},
		{ID: "evt_dispute_events", Type: "charge.dispute.created", ObjectID: "dp_events_12345678", ChargeID: "ch_events", Livemode: false, CreatedAt: processedAt.Add(time.Second)},
	} {
		duplicate, err := repo.InsertStripeWebhookEvent(t.Context(), event, "processed", &processedAt)
		if err != nil || duplicate {
			t.Fatalf("InsertStripeWebhookEvent(%s) duplicate=%v error=%v", event.Type, duplicate, err)
		}
	}

	detail, err := service.GetAdminPaymentOrder(t.Context(), order.PublicOrderNumber)
	if err != nil {
		t.Fatalf("GetAdminPaymentOrder() error = %v", err)
	}
	if len(detail.Refunds) != 1 || detail.Refunds[0].Status != "refund.updated" || detail.Refunds[0].ID != "re_e…5678" {
		t.Fatalf("refunds = %+v", detail.Refunds)
	}
	if len(detail.Disputes) != 1 || detail.Disputes[0].Status != "charge.dispute.created" || detail.Disputes[0].ID != "dp_e…5678" {
		t.Fatalf("disputes = %+v", detail.Disputes)
	}
}

func createAdminPaymentOrder(t *testing.T, repo *paymentpostgres.Repository, userID int64, publicOrderNumber string, quantity int64) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration := activeTestStripeConfiguration(t, repo)
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(), UserID: userID,
		ClientIdempotencyKey: "admin-read-" + strings.ToLower(publicOrderNumber[len(publicOrderNumber)-12:]),
		RequestHash:          "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID:   configuration.CredentialID,
		StripeAccountID:      "acct_admin_read", StripeProductID: "prod_admin_read", IdeaQuantity: quantity,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards", ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePaymentOrderIntent(%s) error = %v", publicOrderNumber, err)
	}
	return order
}

func markAdminPaymentOrderAnomalous(t *testing.T, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, orderID string) {
	t.Helper()
	parsed, err := uuid.Parse(orderID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `
UPDATE payment_order_reconciliations
SET anomalous_at = now(),
    last_checked_at = now(),
    last_error_code = 'configuration_unavailable',
    last_error_summary = '需要人工检查'
WHERE payment_order_id = $1`, parsed); err != nil {
		t.Fatalf("mark payment order anomalous: %v", err)
	}
}
