package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDecodeRechargeHistoryRecordKeepsNullablePaidFacts(t *testing.T) {
	id := mustUUID(t, "0198cd29-f457-7e4b-b95e-6cfde41b8333")
	occurred := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	row := dbsqlc.ListPaymentRechargeHistoryRow{
		ID: idTimestampUUID(id), OccurredAt: dbutil.TimestamptzFromTime(occurred),
		IdeaQuantity: 501, Status: "credited",
		PaidTotalMinor: pgtype.Int8{Int64: 502, Valid: true}, PaidCurrency: "USD",
		ReceiptUrl: "https://example.com/r", RefundedIdeaQuantity: 0, RefundedAmountMinor: 0,
	}
	record := decodeRechargeHistoryRecord(row)
	if record.OrderID != id || !record.OccurredAt.Equal(occurred) {
		t.Fatalf("keyset = (%v, %v)", record.OrderID, record.OccurredAt)
	}
	if record.PaidTotalMinor == nil || *record.PaidTotalMinor != 502 {
		t.Fatalf("paid = %+v", record.PaidTotalMinor)
	}
	if record.PaidCurrency != "usd" {
		t.Fatalf("currency = %q, want lowercased", record.PaidCurrency)
	}

	cancelled := dbsqlc.ListPaymentRechargeHistoryRow{
		ID: idTimestampUUID(id), OccurredAt: dbutil.TimestamptzFromTime(occurred),
		IdeaQuantity: 100, Status: "cancelled",
		PaidTotalMinor: pgtype.Int8{Valid: false}, PaidCurrency: "",
		ReceiptUrl: "", RefundedIdeaQuantity: 0, RefundedAmountMinor: 0,
	}
	cancelledRecord := decodeRechargeHistoryRecord(cancelled)
	if cancelledRecord.PaidTotalMinor != nil || cancelledRecord.PaidCurrency != "" || cancelledRecord.ReceiptURL != "" {
		t.Fatalf("cancelled paid facts must stay absent: %+v", cancelledRecord)
	}
}

func TestDecodeAdminOrderRecordProjectsOnlyUsedFacts(t *testing.T) {
	id := mustUUID(t, "0198cd29-f457-7e4b-b95e-6cfde41b8333")
	userID := mustUUID(t, "0198cd29-f457-7e4b-b95e-6cfde41b8444")
	created := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	row := dbsqlc.ListAdminPaymentOrdersRow{
		ID: idTimestampUUID(id), PublicOrderNumber: "IBP-20260801-ABC123DEF456",
		UserPublicID: pgtype.UUID{Bytes: userID, Valid: true}, DisplayName: "Ada",
		Status: "credited", IdeaQuantity: 500, BaseCurrency: "USD", BaseAmountMinor: 500,
		PaidTotalMinor: pgtype.Int8{Int64: 500, Valid: true}, PaidCurrency: "USD",
		PaidAt: dbutil.TimestamptzFromTime(created), CreditedAt: dbutil.TimestamptzFromTime(created),
		CreatedAt: dbutil.TimestamptzFromTime(created), UpdatedAt: dbutil.TimestamptzFromTime(created),
		AnomalousAt: pgtype.Timestamptz{Valid: false}, LastCheckedAt: pgtype.Timestamptz{Valid: false},
	}
	record := decodeAdminPaymentOrderRecord(row)
	if record.OrderID != id || !record.CreatedAt.Equal(created) {
		t.Fatalf("keyset = (%v, %v)", record.OrderID, record.CreatedAt)
	}
	if record.BaseCurrency != "usd" || record.PaidCurrency != "usd" {
		t.Fatalf("currency = (%q, %q)", record.BaseCurrency, record.PaidCurrency)
	}
	if record.PaidTotalMinor == nil || *record.PaidTotalMinor != 500 {
		t.Fatalf("paid = %+v", record.PaidTotalMinor)
	}
	if record.AnomalousAt != nil || record.LastCheckedAt != nil {
		t.Fatalf("nullable times must stay nil when invalid: %+v", record)
	}
}

