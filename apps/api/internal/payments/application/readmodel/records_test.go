package readmodel

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/testsupport"
)

func int64Ptr(v int64) *int64 { return &v }

func timePtr(v time.Time) *time.Time { return &v }

func TestRechargeHistoryRecordDerivationKeepsPaidCancelledRefunded(t *testing.T) {
	paid := int64(502)
	occurred := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	orderID := uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333")

	credited := RechargeHistoryRecord{
		OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 501,
		Status:         string(paymentsdomain.PaymentOrderCredited),
		PaidTotalMinor: int64Ptr(paid), PaidCurrency: "USD",
		ReceiptURL: "https://example.com/receipt", RefundedIdeaQuantity: 0, RefundedAmountMinor: 0,
	}
	item, err := rechargeHistoryItem(credited)
	if err != nil {
		t.Fatalf("rechargeHistoryItem(credited) error = %v", err)
	}
	if item.Status != "credited" || item.PaidAmountMinor != paid || item.PaidCurrency != "usd" {
		t.Fatalf("credited item = %+v", item)
	}

	full := credited
	full.RefundedIdeaQuantity = 501
	full.RefundedAmountMinor = 502
	item, err = rechargeHistoryItem(full)
	if err != nil {
		t.Fatalf("rechargeHistoryItem(refunded) error = %v", err)
	}
	if item.Status != "refunded" {
		t.Fatalf("refunded status = %q, want refunded", item.Status)
	}

	cancelled := RechargeHistoryRecord{
		OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1000,
		Status: string(paymentsdomain.PaymentOrderCancelled),
	}
	item, err = rechargeHistoryItem(cancelled)
	if err != nil {
		t.Fatalf("rechargeHistoryItem(cancelled) error = %v", err)
	}
	if item.Status != "cancelled" || item.PaidAmountMinor != 0 || item.PaidCurrency != "" || item.ReceiptURL != "" {
		t.Fatalf("cancelled item = %+v", item)
	}
}

func TestRechargeHistoryRecordRejectsNullableViolations(t *testing.T) {
	occurred := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	orderID := uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333")
	cases := []RechargeHistoryRecord{
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: string(paymentsdomain.PaymentOrderCredited)},
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: string(paymentsdomain.PaymentOrderCredited), PaidTotalMinor: int64Ptr(10)},
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: string(paymentsdomain.PaymentOrderCancelled), PaidTotalMinor: int64Ptr(10)},
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: string(paymentsdomain.PaymentOrderCancelled), PaidCurrency: "usd"},
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: string(paymentsdomain.PaymentOrderCancelled), ReceiptURL: "https://example.com/r"},
		{OrderID: orderID, OccurredAt: occurred, IdeaQuantity: 1, Status: "checkout_open"},
	}
	for i, record := range cases {
		if _, err := rechargeHistoryItem(record); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}

func TestAdminSummaryFromRecordKeepsDerivedStatus(t *testing.T) {
	created := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	paid := int64(1000)
	record := AdminPaymentOrderRecord{
		OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333"), CreatedAt: created,
		PublicOrderNumber: "IBP-20260801-ABC123DEF456",
		UserPublicID:      uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8444"),
		DisplayName:       "Ada", Status: string(paymentsdomain.PaymentOrderCredited),
		IdeaQuantity: 500, BaseCurrency: "USD", BaseAmountMinor: 500,
		PaidTotalMinor: &paid, PaidCurrency: "USD",
		PaidAt: timePtr(created), CreditedAt: timePtr(created),
	}
	summary := AdminPaymentOrderSummaryFromRecord(record)
	if summary.Status != paymentsdomain.PaymentOrderCredited {
		t.Fatalf("status = %q", summary.Status)
	}
	if summary.StripeStatus != "paid" || summary.CreditStatus != "credited" {
		t.Fatalf("derived = (%q, %q)", summary.StripeStatus, summary.CreditStatus)
	}
	if summary.BaseCurrency != "usd" || summary.PaidCurrency != "usd" {
		t.Fatalf("currency = (%q, %q)", summary.BaseCurrency, summary.PaidCurrency)
	}
	if summary.PaidAmountMinor == nil || *summary.PaidAmountMinor != paid {
		t.Fatalf("paid = %+v", summary.PaidAmountMinor)
	}
}

