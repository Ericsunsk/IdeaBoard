package domain

// Routing observability projections (#852): Admin snapshot parsing and
// final-model derivation over attempt facts. Pure unit coverage (no DB):
// success, retry/fallback, final failure, stale running rows, in-flight
// tasks, and missing model data.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAdminRoutingSnapshotProjectsPinnedFacts(t *testing.T) {
	raw := []byte(`{
		"service": "render",
		"requested_resolution": "2k",
		"eligible_models": ["openrouter/model-a", "openrouter/model-b"],
		"selected_model": "openrouter/model-b",
		"retry_candidate": "openrouter/model-a",
		"selection_seed": "00000000000000000000000000000000",
		"pool_version": 7
	}`)
	view, ok := ParseAdminRoutingSnapshot(raw)
	if !ok || view == nil {
		t.Fatal("valid snapshot must parse")
	}
	if view.Service != "render" || view.RequestedResolution != "2k" {
		t.Fatalf("service/resolution = %q/%q, want render/2k", view.Service, view.RequestedResolution)
	}
	if len(view.EligibleModels) != 2 || view.SelectedModel != "openrouter/model-b" {
		t.Fatalf("eligible/selected = %+v, want 2 entries + model-b", view)
	}
	if view.RetryCandidate != "openrouter/model-a" || view.PoolVersion != 7 {
		t.Fatalf("retry/pool_version = %q/%d, want model-a/7", view.RetryCandidate, view.PoolVersion)
	}
}

func TestParseAdminRoutingSnapshotMissingAndMalformed(t *testing.T) {
	for name, raw := range map[string][]byte{
		"nil":       nil,
		"empty":     {},
		"blank":     []byte("  \n "),
		"malformed": []byte(`{"service": `),
		"array":     []byte(`[]`),
	} {
		if _, ok := ParseAdminRoutingSnapshot(raw); ok {
			t.Fatalf("%s snapshot must report ok=false (missing model data)", name)
		}
	}
}

func TestFinalResolvedModelSuccess(t *testing.T) {
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "completed", RetryLayer: "gateway", ResolvedModel: "openrouter/model-a"},
	}
	if got := FinalResolvedModel(attempts); got != "openrouter/model-a" {
		t.Fatalf("final = %q, want openrouter/model-a", got)
	}
}

func TestFinalResolvedModelPrefersFallbackOverFirstSend(t *testing.T) {
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "failed", RetryLayer: "gateway", ResolvedModel: "openrouter/model-a", FailureCategory: "provider_error"},
		{AttemptNo: 2, Status: "completed", RetryLayer: "model_fallback", ResolvedModel: "openrouter/model-b"},
	}
	if got := FinalResolvedModel(attempts); got != "openrouter/model-b" {
		t.Fatalf("final = %q, want fallback openrouter/model-b", got)
	}
}

func TestFinalResolvedModelFinalFailureKeepsLastFailedModel(t *testing.T) {
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "requeued", RetryLayer: "gateway", ResolvedModel: "openrouter/model-a", FailureCategory: "timeout"},
		{AttemptNo: 2, Status: "failed", RetryLayer: "river_task", ResolvedModel: "openrouter/model-a", FailureCategory: "provider_error"},
	}
	if got := FinalResolvedModel(attempts); got != "openrouter/model-a" {
		t.Fatalf("final = %q, want openrouter/model-a", got)
	}
}

func TestFinalResolvedModelStaleRunningRowNeverShadowsTerminal(t *testing.T) {
	// Best-effort finalize can orphan a running row (crash between
	// start and finish). The terminal send owns the final model even
	// when the stale row carries a higher attempt number.
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "completed", RetryLayer: "gateway", ResolvedModel: "openrouter/model-a"},
		{AttemptNo: 2, Status: "running", RetryLayer: "gateway", ResolvedModel: "openrouter/model-b"},
	}
	if got := FinalResolvedModel(attempts); got != "openrouter/model-a" {
		t.Fatalf("final = %q, want terminal openrouter/model-a", got)
	}
}

func TestFinalResolvedModelInflightFallsBackToLatestSend(t *testing.T) {
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "requeued", RetryLayer: "gateway", ResolvedModel: "openrouter/model-a"},
		{AttemptNo: 2, Status: "running", RetryLayer: "river_task", ResolvedModel: "openrouter/model-a"},
	}
	if got := FinalResolvedModel(attempts); got != "openrouter/model-a" {
		t.Fatalf("final = %q, want latest send openrouter/model-a", got)
	}
}

func TestFinalResolvedModelMissingModelData(t *testing.T) {
	if got := FinalResolvedModel(nil); got != "" {
		t.Fatalf("nil attempts final = %q, want empty", got)
	}
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "running", RetryLayer: "gateway"},
	}
	if got := FinalResolvedModel(attempts); got != "" {
		t.Fatalf("model-less attempts final = %q, want empty", got)
	}
	if final := FinalAttempt(attempts); final != nil {
		t.Fatalf("model-less FinalAttempt = %+v, want nil", final)
	}
}

func TestFinalAttemptCarriesRetryLayerFacts(t *testing.T) {
	attempts := []AdminTaskAttempt{
		{AttemptNo: 1, Status: "failed", RetryLayer: "gateway", GatewayRetryCount: 3, ResolvedModel: "openrouter/model-a", RequestedModel: "openrouter/model-a", Service: "render", FailureCategory: "provider_error"},
		{AttemptNo: 2, Status: "failed", RetryLayer: "model_fallback", GatewayRetryCount: 0, ResolvedModel: "openrouter/model-b", RequestedModel: "openrouter/model-b", Service: "render", FailureCategory: "timeout"},
	}
	final := FinalAttempt(attempts)
	if final == nil {
		t.Fatal("FinalAttempt must resolve the fallback send")
	}
	if final.RetryLayer != "model_fallback" || final.FailureCategory != "timeout" {
		t.Fatalf("final layer/category = %q/%q, want model_fallback/timeout", final.RetryLayer, final.FailureCategory)
	}
	payload, err := json.Marshal(final)
	if err != nil {
		t.Fatalf("marshal final attempt: %v", err)
	}
	for _, field := range []string{`"retry_layer"`, `"gateway_retry_count"`, `"requested_model"`, `"service"`} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("final attempt JSON missing %s: %s", field, payload)
		}
	}
}
