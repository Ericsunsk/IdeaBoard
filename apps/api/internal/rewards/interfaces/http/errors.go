package http

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	creditsdomain "ideaboard/api/internal/credits/domain"
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

// Reward scopes partition the single domain table across surfaces:
// admin writes render through writeRewardError, user redeem through
// writeRewardRedeemError. Unscoped rows serve both; scoped rows serve
// one surface (notably ErrInvalidCode: 400 invalid for admin, 422
// unavailable for redeem so callers branch on one code).
const (
	rewardScopeAdmin  = "admin"
	rewardScopeRedeem = "redeem"
)

// rewardDomainErrorTable is the single source of truth for mapping domain
// failures to the machine-readable envelope, shared by the admin and
// redeem surfaces via Scope. Rows are ordered from the most specific
// outcome to the most generic; the first in-scope match wins.
var rewardDomainErrorTable = []errorrender.Mapping{
	{
		Targets: []error{rewardsdomain.ErrRewardCodeConflict},
		Status:  http.StatusConflict,
		Code:    codeRewardCodeIdempotencyConflict,
		Message: errorrender.MsgRender2,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrCodeCollision},
		Status:  http.StatusConflict,
		Code:    codeRewardCodeConflict,
		Message: errorrender.MsgRender3,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrRewardCodeNotFound},
		Status:  http.StatusNotFound,
		Code:    codeRewardCodeNotFound,
		Message: errorrender.MsgRender4,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrCapBelowRedemptions},
		Status:  http.StatusConflict,
		Code:    codeRewardCodeCapInvalid,
		Message: errorrender.MsgRender5,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrRewardCodeRedeemed},
		Status:  http.StatusConflict,
		Code:    codeRewardCodeRedeemed,
		Message: errorrender.MsgRender6,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrRedemptionNotFound},
		Status:  http.StatusNotFound,
		Code:    codeRewardRedemptionNotFound,
		Message: errorrender.MsgRender7,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrCodeUnavailable},
		Status:  http.StatusUnprocessableEntity,
		Code:    codeRewardCodeUnavailable,
		Message: errorrender.MsgRender8,
	},
	{
		// Wallet lock converges both sentinels: the adapter maps the
		// canonical credits/domain frozen state to rewardsdomain, but a
		// direct credits failure must render the same 409 instead of a
		// generic 500. credits/domain stays the single source; this row
		// only lists its target.
		Targets: []error{rewardsdomain.ErrWalletFrozen, creditsdomain.ErrWalletFrozen},
		Status:  http.StatusConflict,
		Code:    codeRewardWalletLocked,
		Message: errorrender.MsgRender9,
	},
	{
		Targets: []error{rewardsdomain.ErrInvalidCode},
		Status:  http.StatusBadRequest,
		Code:    codeRewardCodeInvalid,
		Message: errorrender.MsgRewardInvalidInput,
		Scope:   rewardScopeAdmin,
	},
	{
		Targets: []error{rewardsdomain.ErrInvalidCode},
		Status:  http.StatusUnprocessableEntity,
		Code:    codeRewardCodeUnavailable,
		Message: errorrender.MsgRender8,
		Scope:   rewardScopeRedeem,
	},
}

func rewardUnmatchedAttrs(err error) []any {
	return []any{
		"outcome", "internal_error",
		"error_type", fmt.Sprintf("%T", err),
		"error_summary", redactedRewardErrorSummary(err),
	}
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
	errorrender.RenderMapped(c, err, rewardDomainErrorTable, errorrender.MappedOptions{
		Scope:           rewardScopeAdmin,
		DefaultStatus:   http.StatusInternalServerError,
		DefaultCode:     codeRewardOperationFailed,
		DefaultMessage:  errorrender.MsgRewardOperationFailed,
		FallbackMessage: errorrender.MsgRewardsInterfacesHTTPRender,
		UnmatchedLog:    "reward operation failed",
		UnmatchedAttrs:  rewardUnmatchedAttrs,
		// Raw errors carry code plaintext/digests; logs keep only the
		// redacted summary, never the wrapped error.
		OmitUnmatchedErr: true,
	})
}

// writeRewardRedeemError renders user redeem failures through the shared
// table (redeem scope). Invalid and unavailable codes share the 422
// outcome so callers branch on one code.
func writeRewardRedeemError(c *gin.Context, err error) {
	errorrender.RenderMapped(c, err, rewardDomainErrorTable, errorrender.MappedOptions{
		Scope:            rewardScopeRedeem,
		DefaultStatus:    http.StatusInternalServerError,
		DefaultCode:      codeRewardRedeemFailed,
		DefaultMessage:   errorrender.MsgRewardRedeemFailed,
		FallbackMessage:  errorrender.MsgRewardsInterfacesHTTPRender,
		UnmatchedLog:     "reward operation failed",
		UnmatchedAttrs:   rewardUnmatchedAttrs,
		OmitUnmatchedErr: true,
	})
}

// writeRewardInputError renders request-shape failures (malformed JSON,
// unknown fields, illegal IDs, illegal pagination) through the machine
// envelope so callers never match on copy.
func writeRewardInputError(c *gin.Context, code, message string) {
	errorrender.Render(c, http.StatusBadRequest, code, message)
}

// writeRewardRateLimitedError renders the per-user redeem throttle outcome.
func writeRewardRateLimitedError(c *gin.Context) {
	errorrender.Render(c, http.StatusTooManyRequests, codeRewardRateLimited, errorrender.MsgRewardRateLimited)
}

// writeRewardUnavailableError renders a missing service dependency.
func writeRewardUnavailableError(c *gin.Context, userFacing bool) {
	if userFacing {
		errorrender.Render(c, http.StatusServiceUnavailable, codeRewardServiceUnavailable, errorrender.MsgRewardRedeemFailed)
		return
	}
	errorrender.Render(c, http.StatusServiceUnavailable, codeRewardServiceUnavailable, errorrender.MsgRewardServiceUnavailable)
}
