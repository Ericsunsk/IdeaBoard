package http

import (
	"ideaboard/api/internal/platform/errorrender"
	platformhttp "ideaboard/api/internal/platform/http"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/jsonlimit"
	"ideaboard/api/internal/rewards/ports"

	"github.com/gin-gonic/gin"
)

const maxRewardRequestBytes int64 = 64 << 10

type AdminCommandHandler struct {
	service ports.AdminCommandService
}

func NewAdminCommandHandler(service ports.AdminCommandService) *AdminCommandHandler {
	return &AdminCommandHandler{service: service}
}

type createRewardCodeRequest struct {
	DisplayName    string     `json:"display_name"`
	Code           string     `json:"code"`
	CreditAmount   int64      `json:"credit_amount"`
	MaxRedemptions int64      `json:"max_redemptions"`
	ExpiresAt      *time.Time `json:"expires_at"`
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

func (h *AdminCommandHandler) CreateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !platformhttp.RequireDependencyCode(c, h != nil && h.service != nil, codeRewardServiceUnavailable, errorrender.MsgRewardServiceUnavailable) {
		return
	}
	actorID, ok := requireAdmin(c)
	if !ok {
		return
	}
	var request createRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
		return
	}
	// Idempotency-Key header is the only create key (header-only; the
	// body carries no idempotency field — strict decoding rejects it).
	// Key reuse with a different payload conflicts in the durable
	// uniqueness facts instead of forking codes.
	key, err := contract.IdempotencyKeyFromHeader(c)
	if err != nil {
		idempotency.DefaultMetrics.Observe(idempotency.OutcomeMissing)
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
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
	contract.RenderSuccess(c, created)
}

func (h *AdminCommandHandler) UpdateRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !platformhttp.RequireDependencyCode(c, h != nil && h.service != nil, codeRewardServiceUnavailable, errorrender.MsgRewardServiceUnavailable) {
		return
	}
	rewardCodeID, actorID, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request updateRewardCodeRequest
	if err := decodeRewardJSON(c, &request); err != nil {
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
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
	contract.RenderSuccess(c, updated)
}

func (h *AdminCommandHandler) SetRewardCodeEnabled(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !platformhttp.RequireDependencyCode(c, h != nil && h.service != nil, codeRewardServiceUnavailable, errorrender.MsgRewardServiceUnavailable) {
		return
	}
	rewardCodeID, actorID, ok := requireAdminRewardCode(c)
	if !ok {
		return
	}
	var request setRewardCodeEnabledRequest
	if err := decodeRewardJSON(c, &request); err != nil || request.Enabled == nil {
		writeRewardInputError(c, codeRewardInvalidInput, errorrender.MsgRewardInvalidInput)
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
	contract.RenderSuccess(c, updated)
}

func (h *AdminCommandHandler) DeleteRewardCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !platformhttp.RequireDependencyCode(c, h != nil && h.service != nil, codeRewardServiceUnavailable, errorrender.MsgRewardServiceUnavailable) {
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
	contract.RenderSuccess(c, deleted)
}

func requireAdmin(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

// requireAdminRewardCode resolves the numeric :id to the internal reward
// code row. FORMAL EXCEPTION to the opaque-id policy (parent #809 S4):
// reward_codes / reward_redemptions carry no public_id column (raw-SQL
// store, bigint PKs), so there is no rwd_/rdm_ value to resolve yet.
// Migrating the wire needs a schema migration (public_id + backfill) plus
// a coordinated break of the control/legacy admin envelope, the generated
// control clients, and the admin frontend — deferred as a tracked
// follow-up. The kernel prefixes (contract.PrefixRewardCode /
// PrefixRewardRedemption) are reserved for that migration and must not be
// minted until the columns land. This surface stays operator-gated
// (control:credits + audit) so the numeric ids never reach public callers.
func requireAdminRewardCode(c *gin.Context) (int64, int64, bool) {
	actorID, ok := requireAdmin(c)
	if !ok {
		return 0, 0, false
	}
	rewardCodeID, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || rewardCodeID <= 0 {
		writeRewardInputError(c, codeRewardCodeIDInvalid, errorrender.MsgRewardCodeIDInvalid)
		return 0, 0, false
	}
	return rewardCodeID, actorID, true
}

func decodeRewardJSON(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxRewardRequestBytes, destination)
}
