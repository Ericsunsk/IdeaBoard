package bootstrap

import (
	"net"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/platform/config"

	"github.com/gin-gonic/gin"
)

func newHTTPServer(cfg *config.Config, handler *gin.Engine) *http.Server {
	return &http.Server{
		Addr:              listenerAddr(cfg.App.Host, cfg.App.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func listenerAddr(host, port string) string {
	if strings.TrimSpace(host) == "" {
		return "127.0.0.1:" + port
	}
	return net.JoinHostPort(host, port)
}