func TestAdminDetailFromFactsRedactsAndDerives(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	facts := AdminPaymentOrderDetailFacts{
		PublicOrderNumber: "IBP-20260801-ABC123DEF456", InternalUserID: 42,
		UserPublicID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8444"), DisplayName: "Ada",
		Status: string(paymentsdomain.PaymentOrderCredited), IdeaQuantity: 500,
		BaseCurrency: "USD", BaseAmountMinor: 500,
		PaidSubtotalMinor: int64Ptr(450), PaidTaxMinor: int64Ptr(50), PaidTotalMinor: int64Ptr(500),
		PaidCurrency:            "USD",
		StripeAccountID:         "acct_123456789",
		StripeCustomerID:        "cus_123456789",
		StripeCheckoutSessionID: "cs_test_123456789",
		StripePaymentIntentID:   "pi_123456789",
		StripeChargeID:          "ch_123456789",
		TermsVersion:            "v1", RefundPolicyVersion: "v1",
		ExpiresAt: now, CreatedAt: now, UpdatedAt: now,
		PaidAt: timePtr(now), CreditedAt: timePtr(now),
		LastErrorCode: "timeout", LastErrorSummary: "transient",
		AnomalousAt: timePtr(now),
	}
	detail := AdminPaymentOrderDetailFromFacts(facts)
	if detail.InternalUserID != 42 {
		t.Fatalf("internal user = %d", detail.InternalUserID)
	}
	if detail.StripeStatus != "paid" || detail.CreditStatus != "credited" {
		t.Fatalf("derived = (%q, %q)", detail.StripeStatus, detail.CreditStatus)
	}
	if detail.Stripe.AccountID != "acct…6789" || detail.Stripe.CustomerID != "cus_…6789" {
		t.Fatalf("redacted stripe = %+v", detail.Stripe)
	}
	if !detail.Reconciliation.Anomalous || detail.Reconciliation.AnomalousAt == nil {
		t.Fatalf("reconciliation = %+v", detail.Reconciliation)
	}
	if detail.Refunds == nil || detail.Disputes == nil || len(detail.Refunds) != 0 || len(detail.Disputes) != 0 {
		t.Fatalf("refunds/disputes must start as empty slices, got %+v / %+v", detail.Refunds, detail.Disputes)
	}
}

func TestAdminWebhookFactsKeepAttributionRedactionAndTimes(t *testing.T) {
	detail := AdminPaymentOrderDetailRecord{}
	received := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	processed := received.Add(time.Minute)
	events := []AdminPaymentWebhookEventRecord{
		{EventType: "refund.updated", StripeObjectID: "re_durable123456789", ReceivedAt: received, ProcessedAt: &processed},
		{EventType: "refund.updated", StripeObjectID: "re_new1234567890", ReceivedAt: received},
		{EventType: "charge.dispute.created", StripeObjectID: "dp_new1234567890", ReceivedAt: received, ProcessedAt: &processed},
		{EventType: "charge.succeeded", StripeObjectID: "ch_ignored", ReceivedAt: received},
	}
	refundIDs := map[string]struct{}{"re_durable123456789": {}}
	disputeIDs := map[string]struct{}{}
	appendAdminPaymentOrderWebhookFacts(&detail, events, refundIDs, disputeIDs)
	if len(detail.Refunds) != 1 {
		t.Fatalf("refunds = %+v, want 1 transient", detail.Refunds)
	}
	if detail.Refunds[0].ID != MaskPaymentIdentity("re_new1234567890") || detail.Refunds[0].Status != "refund.updated" {
		t.Fatalf("refund webhook fact = %+v", detail.Refunds[0])
	}
	if detail.Refunds[0].UpdatedAt == nil || !detail.Refunds[0].UpdatedAt.Equal(received) {
		t.Fatalf("refund fallback time = %+v, want received", detail.Refunds[0].UpdatedAt)
	}
	if len(detail.Disputes) != 1 {
		t.Fatalf("disputes = %+v, want 1 transient", detail.Disputes)
	}
	if detail.Disputes[0].UpdatedAt == nil || !detail.Disputes[0].UpdatedAt.Equal(processed) {
		t.Fatalf("dispute processed time = %+v, want processed", detail.Disputes[0].UpdatedAt)
	}
}

type stubReadmodelCheckoutStore struct {
	history      []RechargeHistoryRecord
	admin        []AdminPaymentOrderRecord
	total        int64
	cutoff       time.Time
	historyLimit int
	adminFilters AdminPaymentOrderFilters
	detail       AdminPaymentOrderDetailFacts
	events       []AdminPaymentWebhookEventRecord
}

func (s *stubReadmodelCheckoutStore) ListPaymentRechargeHistory(_ context.Context, _ int64, cutoff time.Time, _ *RechargeHistoryCursor, limit int) ([]RechargeHistoryRecord, error) {
	s.cutoff = cutoff
	s.historyLimit = limit
	return s.history, nil
}

