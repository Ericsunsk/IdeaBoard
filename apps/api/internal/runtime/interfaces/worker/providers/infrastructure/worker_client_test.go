package providerinfra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	runtimeports "ideaboard/api/internal/runtime/ports"
)

type workerTestOutboundPolicyProvider struct{ hosts []string }

func (p workerTestOutboundPolicyProvider) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func TestMain(m *testing.M) {
	safehttp.ConfigurePolicyProvider(workerTestOutboundPolicyProvider{hosts: []string{"localhost", "127.0.0.1", "::1"}})
	os.Exit(m.Run())
}

func TestNewWorkerClientRejectsBadConfig(t *testing.T) {
	for name, cfg := range map[string]WorkerClientConfig{
		"missing base":   {Secret: "s"},
		"missing secret": {BaseURL: "https://worker.example"},
		"bad scheme":     {BaseURL: "ftp://worker.example", Secret: "s"},
		"no host":        {BaseURL: "https://", Secret: "s"},
	} {
		if _, err := NewWorkerClient(cfg); err == nil {
			t.Fatalf("%s: NewWorkerClient succeeded, want error", name)
		}
	}
	if _, err := NewWorkerClient(WorkerClientConfig{BaseURL: "https://worker.example", Secret: "s"}); err != nil {
		t.Fatalf("NewWorkerClient() error = %v, want nil", err)
	}
}

func TestNewWorkerClientFailsClosedWithoutConfig(t *testing.T) {
	if _, err := NewWorkerClient(WorkerClientConfig{BaseURL: "", Secret: "s"}); err == nil ||
		!strings.Contains(err.Error(), "WORKER_BASE_URL") {
		t.Fatalf("missing base err = %v, want WORKER_BASE_URL fail-closed", err)
	}
	if _, err := NewWorkerClient(WorkerClientConfig{BaseURL: "https://w.example", Secret: ""}); err == nil ||
		!strings.Contains(err.Error(), "INTERNAL_SECRET") {
		t.Fatalf("missing secret err = %v, want INTERNAL_SECRET fail-closed", err)
	}
}

type workerCapture struct {
	path   string
	auth   string
	body   string
	method string
}

