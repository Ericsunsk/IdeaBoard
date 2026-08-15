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
		w.Header().Set("Content-Type", "image/webp; charset=utf-8")
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
	if !strings.HasPrefix(result.ImageURL, "data:image/webp; charset=utf-8;base64,") {
		t.Fatalf("result URL = %q", result.ImageURL)
	}
	encoded := strings.TrimPrefix(result.ImageURL, "data:image/webp; charset=utf-8;base64,")
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

func TestBackgroundRemovalInputURLRejectsHTTP(t *testing.T) {
	executor := &ProviderExecutor{}
	if _, err := executor.backgroundRemovalInputURL(providerRequest{ImageRefs: []string{"http://media.ideaboard.art/source.jpg"}}); err == nil {
		t.Fatal("expected HTTP input URL to be rejected")
	}
}

func TestCallBackgroundRemovalRejectsNonImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"not an image"}`))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callBackgroundRemoval(context.Background(), &accounts.ApiAccount{
		ApiKey: "secret", ProxyURL: server.URL,
	}, providerRequest{ImageRefs: []string{"https://media.ideaboard.art/source.jpg"}}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "Content-Type") {
		t.Fatalf("error=%v, want invalid Content-Type", err)
	}
}

func TestBackgroundRemovalBatchIgnoresRequestedOutputCount(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callProviderBatchWithRequest(context.Background(), &accounts.ApiAccount{
		ApiKey:   "secret",
		Protocol: providerendpoints.ProtocolImageURLJSONPNG,
		ProxyURL: server.URL,
	}, providerExecutionConfig{
		Timeout: time.Second,
		Request: providerRequest{ImageRefs: []string{"https://media.ideaboard.art/source.jpg"}, OutputCount: 4},
	})
	if err != nil {
		t.Fatalf("callProviderBatchWithRequest: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}