func (s *stubReadmodelCheckoutStore) ListAdminPaymentOrders(_ context.Context, filters AdminPaymentOrderFilters) ([]AdminPaymentOrderRecord, int64, error) {
	s.adminFilters = filters
	return s.admin, s.total, nil
}

func (s *stubReadmodelCheckoutStore) GetAdminPaymentOrder(_ context.Context, _ string) (AdminPaymentOrderDetailFacts, []AdminPaymentWebhookEventRecord, error) {
	return s.detail, s.events, nil
}

// Minimal fakes reusing existing domain seams.

type stubConfigurationStore struct{}

func (stubConfigurationStore) GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error) {
	return paymentconfiguration.StoredStripeConfiguration{}, nil
}

type stubConvergenceStore struct{}

func (stubConvergenceStore) ListPaymentRefundsForOrder(context.Context, string) ([]paymentsdomain.PaymentRefund, error) {
	return nil, nil
}

func (stubConvergenceStore) ListPaymentDisputesForOrder(context.Context, string) ([]paymentsdomain.PaymentDispute, error) {
	return nil, nil
}

func TestListRechargesKeepsCutoffLimitPlusOneCursorAndSort(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	firstAt := now.Add(-time.Hour)
	secondAt := now.Add(-2 * time.Hour)
	thirdAt := now.Add(-3 * time.Hour)
	paid := int64(500)
	store := &stubReadmodelCheckoutStore{history: []RechargeHistoryRecord{
		{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8331"), OccurredAt: firstAt, IdeaQuantity: 100, Status: "cancelled"},
		{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8332"), OccurredAt: secondAt, IdeaQuantity: 500, Status: "credited", PaidTotalMinor: &paid, PaidCurrency: "usd", ReceiptURL: "https://example.com/r"},
		{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333"), OccurredAt: thirdAt, IdeaQuantity: 500, Status: "credited", PaidTotalMinor: &paid, PaidCurrency: "usd", ReceiptURL: "https://example.com/r"},
	}}
	svc, err := NewService(stubConfigurationStore{}, store, stubConvergenceStore{}, testsupport.FakeMissingWalletReader{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListRecharges(context.Background(), 42, "", 2)
	if err != nil {
		t.Fatalf("ListRecharges() error = %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	if store.historyLimit != 3 {
		t.Fatalf("store limit = %d, want limit+1=3", store.historyLimit)
	}
	wantCutoff := now.UTC().AddDate(0, -12, 0)
	if !store.cutoff.Equal(wantCutoff) {
		t.Fatalf("cutoff = %v, want %v", store.cutoff, wantCutoff)
	}
	cursor, err := DecodeRechargeHistoryCursor(page.NextCursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	if cursor.OccurredAt != secondAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("cursor occurred = %q, want second row", cursor.OccurredAt)
	}
}

func TestListAdminPaymentOrdersKeepsLimitPlusOneCursorAndSort(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	store := &stubReadmodelCheckoutStore{
		admin: []AdminPaymentOrderRecord{
			{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8331"), CreatedAt: now, PublicOrderNumber: "IBP-20260801-AAA111AAA111", Status: "credited", IdeaQuantity: 1, BaseCurrency: "usd", BaseAmountMinor: 1},
			{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8332"), CreatedAt: now.Add(-time.Hour), PublicOrderNumber: "IBP-20260801-BBB222BBB222", Status: "cancelled", IdeaQuantity: 2, BaseCurrency: "usd", BaseAmountMinor: 2},
			{OrderID: uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333"), CreatedAt: now.Add(-2 * time.Hour), PublicOrderNumber: "IBP-20260801-CCC333CCC333", Status: "credited", IdeaQuantity: 3, BaseCurrency: "usd", BaseAmountMinor: 3},
		},
		total: 3,
	}
	svc, err := NewService(stubConfigurationStore{}, store, stubConvergenceStore{}, testsupport.FakeMissingWalletReader{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListAdminPaymentOrders(context.Background(), paymentcontracts.AdminPaymentOrderListInput{Limit: 2})
	if err != nil {
		t.Fatalf("ListAdminPaymentOrders() error = %v", err)
	}
	if len(page.Orders) != 2 || page.Total != 3 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	if page.Orders[0].PublicOrderNumber != "IBP-20260801-AAA111AAA111" {
		t.Fatalf("sort not preserved: %+v", page.Orders)
	}
	cursor, err := DecodeAdminPaymentOrderCursor(page.NextCursor)
	if err != nil {
		t.Fatalf("decode admin cursor: %v", err)
	}
	if cursor.OrderID != "0198cd29-f457-7e4b-b95e-6cfde41b8332" {
		t.Fatalf("cursor order = %q", cursor.OrderID)
	}
}
