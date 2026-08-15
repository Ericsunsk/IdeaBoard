package worker

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/platform/config"
)

func TestCallBackgroundRemovalUsesJSONContractAndKeepsBinaryResult(t *testing.T) {
	const inputURL = "https://media.ideaboard.art/uploads/canvas/demo/source.jpg"
	responseBytes := []byte("fake-png")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/matting" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "secret" {
			t.Fatalf("X-Api-Key = %q", got)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(responseBytes)
	}))
	defer server.Close()

	executor := &ProviderExecutor{publicOrigins: config.PublicOriginsConfig{API: "https://media.ideaboard.art"}}
	result, err := executor.callBackgroundRemoval(context.Background(), &accounts.ApiAccount{
		ApiKey:   "secret",
		Protocol: providerendpoints.ProtocolImageURLJSONPNG,
		ProxyURL: server.URL + "/matting",
	}, providerRequest{ImageRefs: []string{inputURL}}, time.Second)
	if err != nil {
		t.Fatalf("callBackgroundRemoval: %v", err)
	}
	if !strings.HasPrefix(result.ImageURL, "data:image/png;base64,") {
		t.Fatalf("result URL = %q", result.ImageURL)
	}
	encoded := strings.TrimPrefix(result.ImageURL, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != string(responseBytes) {
		t.Fatalf("decoded result = %q, err=%v", decoded, err)
	}
}

func TestBackgroundRemovalInputURLResolvesUploadReference(t *testing.T) {
	executor := &ProviderExecutor{publicOrigins: config.PublicOriginsConfig{API: "https://media.ideaboard.art"}}
	got, err := executor.backgroundRemovalInputURL(providerRequest{ImageRefs: []string{"/uploads/canvas/demo/source.jpg"}})
	if err != nil {
		t.Fatalf("backgroundRemovalInputURL: %v", err)
	}
	if got != "https://media.ideaboard.art/uploads/canvas/demo/source.jpg" {
		t.Fatalf("resolved URL = %q", got)
	}
}
