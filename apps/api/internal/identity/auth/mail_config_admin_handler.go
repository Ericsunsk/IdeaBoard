package auth

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// MailConfigAdminHandler is the API-side adapter for the Auth-owned mail
// configuration. It keeps Admin authentication and actor attribution in Go,
// while the Secret and provider lifecycle remain inside Auth.
type MailConfigAdminHandler struct {
	service *ServiceClient
}

func NewMailConfigAdminHandler(service *ServiceClient) *MailConfigAdminHandler {
	return &MailConfigAdminHandler{service: service}
}

func (h *MailConfigAdminHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusInternalServerError, "邮件配置服务未初始化", errors.New("mail config service is not initialized"))
		return
	}
	data, err := h.service.GetMailConfig(c.Request.Context())
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, data)
}

func (h *MailConfigAdminHandler) Update(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusInternalServerError, "邮件配置服务未初始化", errors.New("mail config service is not initialized"))
		return
	}
	var req struct {
		APIKey    string `json:"api_key"`
		FromName  string `json:"from_name"`
		FromEmail string `json:"from_email"`
		ReplyTo   string `json:"reply_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	data, err := h.service.UpdateMailConfig(c.Request.Context(), MailConfigUpdate{
		APIKey: req.APIKey, FromName: req.FromName, FromEmail: req.FromEmail, ReplyTo: req.ReplyTo,
	}, admin.UserID())
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, data)
}

func (h *MailConfigAdminHandler) Clear(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusInternalServerError, "邮件配置服务未初始化", errors.New("mail config service is not initialized"))
		return
	}
	if err := h.service.ClearMailConfig(c.Request.Context(), admin.UserID()); err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"deleted": true})
}

func (h *MailConfigAdminHandler) Test(c *gin.Context) {
	admin, ok := principal.RequireAdmin(c)
	if !ok {
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusInternalServerError, "邮件配置服务未初始化", errors.New("mail config service is not initialized"))
		return
	}
	var req struct {
		Recipient string `json:"recipient" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Recipient) == "" {
		response.Error(c, http.StatusBadRequest, "请输入测试收件地址")
		return
	}
	if err := h.service.SendMailConfigTest(c.Request.Context(), req.Recipient, admin.UserID()); err != nil {
		h.respondError(c, err)
		return
	}
	response.SuccessDTO(c, gin.H{"sent": true})
}

func (h *MailConfigAdminHandler) respondError(c *gin.Context, err error) {
	var configErr *MailConfigError
	if errors.As(err, &configErr) {
		switch configErr.Status {
		case http.StatusBadRequest:
			response.Error(c, http.StatusBadRequest, configErr.Message)
		case http.StatusConflict:
			response.Error(c, http.StatusConflict, configErr.Message)
		default:
			response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
		}
		return
	}
	response.InternalError(c, http.StatusBadGateway, "认证服务暂不可用", err)
}
