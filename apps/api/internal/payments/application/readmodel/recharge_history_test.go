package readmodel_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/google/uuid"

	"ideaboard/api/internal/payments/application/readmodel"
)

func TestRechargeHistoryCursorRoundTrip(t *testing.T) {
	occurredAt := time.Date(2026, 8, 4, 12, 0, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	orderID := uuid.MustParse("0198cd29-f457-7e4b-b95e-6cfde41b8333")
	cursor, err := readmodel.EncodeRechargeHistoryCursor(occurredAt, orderID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := readmodel.DecodeRechargeHistoryCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OccurredAt != occurredAt.UTC().Format(time.RFC3339Nano) || decoded.OrderID != orderID.String() {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
}

func TestAdminReadModelHelpers(t *testing.T) {
	if got := readmodel.MaskPaymentIdentity("cus_123456789"); got != "cus_…6789" {
		t.Fatalf("MaskPaymentIdentity() = %q", got)
	}
	if got := readmodel.MaskPaymentIdentity("short"); got != "" {
		t.Fatalf("MaskPaymentIdentity(short) = %q", got)
	}
	if got := readmodel.StripeDisputeDashboardURL(paymentports.StripeModeTest, "dp_abc123"); got != "https://dashboard.stripe.com/test/disputes/dp_abc123" {
		t.Fatalf("StripeDisputeDashboardURL(test) = %q", got)
	}
	if got := readmodel.StripeDisputeDashboardURL(paymentports.StripeModeLive, "invalid"); got != "" {
		t.Fatalf("StripeDisputeDashboardURL(invalid) = %q", got)
	}
	if got := readmodel.StripeStatus(paymentsdomain.PaymentOrderCredited); got != "paid" {
		t.Fatalf("StripeStatus() = %q", got)
	}
	if got := readmodel.CreditStatus(paymentsdomain.PaymentOrderPaidPendingCredit); got != "pending" {
		t.Fatalf("CreditStatus() = %q", got)
	}
	paidTotal := int64(1000)
	if got := readmodel.DisputeRatioBasisPoints(125, &paidTotal); got != 1250 {
		t.Fatalf("DisputeRatioBasisPoints() = %d", got)
	}
}

func TestDecodeRechargeHistoryCursorRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"bad", strings.Repeat("a", readmodel.MaxRechargeHistoryCursorLen+1)} {
		if _, err := readmodel.DecodeRechargeHistoryCursor(value); !errors.Is(err, paymentcontracts.ErrRechargeHistoryCursor) {
			t.Fatalf("DecodeRechargeHistoryCursor(%q) error = %v", value, err)
		}
	}
}
