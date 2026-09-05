package http

import (
	"errors"
	"fmt"

	"ideaboard/api/internal/accountpool/accounts/application"
	"ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/validation"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	accountService *application.Service
}

type accountLimitsResponse struct {
	Limits domain.AccountLimits `json:"limits"`
}

type accountListResponse struct {
	Total int64                     `json:"total"`
	List  []*domain.AccountWithLoad `json:"list"`
}

type accountResponse struct {
	Account *domain.ApiAccount `json:"account"`
}

type accountDetailResponse struct {
	Account           *domain.ApiAccount `json:"account"`
	CurrentConcurrent int                `json:"current_concurrent"`
	MaxConcurrent     int                `json:"max_concurrent"`
}

type accountQueueResponse struct {
	CurrentConcurrent int `json:"current_concurrent"`
	MaxConcurrent     int `json:"max_concurrent"`
}

func NewHandler(accountService *application.Service) *Handler {
	return &Handler{accountService: accountService}
}

// ============ 账号池 API ============

func (h *Handler) GetAccountLimits(c *gin.Context) {
	response.SuccessDTO(c, accountLimitsResponse{Limits: domain.CurrentAccountLimits()})
}

func (h *Handler) ListAccounts(c *gin.Context) {
	var req domain.AccountListRequest
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

func (h *Handler) GetAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	ctx := c.Request.Context()
	account, err := h.accountService.GetAccount(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "获取账号详情失败", err)
		return
	}

	concurrent, maxConcurrent, err := h.accountService.GetQueueInfo(ctx, id)
	if err != nil {
		response.InternalError(c, 500, "获取账号队列信息失败", err)
		return
	}

	response.SuccessDTO(c, accountDetailResponse{
		Account:           account,
		CurrentConcurrent: concurrent,
		MaxConcurrent:     maxConcurrent,
	})
}

func (h *Handler) CreateAccount(c *gin.Context) {
	var req domain.CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	account, err := h.accountService.CreateAccount(ctx, &req)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNameConflict) {
			response.Error(c, 409, "同渠道下账号名称已存在")
			return
		}
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "创建账号失败: "+err.Error(), err)
		return
	}

	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *Handler) UpdateAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	var req domain.UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	if err := h.accountService.UpdateAccount(ctx, id, &req); err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		if errors.Is(err, domain.ErrAccountNameConflict) {
			response.Error(c, 409, "同渠道下账号名称已存在")
			return
		}
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "更新账号失败", err)
		return
	}

	account, err := h.accountService.GetAccount(ctx, id)
	if err != nil {
		response.InternalError(c, 500, "获取更新后的账号失败", err)
		return
	}

	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *Handler) PatchAccountModel(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	var req domain.PatchAccountModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	ctx := c.Request.Context()
	account, err := h.accountService.PatchAccountModel(ctx, id, &req)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "更新模型配置失败", err)
		return
	}

	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *Handler) DeleteAccountModel(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}
	modelName := c.Query("name")

	ctx := c.Request.Context()
	account, err := h.accountService.DeleteAccountModel(ctx, id, modelName)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		if errors.Is(err, domain.ErrAccountModelNotFound) {
			response.Error(c, 404, "模型不存在")
			return
		}
		if validation.Is(err) {
			response.Error(c, 400, err.Error())
			return
		}
		response.InternalError(c, 500, "删除模型失败", err)
		return
	}

	response.SuccessDTO(c, accountResponse{Account: account})
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	ctx := c.Request.Context()
	if err := h.accountService.DeleteAccount(ctx, id); err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "删除账号失败", err)
		return
	}

	response.Success(c, "账号删除成功")
}

func (h *Handler) StartAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	ctx := c.Request.Context()
	if err := h.accountService.StartAccount(ctx, id); err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "启动账号失败", err)
		return
	}

	response.Success(c, "账号已启动")
}

func (h *Handler) StopAccount(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	ctx := c.Request.Context()
	if err := h.accountService.StopAccount(ctx, id); err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			response.Error(c, 404, "账号不存在")
			return
		}
		response.InternalError(c, 500, "停止账号失败", err)
		return
	}

	response.Success(c, "账号已停止")
}

func (h *Handler) GetAccountQueue(c *gin.Context) {
	idStr := c.Param("id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id == 0 {
		response.Error(c, 400, "无效的账号ID")
		return
	}

	ctx := c.Request.Context()
	concurrent, maxConcurrent, err := h.accountService.GetQueueInfo(ctx, id)
	if err != nil {
		response.InternalError(c, 500, "获取账号队列信息失败", err)
		return
	}

	response.SuccessDTO(c, accountQueueResponse{
		CurrentConcurrent: concurrent,
		MaxConcurrent:     maxConcurrent,
	})
}