func newWorkerCaptureServer(t *testing.T, capture *workerCapture, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.path = r.URL.Path
		capture.method = r.Method
		capture.auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		capture.body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func workerTestRequest() runtimeports.WorkerRequest {
	return runtimeports.WorkerRequest{
		TaskID:    "task-1",
		Provider:  "openai",
		Model:     "gpt-image-1",
		Prompt:    "a red square",
		Width:     1024,
		Height:    1024,
		Seed:      42,
		Images:    []string{"https://media.example/inputs/a.png?sig=x"},
		UploadURL: "https://media.example/w/uploads/ws/abc/task-1/result-0.png?ct=image%2Fpng&exp=1&sig=y",
		Feature:   "generation",
	}
}

func TestWorkerClientPostsCanonicalShape(t *testing.T) {
	var capture workerCapture
	server := newWorkerCaptureServer(t, &capture, http.StatusOK,
		`{"task_id":"task-1","status":"completed","provider":"openai","model":"gpt-image-1","image_url":"https://media.example/w/uploads/ws/abc/task-1/result-0.png","content_type":"image/png","receipt":{"upstream_status":200,"provider":"openai","model":"gpt-image-1"}}`)
	defer server.Close()

	client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s3cr3t"})
	if err != nil {
		t.Fatalf("NewWorkerClient() error = %v", err)
	}
	resp, err := client.Do(context.Background(), workerTestRequest())
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if capture.path != "/v1/generate" || capture.method != http.MethodPost {
		t.Fatalf("request = %s %q, want POST /v1/generate", capture.method, capture.path)
	}
	if capture.auth != "Bearer s3cr3t" {
		t.Fatalf("auth = %q, want Bearer secret", capture.auth)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal([]byte(capture.body), &wire); err != nil {
		t.Fatalf("decode wire body: %v", err)
	}
	for _, key := range []string{"task_id", "provider", "model", "prompt", "width", "height", "seed", "images", "upload_url"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("wire body missing %q: %s", key, capture.body)
		}
	}
	if wire["provider"] != "openai" || wire["model"] != "gpt-image-1" {
		t.Fatalf("provider/model = %v/%v, want split openai + gpt-image-1", wire["provider"], wire["model"])
	}
	for _, banned := range []string{"outputs", "custom_params", "operation", "dry_run", "execution_token"} {
		if _, ok := wire[banned]; ok {
			t.Fatalf("wire body carries legacy %q: %s", banned, capture.body)
		}
	}
	if resp.ImageURL != "https://media.example/w/uploads/ws/abc/task-1/result-0.png" {
		t.Fatalf("image_url = %q", resp.ImageURL)
	}
	if resp.Receipt.UpstreamStatus != 200 || resp.Receipt.Provider != "openai" {
		t.Fatalf("receipt = %+v", resp.Receipt)
	}
}

func TestWorkerClientSurfacesStructuredFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantCode   string
		wantUpstat int
	}{
		{"unauthorized", 401, `{"task_id":"t","status":"failed","error":{"code":"UNAUTHORIZED","message":"unauthorized"}}`, "UNAUTHORIZED", 0},
		{"bad request", 400, `{"task_id":"t","status":"failed","error":{"code":"BAD_REQUEST","message":"upload_url is required"}}`, "BAD_REQUEST", 0},
		{"unsupported", 422, `{"task_id":"t","status":"failed","error":{"code":"UNSUPPORTED_MODEL","message":"queue/async"}}`, "UNSUPPORTED_MODEL", 0},
		{"upstream", 502, `{"task_id":"t","status":"failed","error":{"code":"UPSTREAM_ERROR","message":"boom","upstream_status":500}}`, "UPSTREAM_ERROR", 500},
		{"input fetch", 400, `{"task_id":"t","status":"failed","error":{"code":"INPUT_FETCH_FAILED","message":"fetch failed"}}`, "INPUT_FETCH_FAILED", 0},
		{"upload", 502, `{"task_id":"t","status":"failed","error":{"code":"UPLOAD_FAILED","message":"put failed","upstream_status":403}}`, "UPLOAD_FAILED", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capture workerCapture
			server := newWorkerCaptureServer(t, &capture, tc.status, tc.body)
			defer server.Close()
			client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s"})
			if err != nil {
				t.Fatalf("NewWorkerClient() error = %v", err)
			}
			_, err = client.Do(context.Background(), workerTestRequest())
			if err == nil {
				t.Fatalf("status %d succeeded, want error", tc.status)
			}
			code, ok := runtimeports.WorkerErrorDetails(err)
			if !ok || code != tc.wantCode {
				t.Fatalf("code = %q/%v, want %q", code, ok, tc.wantCode)
			}
			var workerErr *runtimeports.WorkerError
			if !errors.As(err, &workerErr) || workerErr.UpstreamStatus != tc.wantUpstat {
				t.Fatalf("upstream = %+v, want %d", workerErr, tc.wantUpstat)
			}
			if got, ok := runtimeports.ProviderHTTPStatus(err); !ok || got != tc.status {
				t.Fatalf("ProviderHTTPStatus = %d/%v, want %d", got, ok, tc.status)
			}
		})
	}
}

func TestWorkerClientRejectsTaskMismatch(t *testing.T) {
	var capture workerCapture
	server := newWorkerCaptureServer(t, &capture, http.StatusOK,
		`{"task_id":"other","status":"completed","provider":"openai","model":"gpt-image-1","image_url":"https://media.example/x.png","content_type":"image/png","receipt":{"upstream_status":200,"provider":"openai","model":"gpt-image-1"}}`)
	defer server.Close()
	client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s"})
	if err != nil {
		t.Fatalf("NewWorkerClient() error = %v", err)
	}
	if _, err := client.Do(context.Background(), workerTestRequest()); err == nil {
		t.Fatal("task mismatch accepted, want error")
	}
}

func TestWorkerClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s", Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewWorkerClient() error = %v", err)
	}
	req := workerTestRequest()
	req.Timeout = 50 * time.Millisecond
	if _, err := client.Do(context.Background(), req); err == nil {
		t.Fatal("slow worker succeeded, want timeout")
	}
}

// fakeWorkerFuse is the WorkerFuse test double: scripted reject or admit.
// 熔断拒绝/恢复两用例经此 seam 覆盖，不调参真实实例。
type fakeWorkerFuse struct {
	reject   error
	acquired int
	released int
}

func (f *fakeWorkerFuse) checkAndAcquire(string) (func(time.Duration, bool), error) {
	if f.reject != nil {
		return nil, f.reject
	}
	f.acquired++
	return func(time.Duration, bool) { f.released++ }, nil
}

func TestWorkerClientFuseRejectsWithoutNetwork(t *testing.T) {
	var capture workerCapture
	server := newWorkerCaptureServer(t, &capture, http.StatusOK, `{}`)
	defer server.Close()
	fuse := &fakeWorkerFuse{reject: &workerResilienceReject{reason: "breaker open", model: "openai/gpt-image-1"}}
	client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s", Resilience: fuse})
	if err != nil {
		t.Fatalf("NewWorkerClient() error = %v", err)
	}
	_, err = client.Do(context.Background(), workerTestRequest())
	if err == nil {
		t.Fatal("fuse rejection succeeded, want 429")
	}
	if got, ok := runtimeports.ProviderHTTPStatus(err); !ok || got != http.StatusTooManyRequests {
		t.Fatalf("ProviderHTTPStatus = %d/%v, want 429", got, ok)
	}
	if capture.path != "" {
		t.Fatalf("request reached %q, want no network I/O on fuse reject", capture.path)
	}
	if fuse.acquired != 0 || fuse.released != 0 {
		t.Fatalf("fuse acquired=%d released=%d, want 0/0 on reject", fuse.acquired, fuse.released)
	}
}

func TestWorkerClientFuseAdmitsAndReleases(t *testing.T) {
	var capture workerCapture
	server := newWorkerCaptureServer(t, &capture, http.StatusOK,
		`{"task_id":"task-1","status":"completed","provider":"openai","model":"gpt-image-1","image_url":"https://media.example/w/uploads/ws/abc/task-1/result-0.png","content_type":"image/png","receipt":{"upstream_status":200,"provider":"openai","model":"gpt-image-1"}}`)
	defer server.Close()
	fuse := &fakeWorkerFuse{}
	client, err := NewWorkerClient(WorkerClientConfig{BaseURL: server.URL, Secret: "s", Resilience: fuse})
	if err != nil {
		t.Fatalf("NewWorkerClient() error = %v", err)
	}
	resp, err := client.Do(context.Background(), workerTestRequest())
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.ImageURL == "" {
		t.Fatal("admitted send returned no image URL")
	}
	if fuse.acquired != 1 || fuse.released != 1 {
		t.Fatalf("fuse acquired=%d released=%d, want 1/1 on admit", fuse.acquired, fuse.released)
	}
}

func TestWorkerWireExplicitMaskRole(t *testing.T) {
	req := workerTestRequest()
	req.Images = []string{"https://media.test/one", "https://media.test/two", "https://media.test/mask"}
	raw, err := json.Marshal(workerRequestToWire(req))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["mask_index"]; ok {
		t.Fatal("ordinary references acquired a mask")
	}
	index := 2
	req.MaskIndex = &index
	raw, err = json.Marshal(workerRequestToWire(req))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["mask_index"] != float64(2) || len(body["images"].([]interface{})) != 3 {
		t.Fatalf("wrong roles: %s", raw)
	}
	for _, index := range []int{-1, 3} {
		req.MaskIndex = &index
		if req.Validate() == nil {
			t.Fatalf("invalid mask index %d accepted", index)
		}
	}
}
