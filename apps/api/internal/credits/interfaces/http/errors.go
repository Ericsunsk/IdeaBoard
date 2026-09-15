package http

import (
	"errors"
	"net/http"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// creditErrorMapping binds one stable outcome to its wire rendering. The
// table is the single source of truth for credits HTTP error rendering;
// add rows here instead of scattering errors.Is checks across handlers.
type creditErrorMapping struct {
	target   error
	status   int
	message  string
	internal bool
}

var creditErrorTable = []creditErrorMapping{
	{target: domain.ErrInvalidCommand, status: http.StatusBadRequest, message: "参数错误"},
	{target: domain.ErrIdempotencyConflict, status: http.StatusConflict, message: "幂等键已用于不同的Idea操作"},
	{target: domain.ErrWalletNotFound, status: http.StatusNotFound, message: "Idea钱包不存在"},
	{target: domain.ErrWalletFrozen, status: http.StatusLocked, message: "Idea钱包已冻结"},
	{target: domain.ErrWalletRestricted, status: http.StatusLocked, message: "Idea钱包当前受限"},
	{target: domain.ErrInsufficientCredits, status: http.StatusUnprocessableEntity, message: "Idea不足"},
	{target: domain.ErrChargeNotFound, status: http.StatusNotFound, message: "原扣款记录不存在"},
	{target: domain.ErrRefundExceedsCharge, status: http.StatusUnprocessableEntity, message: "退款金额超过原扣款剩余额度"},
	{target: domain.ErrReconciliationNotFound, status: http.StatusNotFound, message: "Idea对账任务不存在"},
	{target: domain.ErrReconciliationUnavailable, status: http.StatusServiceUnavailable, message: "Idea对账服务暂不可用", internal: true},
}

func writeCreditError(c *gin.Context, err error) {
	for _, m := range creditErrorTable {
		if errors.Is(err, m.target) {
			if m.internal {
				response.InternalError(c, m.status, m.message, err)
				return
			}
			response.Error(c, m.status, m.message)
			return
		}
	}
	response.InternalError(c, http.StatusInternalServerError, "Idea操作失败", err)
}
