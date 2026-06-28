package httpapi

import (
	iofs "io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterFrontendRoutes(r *gin.Engine, frontendFS iofs.FS, opts FrontendRouteOptions) {
	if frontendFS == nil {
		return
	}

	if _, err := iofs.Stat(frontendFS, "index.html"); err != nil {
		if opts.WarnMissingIndex {
			slog.Warn("embedded frontend index.html not found; skip SPA routes", "err", err)
		}
		return
	}

	serveIndex := func(c *gin.Context) {
		c.FileFromFS("/index.html", http.FS(frontendFS))
	}

	r.GET("/", serveIndex)

	r.NoRoute(func(c *gin.Context) {
		requestPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
		if requestPath == "." {
			requestPath = ""
		}

		if requestPath == "api" || strings.HasPrefix(requestPath, "api/") || requestPath == "uploads" || strings.HasPrefix(requestPath, "uploads/") {
			c.Status(http.StatusNotFound)
			return
		}

		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		if requestPath != "" {
			if _, err := iofs.Stat(frontendFS, requestPath); err == nil {
				c.FileFromFS("/"+requestPath, http.FS(frontendFS))
				return
			}
		}

		serveIndex(c)
	})
}
