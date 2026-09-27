package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"
	rewardsdomain "ideaboard/api/internal/rewards/domain"

	"github.com/gin-gonic/gin"
)

// Reward error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display and never carry Code plaintext,
// digests, or provider payloads.
//
// Classification lives here at the HTTP edge so domain/ports sentinels stay
// dependency-free; every rewards failure below renders through
// response.MachineError, never through the numeric response.Error envelope.
// Reward error codes are wire-stable. Do not rename a code without
// coordinating the frontend caller that branches on it. Messages stay
// intentionally safe for public display and never carry Code plaintext,
// digests, or provider payloads.
//
// The values below alias the kernel registry
// (platform/contract.CodeReward*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire values are
// unchanged.
const (
	codeRewardInvalidInput            = string(contract.CodeRewardInvalidInput)
	codeRewardCodeIDInvalid           = string(contract.CodeRewardCodeIDInvalid)
	codeRewardRedemptionIDInvalid     = string(contract.CodeRewardRedemptionIDInvalid)
	codeRewardPaginationInvalid       = string(contract.CodeRewardPaginationInvalid)
	codeRewardCodeInvalid             = string(contract.CodeRewardCodeInvalid)
	codeRewardCodeConflict            = string(contract.CodeRewardCodeConflict)
	codeRewardCodeIdempotencyConflict = string(contract.CodeRewardCodeIdempotencyConflict)
	codeRewardCodeNotFound            = string(contract.CodeRewardCodeNotFound)
	codeRewardCodeCapInvalid          = string(contract.CodeRewardCodeCapInvalid)
	codeRewardCodeRedeemed            = string(contract.CodeRewardCodeRedeemed)
	codeRewardRedemptionNotFound      = string(contract.CodeRewardRedemptionNotFound)
	codeRewardCodeUnavailable         = string(contract.CodeRewardCodeUnavailable)
	codeRewardWalletLocked            = string(contract.CodeRewardWalletLocked)
	codeRewardRateLimited             = string(contract.CodeRewardRateLimited)
	codeRewardServiceUnavailable      = string(contract.CodeRewardServiceUnavailable)
	codeRewardOperationFailed         = string(contract.CodeRewardOperationFailed)
	codeRewardRedeemFailed            = string(contract.CodeRewardRedeemFailed)
)

type rewardErrorMapping struct {
	targets []error
	status  int
	code    string
	message string
}

func (m rewardErrorMapping) Matches(err error) bool {
	return errorrender.TargetsMatch(m.targets, err)
}

func (m rewardErrorMapping) render(c *gin.Context, err error) {
	msg := m.message
	if msg == "" {
		slog.Error("reward error mapping missing safe message",
			"code", m.code,
			"status", m.status,
			"error_type", fmt.Sprintf("%T", err),
			"error_summary", redactedRewardErrorSummary(err))
		msg = "操作失败，请稍后重试"
	}
	errorrender.Render(c, m.status, m.code, msg)
}

// rewardDomainErrorTable is the single source of truth for mapping domain
// failures to the machine-readable envelope. Rows are ordered from the most
// specific outcome to the most generic; the first match wins.
var rewardDomainErrorTable = []rewardErrorMapping{
	{
		targets: []error{rewardsdomain.ErrRewardCodeConflict},
		status:  http.StatusConflict,
		code:    codeRewardCodeIdempotencyConflict,
		message: "幂等请求与原创建参数不一致",
	},
	{
		targets: []error{rewardsdomain.ErrCodeCollision},
		status:  http.StatusConflict,
		code:    codeRewardCodeConflict,
		message: "兑换码已存在",
	},
	{
		targets: []error{rewardsdomain.ErrRewardCodeNotFound},
		status:  http.StatusNotFound,
		code:    codeRewardCodeNotFound,
		message: "兑换码不存在",
	},
	{
		targets: []error{rewardsdomain.ErrCapBelowRedemptions},
		status:  http.StatusConflict,
		code:    codeRewardCodeCapInvalid,
		message: "兑换上限不能低于已兑换次数",
	},
	{
		targets: []error{rewardsdomain.ErrRewardCodeRedeemed},
		status:  http.StatusConflict,
		code:    codeRewardCodeRedeemed,
		message: "已有兑换记录，只能停用该兑换码",
	},
	{
		targets: []error{rewardsdomain.ErrRedemptionNotFound},
		status:  http.StatusNotFound,
		code:    codeRewardRedemptionNotFound,
		message: "兑换记录不存在",
	},
	{
		targets: []error{rewardsdomain.ErrCodeUnavailable},
		status:  http.StatusUnprocessableEntity,
		code:    codeRewardCodeUnavailable,
		message: "兑换码无效或已失效",
	},
	{
		targets: []error{rewardsdomain.ErrWalletFrozen},
		status:  http.StatusConflict,
		code:    codeRewardWalletLocked,
		message: "当前账户无法兑换奖励",
	},
	{
		targets: []error{rewardsdomain.ErrInvalidCode},
		status:  http.StatusBadRequest,
		code:    codeRewardCodeInvalid,
		message: "参数错误",
	},
}

