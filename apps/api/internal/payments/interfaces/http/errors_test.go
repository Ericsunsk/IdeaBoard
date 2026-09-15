package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/gin-gonic/gin"
)

func TestWriteCheckoutErrorKeepsProviderDetailsOutOfResponse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
		msg  string
	}{
		{
			name: "provider rejected creation",
			err: errors.Join(
				paymentsdomain.ErrCheckoutRequestRejected,
				errors.New("decline_code=insufficient_funds raw provider body sk_test_secret"),
			),
			code: "checkout_request_rejected",
			msg:  "无法创建支付页面，请重试",
		},
		{name: "session expired", err: paymentcontracts.ErrCheckoutSessionExpired, code: "checkout_session_expired", msg: paymentcontracts.ErrCheckoutSessionExpired.Error()},
		{name: "payment completed", err: paymentcontracts.ErrPaymentAlreadyCompleted, code: "payment_already_completed", msg: paymentcontracts.ErrPaymentAlreadyCompleted.Error()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/checkout-error", func(c *gin.Context) { writeCheckoutError(c, test.err) })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/checkout-error", nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
			}
			var body struct {
				Code        string `json:"code"`
				Msg         string `json:"msg"`
				MessageSafe bool   `json:"message_safe"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code || body.Msg != test.msg || !body.MessageSafe {
				t.Fatalf("response = %+v", body)
			}
			if strings.Contains(recorder.Body.String(), "decline_code") || strings.Contains(recorder.Body.String(), "sk_test_secret") {
				t.Fatalf("response leaked provider details: %s", recorder.Body.String())
			}
		})
	}
}

func TestPaymentErrorTablesRenderLockedMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		writer     func(*gin.Context, error)
		err        error
		status     int
		code       string
		msg        string
		usesErrMsg bool
	}{
		{name: "stripe bad input", writer: writeStripeConfigurationError, err: paymentcontracts.ErrInvalidProductTaxCode, status: http.StatusBadRequest, usesErrMsg: true},
		{name: "stripe forbidden", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripePermissionDenied, status: http.StatusForbidden, msg: paymentcontracts.ErrStripePermissionDenied.Error()},
		{name: "stripe unprocessable", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeAccountUnavailable, status: http.StatusUnprocessableEntity, usesErrMsg: true},
		{name: "stripe unavailable", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeProviderUnavailable, status: http.StatusServiceUnavailable, msg: paymentcontracts.ErrStripeProviderUnavailable.Error()},
		{name: "stripe changed", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeConfigurationChanged, status: http.StatusConflict, msg: "Stripe 配置已被其他操作更新，请重试"},
		{name: "stripe default", writer: writeStripeConfigurationError, err: errors.New("boom"), status: http.StatusInternalServerError, msg: "保存 Stripe 设置失败"},
		{name: "checkout bad input", writer: writeCheckoutError, err: paymentcontracts.ErrPaymentAlreadyInProgress, status: http.StatusConflict, usesErrMsg: true},
		{name: "checkout locked", writer: writeCheckoutError, err: paymentsdomain.ErrWalletNotRechargeable, status: http.StatusLocked, usesErrMsg: true},
		{name: "checkout purchases unavailable", writer: writeCheckoutError, err: paymentsdomain.ErrPurchasesUnavailable, status: http.StatusServiceUnavailable, msg: "充值服务暂不可用"},
		{name: "checkout provider unavailable", writer: writeCheckoutError, err: paymentcontracts.ErrStripeProviderUnavailable, status: http.StatusServiceUnavailable, msg: "支付服务暂不可用"},
		{name: "checkout default", writer: writeCheckoutError, err: errors.New("boom"), status: http.StatusServiceUnavailable, msg: "充值服务暂不可用"},
		{name: "webhook bad signature", writer: writeStripeWebhookError, err: paymentports.ErrMalformedWebhookPayload, status: http.StatusBadRequest, usesErrMsg: true},
		{name: "webhook not configured", writer: writeStripeWebhookError, err: paymentcontracts.ErrStripeConfigurationNotFound, status: http.StatusServiceUnavailable, msg: "Stripe Webhook 尚未配置"},
		{name: "webhook default", writer: writeStripeWebhookError, err: errors.New("boom"), status: http.StatusInternalServerError, msg: "Stripe Webhook 处理失败"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) { test.writer(c, test.err) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			wantMsg := test.msg
			if test.usesErrMsg {
				wantMsg = test.err.Error()
			}
			var body struct {
				Msg string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Msg != wantMsg {
				t.Fatalf("msg = %q, want %q", body.Msg, wantMsg)
			}
		})
	}
}
