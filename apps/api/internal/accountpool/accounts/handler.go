package accounts

import (
	"errors"
	"fmt"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	accountService *AccountService
}

type accountLimitsResponse struct {
	Limits AccountLimits `json:"limits"`
}

type accountListResponse struct {
	Total int64              `json:"total"`
	List  []*AccountWithLoad `json:"list"`
}

type accountResponse struct {
	Account *ApiAccount `json:"account"`
}

type accountDetailResponse struct {
	Account           *ApiAccount `json:"account"`
	CurrentConcurrent int         `json:"current_concurrent"`
	MaxConcurrent     int         `json:"max_concurrent"`
}

type accountQueueResponse struct {
	CurrentConcurrent int `json:"current_concurrent"`
	MaxConcurrent     int `json:"max_concurrent"`
}

func NewAccountHandler(accountService *AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// ============ 账号池 API ============

func (h *AccountHandler) GetAccountLimits(c *gin.Context) {
	response.SuccessDTO(c, accountLimitsResponse{Limits: CurrentAccountLimits()})
}

func (h *AccountHandler) ListAccounts(c *gin.Context) {
	var req AccountListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	ctx := c.Request.Context()
	accounts, total, err := h.accountService.ListAccounts(ctx, &req)
	if err != nil {
		response.InternalError(c, 500, "获取账号列表失败", err)
		return
	}
	response.SuccessDTO(c, accountListResponse{Total: total, List: accounts})
}

func (h *AccountHandler) GetAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	ctx := c.Request.Context()
	account, err := h.accountService.GetAccount(ctx, id)
	if err != nil {
		response.Error(c, 404, "账号不存在")
		return
	}
	concurrent, maxConcurrent, err := h.accountService.GetAccountQueueInfo(ctx, id)
	if err != nil {
		response.InternalError(c, 500, "获取账号队列信息失败", err)
		return
	}
	response.SuccessDTO(c, accountDetailResponse{Account: account, CurrentConcurrent: concurrent, MaxConcurrent: maxConcurrent})
}

func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	account, err := h.accountService.CreateAccount(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrAccountNameConflict) {
			response.Error(c, 409, "同一 ProviderEndpoint 下已存在同名 APIKey")
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	id, ok := parseAccountIDParam(c)
	if !ok {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.accountService.UpdateAccount(c.Request.Context(), id, &req); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		if errors.Is(err, ErrAccountNameConflict) {
			response.Error(c, 409, "同一 ProviderEndpoint 下已存在同名 APIKey")
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) PatchAccountModel(c *gin.Context) {
	id, ok := parseAccountIDParam(c)
	if !ok {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	var req PatchAccountModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	account, err := h.accountService.PatchAccountModel(c.Request.Context(), id, &req)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *AccountHandler) DeleteAccountModel(c *gin.Context) {
	id, ok := parseAccountIDParam(c)
	if !ok {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	modelName := c.Query("model_name")
	account, err := h.accountService.DeleteAccountModel(c.Request.Context(), id, modelName)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		if errors.Is(err, ErrAccountModelNotFound) {
			response.Error(c, 404, "模型不存在")
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	id, ok := parseAccountIDParam(c)
	if !ok {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.DeleteAccount(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "删除失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StartAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StartAccount(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "启动失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) StopAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	if err := h.accountService.StopAccount(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "停止失败", err)
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) GetAccountQueue(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	concurrent, maxConcurrent, err := h.accountService.GetAccountQueueInfo(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, 500, "获取失败", err)
		return
	}
	response.SuccessDTO(c, accountQueueResponse{CurrentConcurrent: concurrent, MaxConcurrent: maxConcurrent})
}

func parseAccountIDParam(c *gin.Context) (int64, bool) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}
