package dbutil

import (
	"context"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
)

func TestWithTxDBTXRejectsNilInputs(t *testing.T) {
	ctx := context.Background()
	noop := func(*dbsqlc.Queries, pgx.Tx) error { return nil }
	if err := WithTxDBTX(ctx, nil, noop); err == nil {
		t.Fatal("WithTxDBTX with nil db succeeded, want error")
	}
	if err := WithTxDBTX(ctx, nil, nil); err == nil {
		t.Fatal("WithTxDBTX with nil fn succeeded, want error")
	}
	if err := WithTx(ctx, nil, func(*dbsqlc.Queries) error { return nil }); err == nil {
		t.Fatal("WithTx with nil db succeeded, want error")
	}
	if err := WithTx(ctx, nil, nil); err == nil {
		t.Fatal("WithTx with nil fn succeeded, want error")
	}
}

func TestRollbackTimeoutIsFiveSeconds(t *testing.T) {
	if RollbackTimeout != 5*time.Second {
		t.Fatalf("RollbackTimeout = %v, want 5s (shared pgx rollback budget)", RollbackTimeout)
	}
}

func TestNullHelpersRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	if got := TimePtr(NullTime(&now)); got == nil || !got.Equal(now) {
		t.Fatalf("NullTime/TimePtr round trip = %v, want %v", got, now)
	}
	if TimePtr(NullTime(nil)) != nil {
		t.Fatal("NullTime(nil) should be invalid")
	}
	var i int64 = 7
	if got := Int64Ptr(NullInt64(&i)); got == nil || *got != i {
		t.Fatalf("NullInt64/Int64Ptr round trip = %v, want %v", got, i)
	}
	s := "  hello  "
	if got := StringPtr(NullString(&s)); got == nil || *got != "hello" {
		t.Fatalf("NullString/StringPtr round trip = %v, want hello", got)
	}
	if StringPtr(NullString(nil)) != nil {
		t.Fatal("NullString(nil) should be invalid")
	}
}
