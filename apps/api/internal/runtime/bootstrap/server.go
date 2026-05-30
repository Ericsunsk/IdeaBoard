package bootstrap

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"api/internal/platform/config"

	"github.com/gin-gonic/gin"
)

func newHTTPServer(cfg *config.Config, handler *gin.Engine) *http.Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	return &http.Server{
		Addr:              listenerAddr(cfg.App.Host, port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func listenerAddr(host, port string) string {
	if strings.TrimSpace(host) == "" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}
