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
	row := dbsqlc.ListAccountRechargeHistoryRow{
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

	cancelled := dbsqlc.ListAccountRechargeHistoryRow{
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

func idTimestampUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