func TestDecodeAdminDetailFactsKeepsRawIdentityAndTimes(t *testing.T) {
	userID := mustUUID(t, "0198cd29-f457-7e4b-b95e-6cfde41b8444")
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	row := dbsqlc.GetAdminPaymentOrderRow{
		PublicOrderNumber: "IBP-20260801-ABC123DEF456", UserID: 42,
		UserPublicID: pgtype.UUID{Bytes: userID, Valid: true}, DisplayName: "Ada",
		Status: "credited", IdeaQuantity: 500, BaseCurrency: "USD", BaseAmountMinor: 500,
		PaidSubtotalMinor: pgtype.Int8{Int64: 450, Valid: true},
		PaidTaxMinor:      pgtype.Int8{Int64: 50, Valid: true},
		PaidTotalMinor:    pgtype.Int8{Int64: 500, Valid: true}, PaidCurrency: "USD",
		StripeAccountID: "acct_123456789", StripeCustomerID: "cus_123456789",
		StripeCheckoutSessionID: pgtype.Text{String: "cs_test_123456789", Valid: true},
		StripePaymentIntentID:   "pi_123456789", StripeChargeID: "ch_123456789",
		TermsVersion: "v1", RefundPolicyVersion: "v1", ReceiptUrl: "https://example.com/r",
		ExpiresAt: dbutil.TimestamptzFromTime(now), CreatedAt: dbutil.TimestamptzFromTime(now), UpdatedAt: dbutil.TimestamptzFromTime(now),
		PaidAt: dbutil.TimestamptzFromTime(now), CreditedAt: dbutil.TimestamptzFromTime(now),
		LastErrorCode:    pgtype.Text{String: "timeout", Valid: true},
		LastErrorSummary: pgtype.Text{String: "transient", Valid: true},
		LastCheckedAt:    pgtype.Timestamptz{Valid: false}, NextAttemptAt: pgtype.Timestamptz{Valid: false},
		AnomalousAt: dbutil.TimestamptzFromTime(now),
	}
	facts := decodeAdminPaymentOrderDetailFacts(row)
	// Adapter must not redact: raw Stripe identity stays for application masking.
	if facts.StripeAccountID != "acct_123456789" || facts.StripeCheckoutSessionID != "cs_test_123456789" {
		t.Fatalf("raw stripe identity = %+v", facts)
	}
	if facts.PaidSubtotalMinor == nil || *facts.PaidSubtotalMinor != 450 {
		t.Fatalf("paid subtotal = %+v", facts.PaidSubtotalMinor)
	}
	if facts.AnomalousAt == nil || !facts.AnomalousAt.Equal(now) {
		t.Fatalf("anomalous = %+v", facts.AnomalousAt)
	}
	if facts.LastCheckedAt != nil || facts.NextAttemptAt != nil {
		t.Fatalf("nullable reconciliation times must stay nil: %+v", facts)
	}
}

func TestDecodeAdminWebhookEventRecordKeepsAttributionAndTimes(t *testing.T) {
	received := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	processed := received.Add(time.Minute)
	row := dbsqlc.ListAdminPaymentOrderWebhookEventsRow{
		EventType: "refund.updated", StripeObjectID: "re_123456789",
		ProcessingResult: "received", ErrorSummary: "ignored",
		StripeCreatedAt: dbutil.TimestamptzFromTime(received),
		ReceivedAt:      dbutil.TimestamptzFromTime(received),
		ProcessedAt:     pgtype.Timestamptz{Time: processed, Valid: true},
	}
	record := decodeAdminPaymentWebhookEventRecord(row)
	if record.EventType != "refund.updated" || record.StripeObjectID != "re_123456789" {
		t.Fatalf("attribution = %+v", record)
	}
	if !record.ReceivedAt.Equal(received) || record.ProcessedAt == nil || !record.ProcessedAt.Equal(processed) {
		t.Fatalf("times = (%v, %+v)", record.ReceivedAt, record.ProcessedAt)
	}

	pending := row
	pending.ProcessedAt = pgtype.Timestamptz{Valid: false}
	pendingRecord := decodeAdminPaymentWebhookEventRecord(pending)
	if pendingRecord.ProcessedAt != nil {
		t.Fatalf("unprocessed event must keep nil processed time: %+v", pendingRecord)
	}
}

func idTimestampUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
