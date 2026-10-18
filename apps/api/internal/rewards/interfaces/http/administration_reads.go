package http

import (
	"context"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/rewards/ports"

	"github.com/gin-gonic/gin"
)

type AdminReadHandler struct {
	service            ports.AdminReads
	identityProjection ports.IdentityProjectionReader
}

func NewAdminReadHandler(service ports.AdminReads, identityProjection ports.IdentityProjectionReader) *AdminReadHandler {
	return &AdminReadHandler{service: service, identityProjection: identityProjection}
}

type searchRewardCodesRequest struct {
	Query    string                 `json:"query"`
	Status   ports.RewardCodeStatus `json:"status"`
	BeforeID *int64                 `json:"before_id"`
	Limit    *int                   `json:"limit"`
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
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
		return
	}
	var request searchRewardCodesRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
		return
	}
	input := ports.SearchRewardCodesInput{Query: request.Query, Status: request.Status, Limit: contract.DefaultRewardLimit}
	if request.BeforeID != nil {
		if *request.BeforeID <= 0 {
			writeRewardInputError(c, codeRewardPaginationInvalid, errorrender.MsgRewardInvalidInput)
			return
		}
		input.BeforeID = *request.BeforeID
	}
	if request.Limit != nil {
		// A present zero stays invalid (body fields have no "absent
		// means default" rule); NormalizeRewardLimit only maps the
		// bounds, so the zero guard preserves the frozen outcome.
		limit, err := contract.NormalizeRewardLimit(*request.Limit)
		if err != nil || *request.Limit == 0 {
			writeRewardInputError(c, codeRewardPaginationInvalid, errorrender.MsgRewardInvalidInput)
			return
		}
		input.Limit = limit
	}
	page, err := h.service.SearchRewardCodes(c.Request.Context(), input)
	if err != nil {
		writeRewardError(c, err)
		return
	}
	contract.RenderSuccess(c, page)
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
		Limit:        contract.DefaultRewardLimit,
	}
	if raw, exists := c.GetQuery(contract.BeforeIDParam); exists {
		trimmed := strings.TrimSpace(raw)
		value, err := contract.ParseBeforeID(trimmed)
		if err != nil || trimmed == "" {
			writeRewardInputError(c, codeRewardPaginationInvalid, errorrender.MsgRewardPaginationInvalid)
			return
		}
		input.BeforeID = value
	}
	if raw, exists := c.GetQuery(contract.LimitParam); exists {
		trimmed := strings.TrimSpace(raw)
		value, err := contract.ParseRewardLimit(trimmed)
		if err != nil || trimmed == "" {
			writeRewardInputError(c, codeRewardPaginationInvalid, errorrender.MsgRewardPaginationInvalid2)
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
	contract.RenderSuccess(c, page)
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
	// :redemption_id shares the requireAdminRewardCode exception (no
	// public_id column on reward_redemptions; rdm_ reserved in the
	// kernel for the migration). Operator-gated only.
	redemptionID, err := strconv.ParseInt(strings.TrimSpace(c.Param("redemption_id")), 10, 64)
	if err != nil || redemptionID <= 0 {
		writeRewardInputError(c, codeRewardRedemptionIDInvalid, errorrender.MsgRewardRedemptionIDInvalid)
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
	contract.RenderSuccess(c, detail)
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
