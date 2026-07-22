package rewards

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const maxRewardRequestBytes int64 = 64 << 10

type RewardService interface {
	AdminCommands
	AdminReads
	UserCommands
	UserReads
}

type Handler struct {
	service RewardService
	limiter *redemptionAttemptLimiter
	metrics redemptionMetrics
}

type redemptionHandlerOptions struct {
	Now             func() time.Time
	MetricsRecorder redemptionMetricsRecorder
}

func NewHandler(service RewardService) *Handler {
	return newHandlerWithOptions(service, redemptionHandlerOptions{})
}

func newHandlerWithOptions(service RewardService, options redemptionHandlerOptions) *Handler {
	if options.MetricsRecorder == nil {
		options.MetricsRecorder = defaultRedemptionMetricsRecorder
	}
	return &Handler{
		service: service,
		limiter: newRedemptionAttemptLimiter(options.Now),
		metrics: newRedemptionMetrics(options.Now, options.MetricsRecorder),
	}
}

type createRewardCodeRequest struct {
	DisplayName    string     `json:"display_name"`
	Code           string     `json:"code"`
	CreditAmount   int64      `json:"credit_amount"`
	MaxRedemptions int64      `json:"max_redemptions"`
	ExpiresAt      *time.Time `json:"expires_at"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type updateRewardCodeRequest struct {
	DisplayName    string     `json:"display_name"`
	CreditAmount   int64      `json:"credit_amount"`
	MaxRedemptions int64      `json:"max_redemptions"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Code           *string    `json:"code"`
}

type setRewardCodeEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type searchRewardCodesRequest struct {
	Query    string           `json:"query"`
	Status   RewardCodeStatus `json:"status"`
	BeforeID *int64           `json:"before_id"`
	Limit    *int             `json:"limit"`
}

type redeemRequest struct {
	Code string `json:"code"`
}

func (h *Handler) CreateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request createRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	created, err := h.service.CreateRewardCode(c.Request.Context(), CreateRewardCodeCommand{
		DisplayName: request.DisplayName, Code: request.Code, CreditAmount: request.CreditAmount,
		MaxRedemptions: request.MaxRedemptions, ExpiresAt: request.ExpiresAt,
		IdempotencyKey: request.IdempotencyKey, ActorUserID: actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, created)
}

func (h *Handler) UpdateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rewardCodeID, actorID, ok := h.requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request updateRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	updated, err := h.service.UpdateRewardCode(c.Request.Context(), UpdateRewardCodeCommand{
		RewardCodeID: rewardCodeID, DisplayName: request.DisplayName,
		CreditAmount: request.CreditAmount, MaxRedemptions: request.MaxRedemptions,
		ExpiresAt: request.ExpiresAt, Code: request.Code, ActorUserID: actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, updated)
}

func (h *Handler) SetRewardCodeEnabled(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rewardCodeID, actorID, ok := h.requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request setRewardCodeEnabledRequest
	if err := decodeRewardJSON(c, &request); err != nil || request.Enabled == nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	updated, err := h.service.SetRewardCodeEnabled(c.Request.Context(), SetRewardCodeEnabledCommand{
		RewardCodeID: rewardCodeID, Enabled: *request.Enabled, ActorUserID: actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, updated)
}

func (h *Handler) DeleteRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	rewardCodeID, actorID, ok := h.requireAdminRewardCode(c)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteRewardCode(c.Request.Context(), DeleteRewardCodeCommand{
		RewardCodeID: rewardCodeID, ActorUserID: actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, deleted)
}

func (h *Handler) SearchRewardCodes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	var request searchRewardCodesRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	input := SearchRewardCodesInput{Query: request.Query, Status: request.Status, Limit: 20}
	if request.BeforeID != nil {
		if *request.BeforeID <= 0 {
			response.Error(c, http.StatusBadRequest, "参数错误")
			return
		}
		input.BeforeID = *request.BeforeID
	}
	if request.Limit != nil {
		if *request.Limit < 1 || *request.Limit > 100 {
			response.Error(c, http.StatusBadRequest, "参数错误")
			return
		}
		input.Limit = *request.Limit
	}
	page, err := h.service.SearchRewardCodes(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) Redeem(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "网络错误，请重试")
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	finishMetrics := h.metrics.Start()
	if !h.limiter.Allow(identityPrincipal.UserID()) {
		finishMetrics(redemptionOutcomeRateLimited, 0, nil)
		response.Error(c, http.StatusTooManyRequests, "操作频繁，请稍后重试")
		return
	}
	var request redeemRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		finishMetrics(redemptionOutcomeInvalid, 0, nil)
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	result, err := h.service.Redeem(c.Request.Context(), RedeemCommand{
		UserID: identityPrincipal.UserID(), Code: request.Code,
	})
	if err != nil {
		finishMetrics(redemptionOutcomeForError(err), 0, err)
		writeRewardRedeemError(c, err)
		return
	}
	if result.Replayed {
		finishMetrics(redemptionOutcomeReplayed, 0, nil)
	} else {
		finishMetrics(redemptionOutcomeSuccess, result.CreditedAmount, nil)
	}
	response.SuccessDTO(c, result)
}