func renderMappedRewardError(c *gin.Context, err error, table []rewardErrorMapping, defaultStatus int, defaultCode, defaultMessage string) {
	if row, ok := errorrender.FirstMatch(table, err); ok {
		row.render(c, err)
		return
	}
	slog.Error("reward operation failed",
		"outcome", "internal_error",
		"error_type", fmt.Sprintf("%T", err),
		"error_summary", redactedRewardErrorSummary(err))
	errorrender.Render(c, defaultStatus, defaultCode, defaultMessage)
}

// redactedRewardErrorSummary keeps an unmatched (5xx) failure attributable
// to the original error without leaking sensitive material into logs. Code
// plaintext, sha256 digests, and JSON code fields are replaced, the result
// is folded to a single line and length-bounded. The wire envelope stays
// generic: this summary never leaves the server in a response body.
func redactedRewardErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	summary := err.Error()
	summary = rewardDigestPattern.ReplaceAllString(summary, "[redacted-digest]")
	summary = rewardJSONCodePattern.ReplaceAllString(summary, `"code":"[redacted]"`)
	summary = rewardCodePairPattern.ReplaceAllString(summary, "code=[redacted]")
	summary = rewardDigestPairPattern.ReplaceAllString(summary, "digest=[redacted]")
	summary = strings.Join(strings.Fields(summary), " ")
	if runes := []rune(summary); len(runes) > maxRewardErrorSummaryRunes {
		summary = string(runes[:maxRewardErrorSummaryRunes])
	}
	return summary
}

var (
	rewardDigestPattern     = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)
	rewardJSONCodePattern   = regexp.MustCompile(`"code"\s*:\s*"[^"]*"`)
	rewardCodePairPattern   = regexp.MustCompile(`\bcode\s*=\s*[^\s,;}"']+`)
	rewardDigestPairPattern = regexp.MustCompile(`(?i)\bdigest\s*=\s*[^\s,;}"']+`)
)

const maxRewardErrorSummaryRunes = 300

// writeRewardError renders admin-side failures through the machine envelope.
func writeRewardError(c *gin.Context, err error) {
	renderMappedRewardError(c, err, rewardDomainErrorTable,
		http.StatusInternalServerError, codeRewardOperationFailed, "奖励操作失败")
}

// writeRewardRedeemError renders user redeem failures. Invalid and
// unavailable codes share the 422 outcome so callers branch on one code.
func writeRewardRedeemError(c *gin.Context, err error) {
	renderMappedRewardError(c, err, []rewardErrorMapping{
		{
			targets: []error{rewardsdomain.ErrInvalidCode, rewardsdomain.ErrCodeUnavailable},
			status:  http.StatusUnprocessableEntity,
			code:    codeRewardCodeUnavailable,
			message: "兑换码无效或已失效",
		},
		{
			targets: []error{rewardsdomain.ErrWalletFrozen},
			status:  http.StatusConflict,
			code:    codeRewardWalletLocked,
			message: "当前账户无法兑换奖励",
		},
	}, http.StatusInternalServerError, codeRewardRedeemFailed, "网络错误，请重试")
}

// writeRewardInputError renders request-shape failures (malformed JSON,
// unknown fields, illegal IDs, illegal pagination) through the machine
// envelope so callers never match on copy.
func writeRewardInputError(c *gin.Context, code, message string) {
	errorrender.Render(c, http.StatusBadRequest, code, message)
}

// writeRewardRateLimitedError renders the per-user redeem throttle outcome.
func writeRewardRateLimitedError(c *gin.Context) {
	errorrender.Render(c, http.StatusTooManyRequests, codeRewardRateLimited, "操作频繁，请稍后重试")
}

// writeRewardUnavailableError renders a missing service dependency.
func writeRewardUnavailableError(c *gin.Context, userFacing bool) {
	if userFacing {
		errorrender.Render(c, http.StatusServiceUnavailable, codeRewardServiceUnavailable, "网络错误，请重试")
		return
	}
	errorrender.Render(c, http.StatusServiceUnavailable, codeRewardServiceUnavailable, "奖励服务暂不可用")
}
