package http

import (
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/jsonlimit"

	"github.com/gin-gonic/gin"
)

const maxAdminCreditRequestBytes = 16 << 10

func requireAdmin(c *gin.Context, serviceConfigured bool) (int64, bool) {
	identityPrincipal, ok := principal.RequireAdmin(c)
	if !ok {
		return 0, false
	}
	if !serviceConfigured {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable)
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func decodeAdminCreditRequest(c *gin.Context, destination any) error {
	return jsonlimit.DecodeStrictRequest(c.Writer, c.Request, maxAdminCreditRequestBytes, destination)
}

func parsePositiveInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && parsed > 0
}
