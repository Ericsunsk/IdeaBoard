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

func TestPaymentErrorRenderNeverLeaksWrappedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "sk_live_must_not_escape"
	tests := []struct {
		name   string
		writer func(*gin.Context, error)
		err    error
		status int
		msg    string
	}{
		{
			name:   "checkout invalid input joined with provider detail",
			writer: writeCheckoutError,
			err: errors.Join(
				paymentsdomain.ErrInvalidIdeaQuantity,
				errors.New("raw provider body "+secret),
			),
			status: http.StatusBadRequest,
			msg:    "充值参数错误",
		},
		{
			name:   "stripe invalid input joined with provider detail",
			writer: writeStripeConfigurationError,
			err: errors.Join(
				paymentcontracts.ErrInvalidProductTaxCode,
				errors.New("raw provider body "+secret),
			),
			status: http.StatusBadRequest,
			msg:    "Stripe 配置参数错误",
		},
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
			var body struct {
				Msg string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Msg != test.msg {
				t.Fatalf("msg = %q, want %q", body.Msg, test.msg)
			}
			if strings.Contains(recorder.Body.String(), secret) {
				t.Fatalf("response leaked wrapped detail: %s", recorder.Body.String())
			}
		})
	}
}

func TestPaymentErrorRenderFallsBackToFixedSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "sk_live_must_not_escape"
	mapping := paymentErrorMapping{
		targets: []error{errors.New("sentinel")},
		status:  http.StatusBadRequest,
		code:    "test_empty_message",
	}
	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		mapping.render(c, errors.Join(errors.New("sentinel"), errors.New("raw "+secret)))
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "test_empty_message" || body.Msg != "请求处理失败，请稍后重试" {
		t.Fatalf("response = %+v, want fixed fallback", body)
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("response leaked wrapped detail: %s", recorder.Body.String())
	}
}

func TestPaymentErrorTablesRenderLockedMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		writer func(*gin.Context, error)
		err    error
		status int
		code   string
		msg    string
	}{
		{name: "stripe bad input", writer: writeStripeConfigurationError, err: paymentcontracts.ErrInvalidProductTaxCode, status: http.StatusBadRequest, msg: "Stripe 配置参数错误"},
		{name: "stripe forbidden", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripePermissionDenied, status: http.StatusForbidden, msg: paymentcontracts.ErrStripePermissionDenied.Error()},
		{name: "stripe unprocessable", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeAccountUnavailable, status: http.StatusUnprocessableEntity, msg: "当前 Stripe 配置不可用"},
		{name: "stripe unavailable", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeProviderUnavailable, status: http.StatusServiceUnavailable, msg: paymentcontracts.ErrStripeProviderUnavailable.Error()},
		{name: "stripe changed", writer: writeStripeConfigurationError, err: paymentcontracts.ErrStripeConfigurationChanged, status: http.StatusConflict, msg: "Stripe 配置已被其他操作更新，请重试"},
		{name: "stripe default", writer: writeStripeConfigurationError, err: errors.New("boom"), status: http.StatusInternalServerError, msg: "保存 Stripe 设置失败"},
		{name: "checkout conflict", writer: writeCheckoutError, err: paymentcontracts.ErrPaymentAlreadyInProgress, status: http.StatusConflict, msg: "充值请求冲突，请稍后重试"},
		{name: "checkout locked", writer: writeCheckoutError, err: paymentsdomain.ErrWalletNotRechargeable, status: http.StatusConflict, msg: "当前账户暂不可充值"},
		{name: "checkout purchases unavailable", writer: writeCheckoutError, err: paymentsdomain.ErrPurchasesUnavailable, status: http.StatusServiceUnavailable, msg: "充值服务暂不可用"},
		{name: "checkout provider unavailable", writer: writeCheckoutError, err: paymentcontracts.ErrStripeProviderUnavailable, status: http.StatusServiceUnavailable, msg: "支付服务暂不可用"},
		{name: "checkout default", writer: writeCheckoutError, err: errors.New("boom"), status: http.StatusServiceUnavailable, msg: "充值服务暂不可用"},
		{name: "webhook bad signature", writer: writeStripeWebhookError, err: paymentports.ErrMalformedWebhookPayload, status: http.StatusBadRequest, msg: "Stripe Webhook 签名无效"},
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
			var body struct {
				Msg string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Msg != test.msg {
				t.Fatalf("msg = %q, want %q", body.Msg, test.msg)
			}
		})
	}
}
