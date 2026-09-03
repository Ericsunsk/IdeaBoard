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
