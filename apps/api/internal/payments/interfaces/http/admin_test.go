package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/config"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type stubAdminService struct {
	createRefund func(context.Context, paymentcontracts.CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error)
}

func (s *stubAdminService) GetStripeConfiguration(context.Context) (paymentcontracts.StripeConfigurationSummary, error) {
	return paymentcontracts.StripeConfigurationSummary{}, errors.New("not implemented")
}

func (s *stubAdminService) ConfigureStripe(context.Context, paymentcontracts.ConfigureStripeCommand) (paymentcontracts.StripeConfigurationSummary, error) {
	return paymentcontracts.StripeConfigurationSummary{}, errors.New("not implemented")
}

func (s *stubAdminService) ListAdminPaymentOrders(context.Context, paymentcontracts.AdminPaymentOrderListInput) (paymentcontracts.AdminPaymentOrderPage, error) {
	return paymentcontracts.AdminPaymentOrderPage{}, errors.New("not implemented")
}

func (s *stubAdminService) GetAdminPaymentOrder(context.Context, string) (paymentcontracts.AdminPaymentOrderDetail, error) {
	return paymentcontracts.AdminPaymentOrderDetail{}, errors.New("not implemented")
}

func (s *stubAdminService) CreatePaymentRefund(ctx context.Context, command paymentcontracts.CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error) {
	return s.createRefund(ctx, command)
}

func serveAdminRefund(t *testing.T, service paymentcontracts.AdminService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "order_number", Value: "PO-1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/payment-orders/PO-1/refunds", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	identity, err := principal.New(1, uuid.New(), "auth-admin", "Test Admin", string(principal.RoleAdmin))
	if err != nil {
		t.Fatal(err)
	}
	if err := principal.Set(c, identity); err != nil {
		t.Fatal(err)
	}
	handler := NewAdminHandler(service, config.PublicOriginsConfig{})
	handler.CreateAdminPaymentRefund(c)
	return recorder
}

func TestCreateAdminPaymentRefundRendersFixedSafeMessage(t *testing.T) {
	const secret = "ch_secret_must_not_escape"
	tests := []struct {
		name   string
		err    error
		status int
		msg    string
	}{
		{
			name:   "invalid amount joined with provider detail",
			err:    errors.Join(paymentcontracts.ErrRefundAmountInvalid, errors.New("stripe charge "+secret)),
			status: http.StatusBadRequest,
			msg:    "退款 Idea 数量无效",
		},
		{
			name:   "amount exceeds joined with provider detail",
			err:    errors.Join(paymentcontracts.ErrRefundAmountExceeds, errors.New("stripe charge "+secret)),
			status: http.StatusConflict,
			msg:    "退款 Idea 数量超过可退余额",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &stubAdminService{
				createRefund: func(context.Context, paymentcontracts.CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error) {
					return paymentsdomain.PaymentRefund{}, test.err
				},
			}
			recorder := serveAdminRefund(t, service, `{"idea_quantity":100,"idempotency_key":"12345678","reason":"test"}`)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d body = %s", recorder.Code, test.status, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"`+test.msg+`"`) {
				t.Fatalf("body = %s, want fixed message %q", recorder.Body.String(), test.msg)
			}
			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf("response leaked wrapped detail: %s", recorder.Body.String())
			}
		})
	}
}
