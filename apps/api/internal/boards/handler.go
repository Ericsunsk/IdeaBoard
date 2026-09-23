package boards

import (
	"encoding/json"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service       *Service
	publicOrigins config.PublicOriginsConfig
	replay        *idempotency.Store
}

// Board POST replay scopes. Keys stay per (scope, user, key); only the
// success DTO and the request digest are cached, never secrets.
const (
	boardInviteMemberScope   = "board:invite-member"
	boardCreateShareScope    = "board:create-share-token"
	boardRevokeShareScope    = "board:revoke-share-token"
	boardJoinByShareScope    = "board:join-by-share-token"
	boardIdempotencyConflict = "幂等请求与原请求不一致"
)

type boardDTO struct {
	BoardID            string      `json:"board_id"`
	Name               string      `json:"name"`
	State              interface{} `json:"state"`
	AccessRole         string      `json:"access_role"`
	Revision           int64       `json:"revision"`
	StateSchemaVersion int32       `json:"state_schema_version"`
	CreatedAt          int64       `json:"created_at"`
	UpdatedAt          int64       `json:"updated_at"`
}

type boardShareTokenDTO struct {
	ID          int64  `json:"id"`
	Token       string `json:"token,omitempty"`
	TokenPrefix string `json:"token_prefix"`
	Role        string `json:"role"`
	Revoked     bool   `json:"revoked"`
	UsageCount  int64  `json:"usage_count"`
	MaxUses     *int32 `json:"max_uses,omitempty"`
	LastUsedAt  *int64 `json:"last_used_at"`
	RevokedAt   *int64 `json:"revoked_at"`
	ExpiresAt   *int64 `json:"expires_at"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	ShareURL    string `json:"share_url,omitempty"`
}

type boardListResponse struct {
	List []boardDTO `json:"list"`
}

type boardResponse struct {
	Board       boardDTO `json:"board"`
	NewRevision int64    `json:"new_revision"`
}

type boardDeleteResponse struct {
	Deleted bool `json:"deleted"`
}

type boardMemberListResponse struct {
	List []MemberWithUser `json:"list"`
}

type boardMemberResponse struct {
	Member *MemberWithUser `json:"member"`
}

type boardShareTokenResponse struct {
	Token boardShareTokenDTO `json:"token"`
}

type boardShareTokenListResponse struct {
	List []boardShareTokenDTO `json:"list"`
}

type boardShareTokenRevokeResponse struct {
	Revoked bool `json:"revoked"`
}

func NewHandler(service *Service, publicOrigins config.PublicOriginsConfig) *Handler {
	return &Handler{service: service, publicOrigins: publicOrigins, replay: idempotency.NewStore(idempotency.DefaultStoreOptions())}
}

// replayStore is nil-safe: handlers built without NewHandler still enforce
// keys and skip replay instead of failing.
func (h *Handler) replayStore() *idempotency.Store {
	if h == nil {
		return nil
	}
	return h.replay
}

// requireBoardIdempotencyKey enforces Idempotency-Key on board POST writes.
// Missing/invalid keys render a stable machine code (no new namespace).
func requireBoardIdempotencyKey(c *gin.Context) (string, bool) {
	key, err := idempotency.Parse(c.GetHeader(idempotency.Header))
	if err != nil {
		idempotency.DefaultMetrics.ObserveError(err)
		response.MachineError(c, idempotency.RenderStatusOf(err), idempotency.RenderCodeOf(err), err.Error())
		return "", false
	}
	return key, true
}

// executeBoardWrite atomically coordinates a process-local write and serves
// the first result to concurrent/retried callers. Durable board operations
// still own their database facts; this closes the handler-side race window.
func executeBoardWrite(c *gin.Context, store *idempotency.Store, scope string, userID int64, key, requestHash string, execute func() (any, error), handleError func(error)) bool {
	cached, replayed, err := store.Do(c.Request.Context(), scope, userID, key, requestHash, execute)
	if err != nil {
		if idempotency.IsConflict(err) {
			idempotency.DefaultMetrics.Observe(idempotency.OutcomeConflict)
			response.MachineError(c, idempotency.RenderStatusOf(err), idempotency.RenderCodeOf(err), boardIdempotencyConflict)
		} else {
			handleError(err)
		}
		return true
	}
	if replayed {
		idempotency.DefaultMetrics.Observe(idempotency.OutcomeReplay)
	}
	response.SuccessDTO(c, cached)
	return true
}

func requireBoardUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func (h *Handler) ListBoards(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	list, err := h.service.ListBoards(c.Request.Context(), userID)
	if err != nil {
		respondBoardError(c, err, nil, "获取画布失败")
		return
	}
	result := make([]boardDTO, 0, len(list))
	for _, board := range list {
		result = append(result, toBoardDTO(board.Board, board.Role))
	}
	response.SuccessDTO(c, boardListResponse{List: result})
}

func (h *Handler) UpsertBoard(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	if boardID == "" {
		writeBoardInputError(c, "缺少画布ID")
		return
	}
	var req UpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBoardInputError(c, "参数错误")
		return
	}
	board, err := h.service.UpsertBoard(c.Request.Context(), userID, boardID, &req)
	if err != nil {
		respondBoardError(c, err, upsertBoardErrorRows, "更新画布失败")
		return
	}
	response.SuccessDTO(c, boardResponse{
		Board:       toBoardDTO(board.Board, board.Role),
		NewRevision: board.Board.Revision,
	})
}

func (h *Handler) DeleteBoard(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	if boardID == "" {
		writeBoardInputError(c, "缺少画布ID")
		return
	}
	if err := h.service.DeleteBoard(c.Request.Context(), userID, boardID); err != nil {
		respondBoardError(c, err, deleteBoardErrorRows, "删除画布失败")
		return
	}
	response.SuccessDTO(c, boardDeleteResponse{Deleted: true})
}

func (h *Handler) ListMembers(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	boardID := strings.TrimSpace(c.Param("board_id"))
	if !ok {
		return
	}
	members, err := h.service.ListMembers(c.Request.Context(), userID, boardID)
	if err != nil {
		h.respondBoardMemberError(c, err, "获取成员列表失败")
		return
	}
	response.SuccessDTO(c, boardMemberListResponse{List: members})
}

func (h *Handler) InviteMember(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	key, ok := requireBoardIdempotencyKey(c)
	if !ok {
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	var req InviteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBoardInputError(c, "参数错误")
		return
	}
	requestHash := idempotency.HashRequest(boardID, req.MemberPublicID, req.Role)
	executeBoardWrite(c, h.replayStore(), boardInviteMemberScope, userID, key, requestHash, func() (any, error) {
		member, err := h.service.InviteMember(c.Request.Context(), userID, boardID, req.MemberPublicID, req.Role)
		if err != nil {
			return nil, err
		}
		return boardMemberResponse{Member: member}, nil
	}, func(err error) { h.respondBoardMemberError(c, err, "邀请成员失败") })
}

func (h *Handler) UpdateMemberRole(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	boardID := strings.TrimSpace(c.Param("board_id"))
	memberPublicID := strings.TrimSpace(c.Param("member_public_id"))
	if !ok {
		return
	}
	if memberPublicID == "" {
		writeBoardInputError(c, "参数错误")
		return
	}
	var req UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBoardInputError(c, "参数错误")
		return
	}
	member, err := h.service.UpdateMemberRole(c.Request.Context(), userID, boardID, memberPublicID, req.Role)
	if err != nil {
		h.respondBoardMemberError(c, err, "更新成员角色失败")
		return
	}
	response.SuccessDTO(c, boardMemberResponse{Member: member})
}

func (h *Handler) RemoveMember(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	boardID := strings.TrimSpace(c.Param("board_id"))
	memberPublicID := strings.TrimSpace(c.Param("member_public_id"))
	if !ok {
		return
	}
	if memberPublicID == "" {
		writeBoardInputError(c, "参数错误")
		return
	}
	if err := h.service.RemoveMember(c.Request.Context(), userID, boardID, memberPublicID); err != nil {
		h.respondBoardMemberError(c, err, "移除成员失败")
		return
	}
	response.SuccessDTO(c, boardDeleteResponse{Deleted: true})
}

func (h *Handler) CreateShareToken(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	key, ok := requireBoardIdempotencyKey(c)
	if !ok {
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	var req CreateShareTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBoardInputError(c, "参数错误")
		return
	}
	var expiresAt string
	if req.ExpiresAt != nil {
		expiresAt = strconv.FormatInt(*req.ExpiresAt, 10)
	}
	requestHash := idempotency.HashRequest(boardID, req.Role, expiresAt)
	executeBoardWrite(c, h.replayStore(), boardCreateShareScope, userID, key, requestHash, func() (any, error) {
		shareToken, _, err := h.service.CreateShareTokenIdempotent(c.Request.Context(), userID, boardID, req.Role, req.ExpiresAt, key, requestHash)
		if err != nil {
			return nil, err
		}
		return boardShareTokenResponse{Token: toBoardShareTokenDTO(*shareToken, h.publicOrigins.Web)}, nil
	}, func(err error) { h.respondShareTokenError(c, err, "创建分享链接失败") })
}

func (h *Handler) ListShareTokens(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	boardID := strings.TrimSpace(c.Param("board_id"))
	if !ok {
		return
	}
	list, err := h.service.ListShareTokens(c.Request.Context(), userID, boardID)
	if err != nil {
		h.respondShareTokenError(c, err, "获取分享链接失败")
		return
	}
	baseURL := h.publicOrigins.Web
	result := make([]boardShareTokenDTO, 0, len(list))
	for _, item := range list {
		result = append(result, toBoardShareTokenDTO(item, baseURL))
	}
	response.SuccessDTO(c, boardShareTokenListResponse{List: result})
}

func (h *Handler) RevokeShareToken(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	key, ok := requireBoardIdempotencyKey(c)
	if !ok {
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	tokenID, err := strconv.ParseInt(strings.TrimSpace(c.Param("token_id")), 10, 64)
	if err != nil || tokenID <= 0 {
		writeBoardInputError(c, "参数错误")
		return
	}
	requestHash := idempotency.HashRequest(boardID, strconv.FormatInt(tokenID, 10))
	executeBoardWrite(c, h.replayStore(), boardRevokeShareScope, userID, key, requestHash, func() (any, error) {
		if err := h.service.RevokeShareToken(c.Request.Context(), userID, boardID, tokenID); err != nil {
			return nil, err
		}
		return boardShareTokenRevokeResponse{Revoked: true}, nil
	}, func(err error) { h.respondShareTokenError(c, err, "撤销分享链接失败") })
}

func (h *Handler) JoinByShareToken(c *gin.Context) {
	userID, ok := requireBoardUserID(c)
	if !ok {
		return
	}
	key, ok := requireBoardIdempotencyKey(c)
	if !ok {
		return
	}
	var req JoinByShareTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBoardInputError(c, "参数错误")
		return
	}
	requestHash := idempotency.HashRequest(strings.TrimSpace(req.Token))
	executeBoardWrite(c, h.replayStore(), boardJoinByShareScope, userID, key, requestHash, func() (any, error) {
		board, err := h.service.JoinBoardByShareToken(c.Request.Context(), userID, req.Token)
		if err != nil {
			return nil, err
		}
		return boardResponse{Board: toBoardDTO(board.Board, board.Role)}, nil
	}, func(err error) { h.respondShareTokenError(c, err, "加入画布失败") })
}

func (h *Handler) respondBoardMemberError(c *gin.Context, err error, internalMessage string) {
	respondBoardError(c, err, boardMemberErrorRows, internalMessage)
}

func (h *Handler) respondShareTokenError(c *gin.Context, err error, internalMessage string) {
	respondBoardError(c, err, shareTokenErrorRows, internalMessage)
}

func toBoardDTO(board Board, role AccessRole) boardDTO {
	return boardDTO{
		BoardID:            board.BoardID,
		Name:               board.Name,
		State:              parseBoardState(board.State),
		AccessRole:         string(role),
		Revision:           board.Revision,
		StateSchemaVersion: int32(StateSchemaVersion(&board)),
		CreatedAt:          board.CreatedAt.UnixMilli(),
		UpdatedAt:          board.UpdatedAt.UnixMilli(),
	}
}

func toBoardShareTokenDTO(token ShareToken, baseURL string) boardShareTokenDTO {
	var lastUsedAt *int64
	if token.LastUsedAt != nil {
		v := token.LastUsedAt.UnixMilli()
		lastUsedAt = &v
	}
	var revokedAt *int64
	if token.RevokedAt != nil {
		v := token.RevokedAt.UnixMilli()
		revokedAt = &v
	}
	var expiresAt *int64
	if token.ExpiresAt != nil {
		v := token.ExpiresAt.UnixMilli()
		expiresAt = &v
	}
	shareURL := ""
	if token.Token != "" {
		shareURL = BuildBoardShareURL(baseURL, token.Token)
	}
	return boardShareTokenDTO{
		ID:          token.ID,
		Token:       token.Token,
		TokenPrefix: token.TokenPrefix,
		Role:        token.Role,
		Revoked:     token.Revoked,
		UsageCount:  token.UsageCount,
		MaxUses:     token.MaxUses,
		LastUsedAt:  lastUsedAt,
		RevokedAt:   revokedAt,
		ExpiresAt:   expiresAt,
		CreatedAt:   token.CreatedAt.UnixMilli(),
		UpdatedAt:   token.UpdatedAt.UnixMilli(),
		ShareURL:    shareURL,
	}
}

func parseBoardState(raw string) interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	return parsed
}
