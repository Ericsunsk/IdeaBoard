package http

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/response"
)

const maxAuthUserIDLength = 128

type LoginMetadataUpdater interface {
	UpdateLastLoginMetadata(ctx context.Context, authUserID, ip, country string) error
}

type InternalHandler struct {
	updater LoginMetadataUpdater
	key     string
}

func NewInternalHandler(updater LoginMetadataUpdater, key string) *InternalHandler {
	return &InternalHandler{updater: updater, key: strings.TrimSpace(key)}
}

func (h *InternalHandler) UpdateLoginMetadata(c *gin.Context) {
	if h == nil || h.updater == nil || !constantTimeHeaderEquals(c.GetHeader("X-Internal-Key"), h.key) {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req ports.LoginMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	req.AuthUserID, req.IP, req.Country = strings.TrimSpace(req.AuthUserID), strings.TrimSpace(req.IP), strings.ToUpper(strings.TrimSpace(req.Country))
	if req.AuthUserID == "" || len(req.AuthUserID) > maxAuthUserIDLength || (req.IP != "" && net.ParseIP(req.IP) == nil) || !validLoginCountry(req.Country) {
		response.Error(c, http.StatusBadRequest, "参数错误")
		return
	}
	if err := h.updater.UpdateLastLoginMetadata(c.Request.Context(), req.AuthUserID, req.IP, req.Country); err != nil {
		response.InternalError(c, http.StatusInternalServerError, "更新登录信息失败", err)
		return
	}
	response.SuccessDTO(c, gin.H{"updated": true})
}

func validLoginCountry(country string) bool {
	if country == "" || country == "XX" {
		return country == ""
	}
	return len(country) == 2 && country[0] >= 'A' && country[0] <= 'Z' && country[1] >= 'A' && country[1] <= 'Z'
}

func constantTimeHeaderEquals(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
