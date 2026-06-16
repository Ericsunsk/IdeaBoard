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

func NewAccountHandler(accountService *AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// ============ 账号池 API ============

func (h *AccountHandler) GetAccountLimits(c *gin.Context) {
	response.Success(c, gin.H{"limits": CurrentAccountLimits()})
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
	response.Success(c, gin.H{"total": total, "list": accounts})
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
	response.Success(c, gin.H{"account": account, "current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}

func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	account, err := h.accountService.CreateAccount(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, 500, "创建失败", err)
		return
	}
	response.Success(c, gin.H{"account": account})
}

func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
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
		response.Error(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
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
	response.Success(c, gin.H{"current_concurrent": concurrent, "max_concurrent": maxConcurrent})
}
