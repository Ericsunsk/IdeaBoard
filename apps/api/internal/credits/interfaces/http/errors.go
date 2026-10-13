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
	{target: domain.ErrInvalidCommand, status: http.StatusBadRequest, message: msgPackage},
	{target: domain.ErrIdempotencyConflict, status: http.StatusConflict, message: msgPackage2},
	{target: domain.ErrWalletNotFound, status: http.StatusNotFound, message: msgPackage3},
	{target: domain.ErrWalletFrozen, status: http.StatusLocked, message: msgPackage4},
	{target: domain.ErrWalletRestricted, status: http.StatusLocked, message: msgPackage5},
	{target: domain.ErrInsufficientCredits, status: http.StatusUnprocessableEntity, message: msgPackage6},
	{target: domain.ErrChargeNotFound, status: http.StatusNotFound, message: msgPackage7},
	{target: domain.ErrRefundExceedsCharge, status: http.StatusUnprocessableEntity, message: msgPackage8},
	{target: domain.ErrReconciliationNotFound, status: http.StatusNotFound, message: msgPackage9},
	{target: domain.ErrReconciliationUnavailable, status: http.StatusServiceUnavailable, message: msgPackage10, internal: true},
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
	response.InternalError(c, http.StatusInternalServerError, msgWriteCreditError, err)
}

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgPackage          = "参数错误"            // Package @ errors.go
	msgPackage2         = "幂等键已用于不同的Idea操作" // Package @ errors.go
	msgPackage3         = "Idea钱包不存在"       // Package @ errors.go
	msgPackage4         = "Idea钱包已冻结"       // Package @ errors.go
	msgPackage5         = "Idea钱包当前受限"      // Package @ errors.go
	msgPackage6         = "Idea不足"          // Package @ errors.go
	msgPackage7         = "原扣款记录不存在"        // Package @ errors.go
	msgPackage8         = "退款金额超过原扣款剩余额度"   // Package @ errors.go
	msgPackage9         = "Idea对账任务不存在"     // Package @ errors.go
	msgPackage10        = "Idea对账服务暂不可用"    // Package @ errors.go
	msgWriteCreditError = "Idea操作失败"        // WriteCreditError @ errors.go
)
