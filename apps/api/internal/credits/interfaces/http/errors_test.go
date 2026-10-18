package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/credits/domain"

	"github.com/gin-gonic/gin"
)

func TestWriteCreditErrorRendersLockedMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		err    error
		status int
		code   string
		msg    string
	}{
		{name: "invalid command", err: domain.ErrInvalidCommand, status: http.StatusBadRequest, code: "credit_invalid_input", msg: "参数错误"},
		{name: "idempotency conflict", err: domain.ErrIdempotencyConflict, status: http.StatusConflict, code: "credit_idempotency_conflict", msg: "幂等键已用于不同的Idea操作"},
		{name: "wallet not found", err: domain.ErrWalletNotFound, status: http.StatusNotFound, code: "credit_wallet_not_found", msg: "Idea钱包不存在"},
		{name: "wallet frozen", err: domain.ErrWalletFrozen, status: http.StatusLocked, code: "credit_wallet_frozen", msg: "Idea钱包已冻结"},
		{name: "wallet restricted", err: domain.ErrWalletRestricted, status: http.StatusLocked, code: "credit_wallet_restricted", msg: "Idea钱包当前受限"},
		{name: "insufficient credits", err: domain.ErrInsufficientCredits, status: http.StatusUnprocessableEntity, code: "credit_insufficient_credits", msg: "Idea不足"},
		{name: "charge not found", err: domain.ErrChargeNotFound, status: http.StatusNotFound, code: "credit_charge_not_found", msg: "原扣款记录不存在"},
		{name: "refund exceeds charge", err: domain.ErrRefundExceedsCharge, status: http.StatusUnprocessableEntity, code: "credit_refund_exceeds_charge", msg: "退款金额超过原扣款剩余额度"},
		{name: "reconciliation not found", err: domain.ErrReconciliationNotFound, status: http.StatusNotFound, code: "credit_reconciliation_not_found", msg: "Idea对账任务不存在"},
		{name: "reconciliation unavailable", err: domain.ErrReconciliationUnavailable, status: http.StatusServiceUnavailable, code: "credit_reconciliation_unavailable", msg: "Idea对账服务暂不可用"},
		{name: "unknown", err: errors.New("boom"), status: http.StatusInternalServerError, code: "credit_operation_failed", msg: "Idea操作失败"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) { writeCreditError(c, test.err) })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			var body struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != test.code {
				t.Fatalf("code = %q, want %q", body.Code, test.code)
			}
			if body.Msg != test.msg {
				t.Fatalf("msg = %q, want %q", body.Msg, test.msg)
			}
		})
	}
}
