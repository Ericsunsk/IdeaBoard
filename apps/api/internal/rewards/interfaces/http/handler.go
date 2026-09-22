package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/rewards/ports"

	"github.com/gin-gonic/gin"
)

const maxRewardRequestBytes int64 = 64 << 10

type UserHandler struct {
	service ports.UserService
	limiter *redemptionAttemptLimiter
	replay  *idempotency.Store
}

type AdminCommandHandler struct {
	service ports.AdminCommandService
}

type AdminReadHandler struct {
	service            ports.AdminReads
	identityProjection ports.IdentityProjectionReader
}

type RedemptionHandlerOptions struct {
	Now func() time.Time
}

func NewUserHandler(service ports.UserService) *UserHandler {
	return NewUserHandlerWithOptions(service, RedemptionHandlerOptions{})
}

func NewUserHandlerWithOptions(service ports.UserService, options RedemptionHandlerOptions) *UserHandler {
	return &UserHandler{
		service: service,
		limiter: newRedemptionAttemptLimiter(options.Now),
		replay:  idempotency.NewStore(idempotency.DefaultStoreOptions()),
	}
}

var errRewardRedeemRateLimited = errors.New("reward redemption rate limited")

func NewAdminCommandHandler(service ports.AdminCommandService) *AdminCommandHandler {
	return &AdminCommandHandler{service: service}
}

func NewAdminReadHandler(service ports.AdminReads, identityProjection ports.IdentityProjectionReader) *AdminReadHandler {
	return &AdminReadHandler{service: service, identityProjection: identityProjection}
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
	Query    string                 `json:"query"`
	Status   ports.RewardCodeStatus `json:"status"`
	BeforeID *int64                 `json:"before_id"`
	Limit    *int                   `json:"limit"`
}

type redeemRequest struct {
	Code string `json:"code"`
}

