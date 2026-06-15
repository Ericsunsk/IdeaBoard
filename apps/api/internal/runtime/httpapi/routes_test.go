package httpapi

import (
	"bytes"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestRegisterFrontendRoutesSkipsMissingIndexQuietlyInDevelopment(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
	})

	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{}, FrontendRouteOptions{})

	if strings.Contains(logs.String(), "embedded frontend index.html not found") {
		t.Fatalf("development route registration should not warn when embedded index is missing: %s", logs.String())
	}
}

func TestRegisterFrontendRoutesWarnsWhenMissingIndexInProduction(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(oldLogger)
	})

	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{}, FrontendRouteOptions{WarnMissingIndex: true})

	if !strings.Contains(logs.String(), "embedded frontend index.html not found") {
		t.Fatalf("production route registration should warn when embedded index is missing: %s", logs.String())
	}
}

func TestRegisterFrontendRoutesAcceptsEmbeddedIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFrontendRoutes(gin.New(), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}, FrontendRouteOptions{WarnMissingIndex: true})
}

var _ fs.FS = fstest.MapFS{}
