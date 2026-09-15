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
		msg    string
	}{
		{name: "invalid command", err: domain.ErrInvalidCommand, status: http.StatusBadRequest, msg: "参数错误"},
		{name: "idempotency conflict", err: domain.ErrIdempotencyConflict, status: http.StatusConflict, msg: "幂等键已用于不同的Idea操作"},
		{name: "wallet not found", err: domain.ErrWalletNotFound, status: http.StatusNotFound, msg: "Idea钱包不存在"},
		{name: "wallet frozen", err: domain.ErrWalletFrozen, status: http.StatusLocked, msg: "Idea钱包已冻结"},
		{name: "wallet restricted", err: domain.ErrWalletRestricted, status: http.StatusLocked, msg: "Idea钱包当前受限"},
		{name: "insufficient credits", err: domain.ErrInsufficientCredits, status: http.StatusUnprocessableEntity, msg: "Idea不足"},
		{name: "charge not found", err: domain.ErrChargeNotFound, status: http.StatusNotFound, msg: "原扣款记录不存在"},
		{name: "refund exceeds charge", err: domain.ErrRefundExceedsCharge, status: http.StatusUnprocessableEntity, msg: "退款金额超过原扣款剩余额度"},
		{name: "reconciliation not found", err: domain.ErrReconciliationNotFound, status: http.StatusNotFound, msg: "Idea对账任务不存在"},
		{name: "reconciliation unavailable", err: domain.ErrReconciliationUnavailable, status: http.StatusServiceUnavailable, msg: "Idea对账服务暂不可用"},
		{name: "unknown", err: errors.New("boom"), status: http.StatusInternalServerError, msg: "Idea操作失败"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/err", func(c *gin.Context) { writeCreditError(c, test.err) })
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
