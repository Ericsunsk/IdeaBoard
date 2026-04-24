package boards

import (
	"encoding/json"
	"errors"
	"strings"

	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type BoardHandler struct {
	service *BoardService
}

type boardDTO struct {
	BoardID   string      `json:"board_id"`
	Name      string      `json:"name"`
	State     interface{} `json:"state"`
	CreatedAt int64       `json:"created_at"`
	UpdatedAt int64       `json:"updated_at"`
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
		result = append(result, toBoardDTO(board))
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
		response.InternalError(c, 500, "更新画布失败", err)
		return
	}
	response.Success(c, gin.H{"board": toBoardDTO(*board)})
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
		response.InternalError(c, 500, "删除画布失败", err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func toBoardDTO(board UserBoard) boardDTO {
	return boardDTO{
		BoardID:   board.BoardID,
		Name:      board.Name,
		State:     parseBoardState(board.State),
		CreatedAt: board.CreatedAt.UnixMilli(),
		UpdatedAt: board.UpdatedAt.UnixMilli(),
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
