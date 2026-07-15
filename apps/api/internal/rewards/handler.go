package rewards

import (
	"errors"
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
}

type Handler struct {
	service RewardService
}

func NewHandler(service RewardService) *Handler { return &Handler{service: service} }

type campaignMutationRequest struct {
	DisplayName    string     `json:"display_name"`
	InternalNote   string     `json:"internal_note"`
	CreditAmount   int64      `json:"credit_amount"`
	MaxRedemptions int64      `json:"max_redemptions"`
	StartsAt       *time.Time `json:"starts_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Reason         string     `json:"reason"`
}

type issuePublicCodeRequest struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type rewardReasonRequest struct {
	Reason string `json:"reason"`
}

type cancelCampaignRequest struct {
	Confirmed bool   `json:"confirmed"`
	Reason    string `json:"reason"`
}

type redeemRequest struct {
	Code string `json:"code"`
}

func (h *Handler) CreateCampaign(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var request campaignMutationRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	campaign, err := h.service.CreateCampaign(c.Request.Context(), CreateCampaignCommand{
		DisplayName: request.DisplayName, InternalNote: request.InternalNote,
		CreditAmount: request.CreditAmount, MaxRedemptions: request.MaxRedemptions,
		StartsAt: request.StartsAt, ExpiresAt: request.ExpiresAt,
		ActorUserID: actorID, Reason: request.Reason,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) UpdateCampaign(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request campaignMutationRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	campaign, err := h.service.UpdateCampaign(c.Request.Context(), UpdateCampaignCommand{
		CampaignID: campaignID, DisplayName: request.DisplayName, InternalNote: request.InternalNote,
		CreditAmount: request.CreditAmount, MaxRedemptions: request.MaxRedemptions,
		StartsAt: request.StartsAt, ExpiresAt: request.ExpiresAt,
		ActorUserID: actorID, Reason: request.Reason,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) IssuePublicCode(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request issuePublicCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	issued, err := h.service.IssuePublicCode(c.Request.Context(), IssuePublicCodeCommand{
		CampaignID: campaignID, Code: request.Code, ActorUserID: actorID, Reason: request.Reason,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.SuccessDTO(c, issued)
}

func (h *Handler) ActivateCampaign(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request rewardReasonRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	campaign, err := h.service.ActivateCampaign(c.Request.Context(), ActivateCampaignCommand{
		CampaignID: campaignID, ActorUserID: actorID, Reason: request.Reason,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) PauseCampaign(c *gin.Context) {
	h.handleLifecycleCommand(c, func(campaignID, actorID int64, reason string) (Campaign, error) {
		return h.service.PauseCampaign(c.Request.Context(), PauseCampaignCommand{
			CampaignID: campaignID, ActorUserID: actorID, Reason: reason,
		})
	})
}

func (h *Handler) ResumeCampaign(c *gin.Context) {
	h.handleLifecycleCommand(c, func(campaignID, actorID int64, reason string) (Campaign, error) {
		return h.service.ResumeCampaign(c.Request.Context(), ResumeCampaignCommand{
			CampaignID: campaignID, ActorUserID: actorID, Reason: reason,
		})
	})
}

func (h *Handler) CancelCampaign(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request cancelCampaignRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	campaign, err := h.service.CancelCampaign(c.Request.Context(), CancelCampaignCommand{
		CampaignID: campaignID, ActorUserID: actorID, Confirmed: request.Confirmed, Reason: request.Reason,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) DeleteCampaign(c *gin.Context) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request rewardReasonRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if err := h.service.DeleteCampaign(c.Request.Context(), DeleteCampaignCommand{
		CampaignID: campaignID, ActorUserID: actorID, Reason: request.Reason,
	}); err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, struct{}{})
}

func (h *Handler) handleLifecycleCommand(
	c *gin.Context,
	command func(campaignID, actorID int64, reason string) (Campaign, error),
) {
	actorID, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	var request rewardReasonRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	campaign, err := command(campaignID, actorID, request.Reason)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) ListCampaigns(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	input := ListCampaignsInput{Limit: 20}
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
		if err != nil || value < 1 || value > 100 {
			response.Error(c, http.StatusBadRequest, "limit 无效")
			return
		}
		input.Limit = value
	}
	page, err := h.service.ListCampaigns(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, page)
}

func (h *Handler) GetCampaign(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	campaignID, ok := parseRewardPathID(c)
	if !ok {
		return
	}
	campaign, err := h.service.GetCampaign(c.Request.Context(), campaignID)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, campaign)
}

func (h *Handler) Redeem(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "奖励服务暂不可用", nil)
		return
	}
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return
	}
	var request redeemRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	result, err := h.service.Redeem(c.Request.Context(), RedeemCommand{
		UserID: identityPrincipal.UserID(), Code: request.Code,
	})
	if err != nil {
		writeRewardError(c, err)
		return
	}
	response.SuccessDTO(c, result)
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

func decodeRewardJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxRewardRequestBytes, destination)
}

func parseRewardPathID(c *gin.Context) (int64, bool) {
	value, err := strconv.ParseInt(c.Param("campaign_id"), 10, 64)
	if err != nil || value <= 0 {
		response.Error(c, http.StatusBadRequest, "campaign_id 无效")
		return 0, false
	}
	return value, true
}

func writeRewardError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCommand):
		response.Error(c, http.StatusBadRequest, "参数错误")
	case errors.Is(err, ErrCampaignNotFound):
		response.Error(c, http.StatusNotFound, "奖励活动不存在")
	case errors.Is(err, ErrCampaignNotDraft), errors.Is(err, ErrPublicCodeExists),
		errors.Is(err, ErrPublicCodeRequired), errors.Is(err, ErrCampaignExhausted),
		errors.Is(err, ErrAlreadyRedeemed), errors.Is(err, ErrCampaignNotActive),
		errors.Is(err, ErrCampaignNotPaused), errors.Is(err, ErrCampaignRulesFrozen),
		errors.Is(err, ErrCampaignNotEmpty), errors.Is(err, ErrCampaignExpired):
		response.Error(c, http.StatusConflict, "奖励活动状态冲突")
	case errors.Is(err, ErrCodeUnavailable):
		response.Error(c, http.StatusUnprocessableEntity, "兑换码无效或已失效")
	case errors.Is(err, credits.ErrWalletFrozen):
		response.Error(c, http.StatusLocked, "当前账户无法兑换奖励")
	default:
		response.InternalError(c, http.StatusInternalServerError, "奖励操作失败", err)
	}
}
