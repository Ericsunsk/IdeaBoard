package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/workflow"
)

func TestCallCutoutUsesJSONContractAndKeepsBinaryResult(t *testing.T) {
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
	result, err := executor.callCutout(context.Background(), &accounts.ApiAccount{
		ApiKey:       "secret",
		Protocol:     providerendpoints.ProtocolImagePipeline,
		Capabilities: []string{providerendpoints.CapabilityCutout},
		ProxyURL:     server.URL + "/matting",
	}, providerRequest{ImageRefs: []string{inputURL}}, time.Second)
	if err != nil {
		t.Fatalf("callCutout: %v", err)
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

func TestCutoutInputURLResolvesUploadReference(t *testing.T) {
	executor := &ProviderExecutor{publicOrigins: config.PublicOriginsConfig{API: "https://media.ideaboard.art"}}
	got, err := executor.imageTransformInputURL(providerRequest{ImageRefs: []string{"/uploads/canvas/demo/source.jpg"}}, "抠图")
	if err != nil {
		t.Fatalf("imageTransformInputURL: %v", err)
	}
	if got != "https://media.ideaboard.art/uploads/canvas/demo/source.jpg" {
		t.Fatalf("resolved URL = %q", got)
	}
}

func TestCutoutInputURLRejectsHTTP(t *testing.T) {
	executor := &ProviderExecutor{}
	if _, err := executor.imageTransformInputURL(providerRequest{ImageRefs: []string{"http://media.ideaboard.art/source.jpg"}}, "抠图"); err == nil {
		t.Fatal("expected HTTP input URL to be rejected")
	}
}

func TestCallCutoutRejectsNonImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"not an image"}`))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callCutout(context.Background(), &accounts.ApiAccount{
		ApiKey: "secret", ProxyURL: server.URL,
	}, providerRequest{ImageRefs: []string{"https://media.ideaboard.art/source.jpg"}}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "Content-Type") {
		t.Fatalf("error=%v, want invalid Content-Type", err)
	}
}

func TestCallUpscaleUsesScaleContractAndSingleImage(t *testing.T) {
	const inputURL = "https://media.ideaboard.art/uploads/canvas/demo/source.jpg"
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("upscaled"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	result, err := executor.callUpscale(context.Background(), &accounts.ApiAccount{
		ApiKey:       "secret",
		Protocol:     providerendpoints.ProtocolImagePipeline,
		Capabilities: []string{providerendpoints.CapabilityUpscale},
		ProxyURL:     server.URL,
	}, providerRequest{
		ImageRefs:        []string{inputURL, "https://media.ideaboard.art/uploads/canvas/demo/ignored.jpg"},
		Prompt:           "must not be sent",
		OutputCount:      4,
		OutputResolution: "4k",
		Scale:            2.3,
	}, time.Second)
	if err != nil {
		t.Fatalf("callUpscale: %v", err)
	}
	if got["image_url"] != inputURL || got["scale"] != 2.3 || got["output_format"] != "png" {
		t.Fatalf("payload = %#v, want image_url, scale and output_format", got)
	}
	if len(got) != 3 {
		t.Fatalf("payload contains ignored controls: %#v", got)
	}
	if !strings.HasPrefix(result.ImageURL, "data:image/png;base64,") {
		t.Fatalf("result URL = %q", result.ImageURL)
	}
}

func TestCallUpscaleRejectsInvalidScale(t *testing.T) {
	executor := &ProviderExecutor{}
	for _, scale := range []float64{0.9, 4.1} {
		_, err := executor.callUpscale(context.Background(), &accounts.ApiAccount{
			ApiKey:       "secret",
			Protocol:     providerendpoints.ProtocolImagePipeline,
			Capabilities: []string{providerendpoints.CapabilityUpscale},
			ProxyURL:     "https://upscale.example/modal",
		}, providerRequest{ImageRefs: []string{"https://media.ideaboard.art/source.jpg"}, Scale: scale}, time.Second)
		if err == nil || !strings.Contains(err.Error(), "scale") {
			t.Fatalf("scale=%v error=%v, want scale validation", scale, err)
		}
	}
}

func TestCutoutBatchIgnoresRequestedOutputCount(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callProviderBatchWithRequest(context.Background(), resolvedProviderExecution{
		providerEndpointBaseURL: server.URL,
		apiType:                 workflow.ProviderOpenAICompatible,
		protocol:                providerendpoints.ProtocolImagePipeline,
		apiKey:                  "secret",
		runtimeOperation:        workflow.RuntimeOperationCutout,
		adapter:                 providerRuntimeAdapterCutout,
		timeout:                 time.Second,
		request:                 providerRequest{ImageRefs: []string{"https://media.ideaboard.art/source.jpg"}, OutputCount: 4},
	})
	if err != nil {
		t.Fatalf("callProviderBatchWithRequest: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}
