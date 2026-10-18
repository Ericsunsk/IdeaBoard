package readmodel_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/pagination"

	"github.com/google/uuid"

	"ideaboard/api/internal/payments/application/readmodel"
)

func TestAdminPaymentOrderPaginationTracksSharedStandard(t *testing.T) {
	if readmodel.DefaultAdminPaymentOrderLimit != pagination.DefaultLimit {
		t.Fatalf("DefaultAdminPaymentOrderLimit = %d, want shared %d", readmodel.DefaultAdminPaymentOrderLimit, pagination.DefaultLimit)
	}
	if readmodel.MaxAdminPaymentOrderLimit != pagination.MaxLimit {
		t.Fatalf("MaxAdminPaymentOrderLimit = %d, want shared %d", readmodel.MaxAdminPaymentOrderLimit, pagination.MaxLimit)
	}
	if readmodel.MaxAdminPaymentOrderCursorLen != pagination.MaxCursorLen {
		t.Fatalf("MaxAdminPaymentOrderCursorLen = %d, want shared %d", readmodel.MaxAdminPaymentOrderCursorLen, pagination.MaxCursorLen)
	}
	filters, err := readmodel.NormalizeAdminPaymentOrderFilters(mustAdminListInput(t, 0, ""))
	if err != nil {
		t.Fatalf("NormalizeAdminPaymentOrderFilters(0) error = %v", err)
	}
	if filters.Limit != pagination.DefaultLimit {
		t.Fatalf("default limit = %d, want shared %d", filters.Limit, pagination.DefaultLimit)
	}
	for _, limit := range []int{1, 20, 200} {
		filters, err := readmodel.NormalizeAdminPaymentOrderFilters(mustAdminListInput(t, limit, ""))
		if err != nil || filters.Limit != limit {
			t.Fatalf("NormalizeAdminPaymentOrderFilters(%d) = %+v, %v", limit, filters, err)
		}
	}
	for _, limit := range []int{-1, 201} {
		if _, err := readmodel.NormalizeAdminPaymentOrderFilters(mustAdminListInput(t, limit, "")); err != paymentcontracts.ErrAdminPaymentOrderFilter {
			t.Fatalf("NormalizeAdminPaymentOrderFilters(%d) error = %v, want ErrAdminPaymentOrderFilter", limit, err)
		}
	}
}

func TestAdminPaymentOrderCursorRoundTripThroughSharedCodec(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 12, 0, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	orderID := uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333")
	raw, err := readmodel.EncodeAdminPaymentOrderCursor(createdAt, orderID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := readmodel.DecodeAdminPaymentOrderCursor(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.CreatedAt != createdAt.UTC().Format(time.RFC3339Nano) || decoded.OrderID != orderID.String() {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	if decoded, err := readmodel.DecodeAdminPaymentOrderCursor(""); err != nil || decoded != nil {
		t.Fatalf("DecodeAdminPaymentOrderCursor(\"\") = %#v, %v, want nil, nil", decoded, err)
	}
}

func TestDecodeAdminPaymentOrderCursorRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"bad", strings.Repeat("a", readmodel.MaxAdminPaymentOrderCursorLen+1)} {
		if _, err := readmodel.DecodeAdminPaymentOrderCursor(value); err != paymentcontracts.ErrAdminPaymentOrderCursor {
			t.Fatalf("DecodeAdminPaymentOrderCursor(%q) error = %v, want ErrAdminPaymentOrderCursor", value, err)
		}
	}
	// Non-canonical timezones and non-lowercase UUIDs must be rejected so
	// cursors stay a canonical keyset position.
	nonCanonicalTime := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-01T20:00:00.123456789+08:00","order_id":"0198cd29-f457-7e4b-b95e-6cfde41b8333"}`))
	if _, err := readmodel.DecodeAdminPaymentOrderCursor(nonCanonicalTime); err != paymentcontracts.ErrAdminPaymentOrderCursor {
		t.Fatalf("DecodeAdminPaymentOrderCursor(non-canonical time) error = %v", err)
	}
	upperID := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-01T12:00:00.123456789Z","order_id":"0198CD29-F457-7E4B-B95E-6CFDE41B8333"}`))
	if _, err := readmodel.DecodeAdminPaymentOrderCursor(upperID); err != nil {
		t.Fatalf("DecodeAdminPaymentOrderCursor(uppercase uuid) error = %v, want nil (legacy case-insensitive contract)", err)
	}
	unknown := base64.RawURLEncoding.EncodeToString([]byte(`{"created_at":"2026-08-01T12:00:00.123456789Z","order_id":"0198cd29-f457-7e4b-b95e-6cfde41b8333","extra":1}`))
	if _, err := readmodel.DecodeAdminPaymentOrderCursor(unknown); err != paymentcontracts.ErrAdminPaymentOrderCursor {
		t.Fatalf("DecodeAdminPaymentOrderCursor(unknown fields) error = %v", err)
	}
}

func mustAdminListInput(t *testing.T, limit int, cursor string) paymentcontracts.AdminPaymentOrderListInput {
	t.Helper()
	return paymentcontracts.AdminPaymentOrderListInput{Limit: limit, Cursor: cursor}
}
