package http

import (
	"encoding/json"
	"errors"
	"ideaboard/api/internal/platform/errorrender"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentports "ideaboard/api/internal/payments/ports"

	"github.com/gin-gonic/gin"
)

func TestPaymentErrorRenderFallsBackToFixedSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "sk_live_must_not_escape"
	sentinel := errors.New("sentinel")
	mapping := paymentErrorMapping{
		Targets: []error{sentinel},
		Status:  http.StatusBadRequest,
		Code:    "test_empty_message",
	}
	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		renderMappedPaymentError(c, errors.Join(sentinel, errors.New("raw "+secret)), []paymentErrorMapping{mapping}, http.StatusBadRequest, errorrender.MsgPaymentsInterfacesHTTPRender)
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
		msg    string
	}{
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
