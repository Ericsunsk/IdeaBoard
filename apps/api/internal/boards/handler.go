package boards

import (
	"encoding/json"
	"errors"
	"strings"

	"api/internal/platform/response"
	"api/internal/users"

	"github.com/gin-gonic/gin"
)

type BoardHandler struct {
	service *BoardService
}

type boardDTO struct {
	BoardID    string      `json:"board_id"`
	Name       string      `json:"name"`
	State      interface{} `json:"state"`
	AccessRole string      `json:"access_role"`
	CreatedAt  int64       `json:"created_at"`
	UpdatedAt  int64       `json:"updated_at"`
}

type boardShareTokenDTO struct {
	Token     string `json:"token"`
	Role      string `json:"role"`
	Revoked   bool   `json:"revoked"`
	ExpiresAt *int64 `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	ShareURL  string `json:"share_url"`
}

func NewBoardHandler(service *BoardService) *BoardHandler {
	return &BoardHandler{service: service}
}

func (h *BoardHandler) ListBoards(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	list, err := h.service.ListBoards(userID)
	if err != nil {
		response.InternalError(c, 500, "获取画布失败", err)
		return
	}
	result := make([]boardDTO, 0, len(list))
	for _, board := range list {
		result = append(result, toBoardDTO(board.Board, board.Role))
	}
	response.Success(c, gin.H{"list": result})
}

func (h *BoardHandler) UpsertBoard(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	if boardID == "" {
		response.Error(c, 400, "缺少画布ID")
		return
	}
	var req UpsertBoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	board, err := h.service.UpsertBoard(userID, boardID, &req)
	if err != nil {
		if errors.Is(err, ErrInvalidBoardInput) {
			response.Error(c, 400, "参数错误")
			return
		}
		if errors.Is(err, ErrBoardPermissionDenied) {
			response.Error(c, 403, "无权修改该画布")
			return
		}
		if errors.Is(err, ErrBoardNameConflict) {
			response.Error(c, 409, "画布名称已存在，请使用其他名称")
			return
		}
		response.InternalError(c, 500, "更新画布失败", err)
		return
	}
	response.Success(c, gin.H{"board": toBoardDTO(board.Board, board.Role)})
}

func (h *BoardHandler) DeleteBoard(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	boardID := strings.TrimSpace(c.Param("board_id"))
	if boardID == "" {
		response.Error(c, 400, "缺少画布ID")
		return
	}
	if err := h.service.DeleteBoard(userID, boardID); err != nil {
		if errors.Is(err, ErrInvalidBoardInput) {
			response.Error(c, 400, "参数错误")
			return
		}
		if errors.Is(err, ErrBoardNotFound) {
			response.Error(c, 404, "画布不存在")
			return
		}
		if errors.Is(err, ErrBoardPermissionDenied) {
			response.Error(c, 403, "无权删除该画布")
			return
		}
		if errors.Is(err, ErrBoardWorkspaceCleanupFailed) {
			response.InternalError(c, 500, "删除画布失败：文件清理失败", err)
			return
		}
		response.InternalError(c, 500, "删除画布失败", err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *BoardHandler) ListMembers(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	members, err := h.service.ListMembers(userID, boardID)
	if err != nil {
		h.respondBoardMemberError(c, err, "获取成员列表失败")
		return
	}
	response.Success(c, gin.H{"list": members})
}

func (h *BoardHandler) InviteMember(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req InviteBoardMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil || !users.IsValidStorageUID(req.MemberUID) {
		response.Error(c, 400, "参数错误")
		return
	}
	member, err := h.service.InviteMember(userID, boardID, req.MemberUID, req.Role)
	if err != nil {
		h.respondBoardMemberError(c, err, "邀请成员失败")
		return
	}
	response.Success(c, gin.H{"member": member})
}

func (h *BoardHandler) UpdateMemberRole(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	memberUID := strings.TrimSpace(c.Param("member_uid"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	if !users.IsValidStorageUID(memberUID) {
		response.Error(c, 400, "参数错误")
		return
	}
	var req UpdateBoardMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	member, err := h.service.UpdateMemberRole(userID, boardID, memberUID, req.Role)
	if err != nil {
		h.respondBoardMemberError(c, err, "更新成员角色失败")
		return
	}
	response.Success(c, gin.H{"member": member})
}

func (h *BoardHandler) RemoveMember(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	memberUID := strings.TrimSpace(c.Param("member_uid"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	if !users.IsValidStorageUID(memberUID) {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.service.RemoveMember(userID, boardID, memberUID); err != nil {
		h.respondBoardMemberError(c, err, "移除成员失败")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *BoardHandler) CreateShareToken(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req CreateBoardShareTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	shareToken, err := h.service.CreateShareToken(userID, boardID, req.Role, req.ExpiresAt)
	if err != nil {
		h.respondShareTokenError(c, err, "创建分享链接失败")
		return
	}
	baseURL := ResolvePublicBaseURL(requestScheme(c), c.Request.Host)
	response.Success(c, gin.H{
		"token": toBoardShareTokenDTO(*shareToken, baseURL),
	})
}

func (h *BoardHandler) ListShareTokens(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	list, err := h.service.ListShareTokens(userID, boardID)
	if err != nil {
		h.respondShareTokenError(c, err, "获取分享链接失败")
		return
	}
	baseURL := ResolvePublicBaseURL(requestScheme(c), c.Request.Host)
	result := make([]boardShareTokenDTO, 0, len(list))
	for _, item := range list {
		result = append(result, toBoardShareTokenDTO(item, baseURL))
	}
	response.Success(c, gin.H{"list": result})
}

func (h *BoardHandler) RevokeShareToken(c *gin.Context) {
	userID := c.GetUint64("user_id")
	boardID := strings.TrimSpace(c.Param("board_id"))
	token := strings.TrimSpace(c.Param("token"))
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	if token == "" {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.service.RevokeShareToken(userID, boardID, token); err != nil {
		h.respondShareTokenError(c, err, "撤销分享链接失败")
		return
	}
	response.Success(c, gin.H{"revoked": true})
}

func (h *BoardHandler) JoinByShareToken(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}
	var req JoinBoardByShareTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	board, err := h.service.JoinBoardByShareToken(userID, req.Token)
	if err != nil {
		h.respondShareTokenError(c, err, "加入画布失败")
		return
	}
	response.Success(c, gin.H{"board": toBoardDTO(board.Board, board.Role)})
}

func (h *BoardHandler) respondBoardMemberError(c *gin.Context, err error, internalMessage string) {
	switch {
	case errors.Is(err, ErrInvalidBoardInput):
		response.Error(c, 400, "参数错误")
	case errors.Is(err, ErrBoardNotFound):
		response.Error(c, 404, "画布不存在")
	case errors.Is(err, ErrBoardPermissionDenied):
		response.Error(c, 403, "仅 owner 可管理成员")
	case errors.Is(err, ErrBoardMemberNotFound):
		response.Error(c, 404, "成员不存在")
	case errors.Is(err, ErrBoardMemberInvalidRole):
		response.Error(c, 400, "角色无效，仅支持 editor/viewer")
	case errors.Is(err, ErrBoardMemberInviteSelf):
		response.Error(c, 400, "不能邀请或移除 owner 自己")
	default:
		response.InternalError(c, 500, internalMessage, err)
	}
}

func (h *BoardHandler) respondShareTokenError(c *gin.Context, err error, internalMessage string) {
	switch {
	case errors.Is(err, ErrInvalidBoardInput):
		response.Error(c, 400, "参数错误")
	case errors.Is(err, ErrBoardNotFound):
		response.Error(c, 404, "画布不存在")
	case errors.Is(err, ErrBoardPermissionDenied):
		response.Error(c, 403, "仅 owner 可管理分享链接")
	case errors.Is(err, ErrBoardMemberInvalidRole):
		response.Error(c, 400, "角色无效，仅支持 editor/viewer")
	case errors.Is(err, ErrShareTokenNotFound):
		response.Error(c, 404, "分享链接不存在")
	case errors.Is(err, ErrShareTokenRevoked):
		response.Error(c, 400, "分享链接已撤销")
	case errors.Is(err, ErrShareTokenExpired):
		response.Error(c, 400, "分享链接已过期")
	case errors.Is(err, ErrShareTokenInvalid):
		response.Error(c, 400, "分享链接无效")
	default:
		response.InternalError(c, 500, internalMessage, err)
	}
}

func toBoardDTO(board UserBoard, role BoardAccessRole) boardDTO {
	return boardDTO{
		BoardID:    board.BoardID,
		Name:       board.Name,
		State:      parseBoardState(board.State),
		AccessRole: string(role),
		CreatedAt:  board.CreatedAt.UnixMilli(),
		UpdatedAt:  board.UpdatedAt.UnixMilli(),
	}
}

func toBoardShareTokenDTO(token BoardShareToken, baseURL string) boardShareTokenDTO {
	var expiresAt *int64
	if token.ExpiresAt != nil {
		v := token.ExpiresAt.UnixMilli()
		expiresAt = &v
	}
	return boardShareTokenDTO{
		Token:     token.Token,
		Role:      token.Role,
		Revoked:   token.Revoked,
		ExpiresAt: expiresAt,
		CreatedAt: token.CreatedAt.UnixMilli(),
		UpdatedAt: token.UpdatedAt.UnixMilli(),
		ShareURL:  BuildBoardShareURL(baseURL, token.Token),
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

func requestScheme(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if c.Request.TLS != nil {
		return "https"
	}
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		return strings.ToLower(proto)
	}
	return "http"
}
