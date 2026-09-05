package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func TestCallFluxFillOutpaintUsesPromptSeedAndGeometryContract(t *testing.T) {
	const inputURL = "https://media.ideaboard.art/uploads/canvas/demo/source.png"
	var payload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fill/outpaint" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("outpainted"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callFluxFill(context.Background(), &accounts.ApiAccount{
		ProxyURL: server.URL + "/fill",
		ApiKey:   "secret",
	}, providerRequest{
		Prompt:         "legacy prompt must not leak",
		SystemPrompt:   "保持原有摄影风格",
		UserPrompt:     "向左补全海岸线",
		Seed:           42,
		ImageRefs:      []string{inputURL},
		OriginalWidth:  640,
		OriginalHeight: 480,
		Outpaint: &tasks.OutpaintParams{
			TargetWidth:  960,
			TargetHeight: 720,
			ImageOffsetX: 160,
			ImageOffsetY: 48,
		},
	}, workflow.RuntimeOperationOutpaint, time.Second)
	if err != nil {
		t.Fatalf("callFluxFill: %v", err)
	}

	want := map[string]interface{}{
		"image_url":      inputURL,
		"system_prompt":  "保持原有摄影风格",
		"prompt":         "向左补全海岸线",
		"seed":           float64(42),
		"custom_width":   float64(960),
		"custom_height":  float64(720),
		"image_offset_x": float64(160),
		"image_offset_y": float64(48),
	}
	if len(payload) != len(want) {
		t.Fatalf("payload = %#v, want exactly %#v", payload, want)
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("payload[%q] = %#v, want %#v", key, payload[key], value)
		}
	}
}

func TestCallFluxFillInpaintUsesMaskURLAndConfiguredSystemPrompt(t *testing.T) {
	const inputURL = "https://media.ideaboard.art/uploads/canvas/demo/source.png"
	const maskURL = "https://media.ideaboard.art/uploads/canvas/demo/mask.png"
	var payload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/inpaint" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("inpainted"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callFluxFill(context.Background(), &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "secret",
	}, providerRequest{
		SystemPrompt: "仅编辑蒙版区域",
		UserPrompt:   "  移除路人  ",
		Seed:         7,
		ImageRefs:    []string{inputURL},
		MaskRefs:     []string{maskURL},
	}, workflow.RuntimeOperationInpaint, time.Second)
	if err != nil {
		t.Fatalf("callFluxFill: %v", err)
	}

	if payload["image_url"] != inputURL || payload["mask_url"] != maskURL {
		t.Fatalf("media payload = %#v", payload)
	}
	if payload["system_prompt"] != "仅编辑蒙版区域" {
		t.Fatalf("system_prompt = %#v", payload["system_prompt"])
	}
	if payload["prompt"] != "移除路人" || payload["seed"] != float64(7) {
		t.Fatalf("generation payload = %#v", payload)
	}
}

func TestCallFluxFillHTTPErrorRetryClassification(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "bad request", status: http.StatusBadRequest},
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "payment required", status: http.StatusPaymentRequired},
		{name: "forbidden", status: http.StatusForbidden},
		{name: "not found", status: http.StatusNotFound},
		{name: "conflict", status: http.StatusConflict},
		{name: "unprocessable", status: http.StatusUnprocessableEntity},
		{name: "request timeout", status: http.StatusRequestTimeout, retryable: true},
		{name: "too many requests", status: http.StatusTooManyRequests, retryable: true},
		{name: "internal server error", status: http.StatusInternalServerError, retryable: true},
		{name: "service unavailable", status: http.StatusServiceUnavailable, retryable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"error":"provider failure"}`))
			}))
			defer server.Close()

			executor := &ProviderExecutor{}
			_, err := executor.callFluxFill(context.Background(), &accounts.ApiAccount{
				ProxyURL: server.URL,
			}, providerRequest{
				SystemPrompt: "仅编辑蒙版区域",
				ImageRefs:    []string{"https://media.ideaboard.art/source.png"},
				MaskRefs:     []string{"https://media.ideaboard.art/mask.png"},
			}, workflow.RuntimeOperationInpaint, time.Second)
			if err == nil {
				t.Fatal("expected provider error")
			}
			if got := shouldFailoverProviderError(err); got != test.retryable {
				t.Fatalf("shouldFailoverProviderError() = %v, want %v; err=%v", got, test.retryable, err)
			}
		})
	}
}

func TestCallFluxFillNetworkErrorIsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callFluxFill(context.Background(), &accounts.ApiAccount{
		ProxyURL: endpoint,
	}, providerRequest{
		SystemPrompt: "仅编辑蒙版区域",
		ImageRefs:    []string{"https://media.ideaboard.art/source.png"},
		MaskRefs:     []string{"https://media.ideaboard.art/mask.png"},
	}, workflow.RuntimeOperationInpaint, time.Second)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !shouldFailoverProviderError(err) {
		t.Fatalf("network error must be retryable: %v", err)
	}
}

func TestFluxFillTimedOutInvocationIsNotRetried(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("inpainted"))
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	orchestrator := newProviderBatchOrchestrator(func(ctx context.Context, request providerRequest) (providerCallResult, error) {
		return executor.callFluxFill(ctx, &accounts.ApiAccount{
			ProxyURL: server.URL,
		}, request, workflow.RuntimeOperationInpaint, 10*time.Millisecond)
	})
	orchestrator.wait = func(context.Context, time.Duration) error { return nil }

	_, err := orchestrator.run(context.Background(), providerRequest{
		SystemPrompt: "仅编辑蒙版区域",
		ImageRefs:    []string{"https://media.ideaboard.art/source.png"},
		MaskRefs:     []string{"https://media.ideaboard.art/mask.png"},
		OutputCount:  1,
	})
	if err == nil {
		t.Fatal("expected provider timeout")
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("provider requests = %d, want 1 for an invocation with unknown outcome", got)
	}
	if shouldFailoverProviderError(err) {
		t.Fatalf("timed out Flux Fill invocation must not be retried: %v", err)
	}
}

func TestFluxFillFollowsModalResultRedirect(t *testing.T) {
	var server *httptest.Server
	var requestCount atomic.Int32
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Query().Get("modal-result") == "ready" {
			if r.Method != http.MethodGet {
				t.Fatalf("result request method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("inpainted"))
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("initial request method = %s, want POST", r.Method)
		}
		http.Redirect(w, r, server.URL+"/inpaint?modal-result=ready", http.StatusSeeOther)
	}))
	defer server.Close()

	executor := &ProviderExecutor{}
	_, err := executor.callFluxFill(context.Background(), &accounts.ApiAccount{
		ProxyURL: server.URL,
	}, providerRequest{
		SystemPrompt: "仅编辑蒙版区域",
		ImageRefs:    []string{"https://media.ideaboard.art/source.png"},
		MaskRefs:     []string{"https://media.ideaboard.art/mask.png"},
	}, workflow.RuntimeOperationInpaint, time.Second)
	if err != nil {
		t.Fatalf("callFluxFill: %v", err)
	}
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("Modal redirect requests = %d, want 2", got)
	}
}
