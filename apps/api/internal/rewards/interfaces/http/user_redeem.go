package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/response"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	service ports.UserService
	limiter *redemptionAttemptLimiter
	replay  *idempotency.Store
	metrics redemptionMetrics
}

type RedemptionHandlerOptions struct {
	Now             func() time.Time
	MetricsRecorder redemptionMetricsRecorder
}

func NewUserHandler(service ports.UserService) *UserHandler {
	return NewUserHandlerWithOptions(service, RedemptionHandlerOptions{})
}

func NewUserHandlerWithOptions(service ports.UserService, options RedemptionHandlerOptions) *UserHandler {
	if options.MetricsRecorder == nil {
		options.MetricsRecorder = defaultRedemptionMetricsRecorder
	}
	return &UserHandler{
		service: service,
		limiter: newRedemptionAttemptLimiter(options.Now),
		replay:  idempotency.NewStore(idempotency.DefaultStoreOptions()),
		metrics: newRedemptionMetrics(options.Now, options.MetricsRecorder),
	}
}

var errRewardRedeemRateLimited = errors.New("reward redemption rate limited")

type redeemRequest struct {
	Code string `json:"code"`
}

func (h *UserHandler) Redeem(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, true)
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	finishMetrics := h.metrics.Start()
	key, err := idempotency.Parse(c.GetHeader(idempotency.Header))
	if err != nil {
		finishMetrics(redemptionOutcomeInvalid, 0, err)
		idempotency.DefaultMetrics.ObserveError(err)
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	var request redeemRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		finishMetrics(redemptionOutcomeInvalid, 0, err)
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	requestHash := idempotency.HashRequest(strings.TrimSpace(request.Code))
	data, replayed, err := h.replay.Do(c.Request.Context(), "reward:redeem", identityPrincipal.UserID(), key, requestHash, func() (any, error) {
		if !h.limiter.Allow(identityPrincipal.UserID()) {
			return nil, errRewardRedeemRateLimited
		}
		return h.service.Redeem(c.Request.Context(), ports.RedeemCommand{
			UserID: identityPrincipal.UserID(), Code: request.Code,
		})
	})
	if err != nil {
		if errors.Is(err, errRewardRedeemRateLimited) {
			finishMetrics(redemptionOutcomeRateLimited, 0, nil)
			writeRewardRateLimitedError(c)
			return
		}
		if idempotency.IsConflict(err) {
			finishMetrics(redemptionOutcomeInvalid, 0, err)
			idempotency.DefaultMetrics.Observe(idempotency.OutcomeConflict)
			response.MachineError(c, http.StatusConflict, codeRewardCodeIdempotencyConflict, "幂等请求与原请求不一致")
			return
		}
		finishMetrics(redemptionOutcomeForError(err), 0, err)
		writeRewardRedeemError(c, err)
		return
	}
	result, ok := data.(ports.RedeemResult)
	if !ok {
		finishMetrics(redemptionOutcomeInternalError, 0, fmt.Errorf("unexpected reward redemption result type"))
		writeRewardUnavailableError(c, true)
		return
	}
	if replayed {
		finishMetrics(redemptionOutcomeReplayed, 0, nil)
		idempotency.DefaultMetrics.Observe(idempotency.OutcomeReplay)
	} else {
		finishMetrics(redemptionOutcomeSuccess, result.CreditedAmount, nil)
	}
	if replayed {
		result.Replayed = true
	}
	result.Redemption.UserPublicID = identityPrincipal.PublicID()
	result.Redemption.DisplayName = identityPrincipal.DisplayName()
	contract.RenderSuccess(c, result)
}

func redemptionOutcomeForError(err error) redemptionOutcome {
	switch {
	case errors.Is(err, rewardsdomain.ErrInvalidCode), errors.Is(err, rewardsdomain.ErrCodeUnavailable), errors.Is(err, rewardsdomain.ErrWalletFrozen):
		return redemptionOutcomeInvalid
	default:
		return redemptionOutcomeInternalError
	}
}

func (h *UserHandler) ListRedemptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	input := ports.ListRedemptionsInput{UserID: identityPrincipal.UserID(), Limit: 20}
	if raw, exists := c.GetQuery("before_id"); exists {
		raw = strings.TrimSpace(raw)
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			writeRewardInputError(c, codeRewardPaginationInvalid, "before_id 无效")
			return
		}
		input.BeforeID = value
	}
	if raw, exists := c.GetQuery("limit"); exists {
		raw = strings.TrimSpace(raw)
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeRewardInputError(c, codeRewardPaginationInvalid, "limit 无效")
			return
		}
		input.Limit = value
	}
	page, err := h.service.ListRedemptions(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	contract.RenderSuccess(c, page)
}
