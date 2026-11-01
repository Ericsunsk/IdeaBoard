package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"ideaboard/api/internal/platform/safehttp"
)

// Parity pin: workerDryRunBody/workerDryRunRequest must stay key-identical to
// apps/ai-proxy/src/contracts.ts CanonicalImageRequest (task_id, provider,
// model, prompt, negative_prompt?, width?, height?, steps?, guidance_scale?,
// seed?, images?, provider_options?, upload_url?, public_url?).
//
// The constructor is exercised by marshaling only (no network). If the TS
// side adds or renames a field, extend workerDryRunRequest in model.go and
// the expectation below together — a red test here means the Go dry-run
// body drifted from the TS contract.
func TestWorkerDryRunBodyMatchesContractsShape(t *testing.T) {
	full := workerDryRunRequest{
		TaskID:          "probe",
		Provider:        "openai",
		Model:           "gpt-image-1",
		Prompt:          "probe",
		NegativePrompt:  strPtr("no blur"),
		Width:           intPtr(1024),
		Height:          intPtr(1024),
		Steps:           intPtr(20),
		GuidanceScale:   floatPtr(3.5),
		Seed:            uintPtr(7),
		Images:          []string{"https://example.com/in.png"},
		ProviderOptions: map[string]any{"quality": "high"},
		UploadURL:       strPtr("https://example.com/up"),
		PublicURL:       strPtr("https://example.com/pub"),
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal full dry-run request: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal dry-run request: %v", err)
	}
	var got []string
	for k := range decoded {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{
		"guidance_scale", "height", "images", "model", "negative_prompt",
		"prompt", "provider", "provider_options", "public_url", "seed",
		"steps", "task_id", "upload_url", "width",
	}
	if len(got) != len(want) {
		t.Fatalf("dry-run keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dry-run keys = %v, want %v", got, want)
		}
	}
}

// The shared constructor omits every unset optional (omitempty), so the
// minimal probe body carries exactly the four required TS keys.
func TestWorkerDryRunBodyOmitsEmptyOptionals(t *testing.T) {
	raw, err := newWorkerDryRunBody("openai", "gpt-image-1", "")
	if err != nil {
		t.Fatalf("newWorkerDryRunBody: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal dry-run body: %v", err)
	}
	var got []string
	for k := range decoded {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"model", "prompt", "provider", "task_id"}
	if len(got) != len(want) {
		t.Fatalf("minimal dry-run keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("minimal dry-run keys = %v, want %v", got, want)
		}
	}
}

// Preview and Probe share newWorkerDryRunBody: the same customParams must
// produce byte-comparable provider_options on both paths.
func TestPreviewAndProbeShareDryRunBody(t *testing.T) {
	restore := safehttp.ConfigurePolicyProvider(stubOutboundPolicy{hosts: []string{"127.0.0.1"}})
	t.Cleanup(restore)

	var bodies []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode dry-run body: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"provider":"openai","model":"gpt-image-1","target":{"url":"https://up.example/v1","method":"POST","body":{}},"path_source":"openai:images:generations"}`))
	}))
	defer upstream.Close()

	probe := probeWorkerConnection(t.Context(), upstream.URL, "secret", "openai/gpt-image-1", `{"quality":"high"}`)
	if !probe.Reachable {
		t.Fatalf("probe = %+v, want reachable", probe)
	}
	preview := PreviewWorkerRoute(t.Context(), upstream.URL, "secret", "openai/gpt-image-1", `{"quality":"high"}`)
	if !preview.Reachable {
		t.Fatalf("preview = %+v, want reachable", preview)
	}
	if len(bodies) != 2 {
		t.Fatalf("captured bodies = %d, want 2", len(bodies))
	}
	for i, body := range bodies {
		opts, ok := body["provider_options"].(map[string]any)
		if !ok || opts["quality"] != "high" {
			t.Fatalf("body[%d] provider_options = %v, want quality:high", i, body["provider_options"])
		}
	}
	probeKeys := sortedKeys(bodies[0])
	previewKeys := sortedKeys(bodies[1])
	if len(probeKeys) != len(previewKeys) {
		t.Fatalf("probe keys = %v, preview keys = %v, want identical", probeKeys, previewKeys)
	}
	for i := range probeKeys {
		if probeKeys[i] != previewKeys[i] {
			t.Fatalf("probe keys = %v, preview keys = %v, want identical", probeKeys, previewKeys)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func strPtr(s string) *string     { return &s }
func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }
func uintPtr(u uint64) *uint64    { return &u }
