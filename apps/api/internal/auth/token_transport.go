package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const authTokenTTL = 24 * time.Hour

var errInvalidBearerToken = errors.New("invalid bearer token")

func extractTokenFromRequest(c *gin.Context) (string, error) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return "", errInvalidBearerToken
		}
		token := strings.TrimSpace(parts[1])
		if token == "" {
			return "", errInvalidBearerToken
		}
		return token, nil
	}

	token, err := c.Cookie(AuthTokenCookieName)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(token), nil
}

func setAuthTokenCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AuthTokenCookieName, token, int(authTokenTTL.Seconds()), "/", "", requestIsHTTPS(c), true)
}

func clearAuthTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AuthTokenCookieName, "", -1, "/", "", requestIsHTTPS(c), true)
}

func requestIsHTTPS(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}