func redemptionOutcomeForError(err error) redemptionOutcome {
	switch {
	case errors.Is(err, ErrInvalidCommand), errors.Is(err, ErrCodeUnavailable), errors.Is(err, credits.ErrWalletFrozen):
		return redemptionOutcomeInvalid
	default:
		return redemptionOutcomeInternalError
	}
}

func writeRewardRedeemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCommand), errors.Is(err, ErrCodeUnavailable):
		response.Error(c, http.StatusUnprocessableEntity, "兑换码无效或已失效")
	case errors.Is(err, credits.ErrWalletFrozen):
		response.Error(c, http.StatusLocked, "当前账户无法兑换奖励")
	default:
		slog.Warn("reward redemption failed", "outcome", "internal_error", "error_type", fmt.Sprintf("%T", err))
		response.Error(c, http.StatusInternalServerError, "网络错误，请重试")
	}
}

func (h *Handler) ListRedemptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "奖励服务暂不可用", nil)
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	input := ListRedemptionsInput{UserID: identityPrincipal.UserID(), Limit: defaultRedemptionHistoryLimit}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			response.Error(c, http.StatusBadRequest, "before_id 无效")
			return
		}
		input.BeforeID = value
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxRedemptionHistoryLimit {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return
		}
		input.Limit = value
	}
	page, err := h.service.ListRedemptions(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) requireAdmin(c *gin.Context) (int64, bool) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "奖励服务暂不可用", nil)
		return 0, false
	}
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *Handler) requireAdminRewardCode(c *gin.Context) (int64, int64, bool) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return 0, 0, false
	}
	rewardCodeID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || rewardCodeID <= 0 {
		response.Error(c, http.StatusBadRequest, "兑换码 ID 无效")
		return 0, 0, false
	}
	return rewardCodeID, actorID, true
}

func decodeRewardJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxRewardRequestBytes, destination)
}

func writeRewardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCommand):
		response.Error(c, http.StatusBadRequest, "参数错误")
	case errors.Is(err, ErrRewardCodeConflict):
		response.Error(c, http.StatusConflict, "幂等请求与原创建参数不一致")
	case errors.Is(err, ErrCodeCollision):
		response.Error(c, http.StatusConflict, "兑换码已存在")
	case errors.Is(err, ErrRewardCodeNotFound):
		response.Error(c, http.StatusNotFound, "兑换码不存在")
	case errors.Is(err, ErrCapBelowRedemptions):
		response.Error(c, http.StatusConflict, "兑换上限不能低于已兑换次数")
	case errors.Is(err, ErrRewardCodeRedeemed):
		response.Error(c, http.StatusConflict, "已有兑换记录，只能停用该兑换码")
	case errors.Is(err, ErrCodeUnavailable):
		response.Error(c, http.StatusUnprocessableEntity, "兑换码无效或已失效")
	case errors.Is(err, credits.ErrWalletFrozen):
		response.Error(c, http.StatusLocked, "当前账户无法兑换奖励")
	default:
		slog.Warn("reward admin operation failed", "error_type", fmt.Sprintf("%T", err))
		response.Error(c, http.StatusInternalServerError, "奖励操作失败")
	}
}
