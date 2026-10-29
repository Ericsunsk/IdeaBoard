// Package http provides shared HTTP dependency guards.
package http

import (
	stdhttp "net/http"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// RequireDependency renders 503 in the numeric envelope when a dependency is missing.
func RequireDependency(c *gin.Context, present bool, msg string) bool {
	if present {
		return true
	}
	response.InternalError(c, stdhttp.StatusServiceUnavailable, msg, nil)
	return false
}

// RequireDependencyCode renders 503 in the machine envelope when a dependency is missing.
func RequireDependencyCode(c *gin.Context, present bool, code, msg string) bool {
	if present {
		return true
	}
	errorrender.Render(c, stdhttp.StatusServiceUnavailable, code, msg)
	return false
}