func (h *AdminCommandHandler) CreateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	actorID, ok := requireAdmin(c)
	if !ok {
		return
	}
	var request createRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	// Idempotency-Key header is canonical; the body idempotency_key stays
	// accepted as fallback. A mismatch conflicts instead of forking codes.
	key, err := idempotency.Resolve(c.GetHeader(idempotency.Header), request.IdempotencyKey)
	if err != nil {
		if idempotency.IsConflict(err) {
			idempotency.DefaultMetrics.Observe(idempotency.OutcomeConflict)
			response.MachineError(c, http.StatusConflict, codeRewardCodeIdempotencyConflict, "幂等请求与原创建参数不一致")
			return
		}
		idempotency.DefaultMetrics.Observe(idempotency.OutcomeMissing)
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	created, err := h.service.CreateRewardCode(c.Request.Context(), ports.CreateRewardCodeCommand{
		DisplayName:    request.DisplayName,
		Code:           request.Code,
		CreditAmount:   request.CreditAmount,
		MaxRedemptions: request.MaxRedemptions,
		ExpiresAt:      request.ExpiresAt,
		IdempotencyKey: key,
		ActorUserID:    actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, created)
}

func (h *AdminCommandHandler) UpdateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	rewardCodeID, actorID, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request updateRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	updated, err := h.service.UpdateRewardCode(c.Request.Context(), ports.UpdateRewardCodeCommand{
		RewardCodeID:   rewardCodeID,
		DisplayName:    request.DisplayName,
		CreditAmount:   request.CreditAmount,
		MaxRedemptions: request.MaxRedemptions,
		ExpiresAt:      request.ExpiresAt,
		Code:           request.Code,
		ActorUserID:    actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, updated)
}

func (h *AdminCommandHandler) SetRewardCodeEnabled(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	rewardCodeID, actorID, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request setRewardCodeEnabledRequest
	if err := decodeRewardJSON(c, &request); err != nil || request.Enabled == nil {
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	updated, err := h.service.SetRewardCodeEnabled(c.Request.Context(), ports.SetRewardCodeEnabledCommand{
		RewardCodeID: rewardCodeID,
		Enabled:      *request.Enabled,
		ActorUserID:  actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, updated)
}

func (h *AdminCommandHandler) DeleteRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	rewardCodeID, actorID, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteRewardCode(c.Request.Context(), ports.DeleteRewardCodeCommand{
		RewardCodeID: rewardCodeID,
		ActorUserID:  actorID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, deleted)
}

func (h *AdminReadHandler) SearchRewardCodes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	if _, ok := requireAdmin(c); !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	var request searchRewardCodesRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	input := ports.SearchRewardCodesInput{Query: request.Query, Status: request.Status, Limit: 20}
	if request.BeforeID != nil {
		if *request.BeforeID <= 0 {
			writeRewardInputError(c, codeRewardPaginationInvalid, "参数错误")
			return
		}
		input.BeforeID = *request.BeforeID
	}
	if request.Limit != nil {
		if *request.Limit < 1 || *request.Limit > 100 {
			writeRewardInputError(c, codeRewardPaginationInvalid, "参数错误")
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

func (h *AdminReadHandler) ListAdminRedemptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	rewardCodeID, _, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	input := ports.ListAdminRedemptionsInput{
		RewardCodeID: rewardCodeID,
		Limit:        20,
	}
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
	page, err := h.service.ListAdminRedemptions(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	if err := h.enrichAdminRedemptionPage(c.Request.Context(), &page); err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *AdminReadHandler) GetAdminRedemption(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeRewardUnavailableError(c, false)
		return
	}
	rewardCodeID, _, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	redemptionID, err := strconv.ParseInt(strings.TrimSpace(c.Param("redemption_id")), 10, 64)
	if err != nil || redemptionID <= 0 {
		writeRewardInputError(c, codeRewardRedemptionIDInvalid, "兑换记录 ID 无效")
		return
	}
	detail, err := h.service.GetAdminRedemption(c.Request.Context(), ports.GetAdminRedemptionInput{
		RewardCodeID: rewardCodeID,
		RedemptionID: redemptionID,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	if err := h.enrichAdminRedemption(c.Request.Context(), &detail); err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, detail)
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
	key, err := idempotency.Parse(c.GetHeader(idempotency.Header))
	if err != nil {
		idempotency.DefaultMetrics.ObserveError(err)
		writeRewardInputError(c, codeRewardInvalidInput, "参数错误")
		return
	}
	var request redeemRequest
	if err := decodeRewardJSON(c, &request); err != nil {
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
			writeRewardRateLimitedError(c)
			return
		}
		if idempotency.IsConflict(err) {
			idempotency.DefaultMetrics.Observe(idempotency.OutcomeConflict)
			response.MachineError(c, http.StatusConflict, codeRewardCodeIdempotencyConflict, "幂等请求与原请求不一致")
			return
		}
		writeRewardRedeemError(c, err)
		return
	}
	if replayed {
		idempotency.DefaultMetrics.Observe(idempotency.OutcomeReplay)
	}
	result, ok := data.(ports.RedeemResult)
	if !ok {
		writeRewardUnavailableError(c, true)
		return
	}
	if replayed {
		result.Replayed = true
	}
	result.Redemption.UserPublicID = identityPrincipal.PublicID()
	result.Redemption.DisplayName = identityPrincipal.DisplayName()
	response.SuccessDTO(c, result)
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
	response.SuccessDTO(c, page)
}

func requireAdmin(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *AdminReadHandler) enrichAdminRedemptionPage(ctx context.Context, page *ports.AdminRedemptionPage) error {
	if h == nil || page == nil {
		return nil
	}
	if h.identityProjection == nil {
		return fmt.Errorf("resolve reward redemption identity: projection unavailable")
	}
	userIDs := make([]int64, 0, len(page.Redemptions))
	for _, redemption := range page.Redemptions {
		userIDs = append(userIDs, redemption.UserID)
	}
	identities, err := h.identityProjection.ResolveIdentities(ctx, userIDs)
	if err != nil {
		return fmt.Errorf("resolve reward redemption identities: %w", err)
	}
	for index := range page.Redemptions {
		identity, ok := identities[page.Redemptions[index].UserID]
		if !ok {
			return fmt.Errorf("resolve reward redemption identity: user %d not found", page.Redemptions[index].UserID)
		}
		page.Redemptions[index].UserPublicID = identity.PublicID
		page.Redemptions[index].DisplayName = identity.DisplayName
	}
	return nil
}

func (h *AdminReadHandler) enrichAdminRedemption(ctx context.Context, detail *ports.AdminRedemptionDetail) error {
	if h == nil || detail == nil {
		return nil
	}
	if h.identityProjection == nil {
		return fmt.Errorf("resolve reward redemption identity: projection unavailable")
	}
	identity, err := h.identityProjection.ResolveIdentity(ctx, detail.UserID)
	if err != nil {
		return fmt.Errorf("resolve reward redemption identity: %w", err)
	}
	detail.UserPublicID = identity.PublicID
	detail.DisplayName = identity.DisplayName
	return nil
}

func requireAdminRewardCode(c *gin.Context) (int64, int64, bool) {
	actorID, ok := requireAdmin(c)
	if !ok {
		return 0, 0, false
	}
	rewardCodeID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || rewardCodeID <= 0 {
		writeRewardInputError(c, codeRewardCodeIDInvalid, "兑换码 ID 无效")
		return 0, 0, false
	}
	return rewardCodeID, actorID, true
}

func decodeRewardJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxRewardRequestBytes, destination)
}
