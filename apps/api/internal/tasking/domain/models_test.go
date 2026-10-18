package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskParamsColumnSupportsSubmitLimit(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "workflow_input JSONB NOT NULL") {
		t.Fatalf("tasks.workflow_input must use JSONB to support submit params limit")
	}
}

func TestTaskResultColumnSupportsProviderMetadata(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "result TEXT NOT NULL") {
		t.Fatalf("tasks.result must use TEXT to support provider metadata")
	}
}

func TestTaskOutputUsesTaskResultIndex(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "CREATE INDEX idx_task_outputs_task ON task_outputs (task_id, output_index)") {
		t.Fatalf("task_outputs must keep task_id/output_index index")
	}
	if strings.Contains(schema, "sort_index") {
		t.Fatalf("task_outputs must not retain sort_index")
	}
}

func TestTaskOutputCascadesWithTaskDelete(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "task_id BIGINT NOT NULL REFERENCES tasks(internal_id) ON DELETE CASCADE") {
		t.Fatalf("task_outputs must cascade when parent task is deleted")
	}
}

func TestTaskOutputSchemaStoresRelationOnly(t *testing.T) {
	schema := readDBSchema(t)
	taskOutputsStart := strings.Index(schema, "CREATE TABLE task_outputs")
	if taskOutputsStart < 0 {
		t.Fatal("task_outputs table missing")
	}
	taskOutputsEnd := strings.Index(schema[taskOutputsStart:], ");")
	if taskOutputsEnd < 0 {
		t.Fatal("task_outputs table block missing terminator")
	}
	taskOutputs := schema[taskOutputsStart : taskOutputsStart+taskOutputsEnd]
	for _, forbidden := range []string{
		"storage_key",
		"mime_type",
		"width INTEGER",
		"height INTEGER",
		"size_bytes",
		"checksum_sha256",
		"storage_profile",
	} {
		if strings.Contains(taskOutputs, forbidden) {
			t.Fatalf("task_outputs should not store MediaAsset field %q", forbidden)
		}
	}
	for _, expected := range []string{
		"output_kind TEXT NOT NULL",
		"CONSTRAINT chk_task_outputs_kind CHECK (output_kind IN ('media_asset','remote_url'))",
		"CONSTRAINT chk_task_outputs_kind_shape CHECK",
	} {
		if !strings.Contains(taskOutputs, expected) {
			t.Fatalf("task_outputs missing relation-only contract: %s", expected)
		}
	}
}

func TestTaskUserViewIncludesCancelReason(t *testing.T) {
	task := &Task{
		ID:           1,
		TaskID:       "task-cancelled",
		UserID:       42,
		UserPrompt:   "draw",
		Status:       StatusCancelled,
		CancelReason: "用户取消",
	}

	view := task.ToUserView()
	if got := view.CancelReason; got != "用户取消" {
		t.Fatalf("cancel_reason = %#v, want 用户取消", got)
	}
}

func TestTaskUserViewOmitsInternalIdentity(t *testing.T) {
	view := (&Task{ID: 1, UserID: 42, TaskID: "task-public"}).ToUserView()
	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal user view: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode user view: %v", err)
	}
	for _, forbidden := range []string{"id", "user_id"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("user view leaked internal field %q: %s", forbidden, payload)
		}
	}
}

func TestTaskOutputPublicJSONUsesAssetID(t *testing.T) {
	internalID := int64(42)
	output := TaskOutput{
		ID:           7,
		TaskID:       "task-output-contract",
		MediaAssetID: &internalID,
		AssetID:      "asset_public",
		OutputKind:   "asset",
		Role:         "image",
		URL:          "/api/v1/workspaces/ws_public/media-assets/asset_public/content",
	}

	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal task output: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode task output: %v", err)
	}
	if _, exists := fields["media_asset_id"]; exists {
		t.Fatalf("task output leaked internal media_asset_id: %s", payload)
	}
	if got, ok := fields["asset_id"]; !ok || string(got) != `"asset_public"` {
		t.Fatalf("task output asset_id = %s, want public asset id", got)
	}

	view := (&Task{TaskID: output.TaskID, Outputs: []TaskOutput{output}}).ToUserView()
	if len(view.Outputs) != 1 || view.Outputs[0].AssetID != "asset_public" {
		t.Fatalf("user output asset_id = %#v, want asset_public", view.Outputs)
	}
}

func TestTaskCorrelationIDFallsBackToTaskID(t *testing.T) {
	task := &Task{TaskID: "task-trace"}

	if got := task.EffectiveCorrelationID(); got != "task-trace" {
		t.Fatalf("correlation_id=%q, want task id", got)
	}

	view := task.ToUserView()
	if view.CorrelationID != "task-trace" {
		t.Fatalf("user view correlation_id=%q, want task id", view.CorrelationID)
	}
}

func TestTaskUserViewIncludesExecutionFailureMetadata(t *testing.T) {
	task := &Task{
		ID:         1,
		TaskID:     "task-storage-failed",
		UserID:     42,
		UserPrompt: "draw",
		Status:     StatusFailed,
		ErrorMsg:   "图片结果保存失败，请检查存储配置",
		ExecutionMetrics: &TaskExecutionMetrics{
			TaskID:          "task-storage-failed",
			CurrentStage:    ExecutionStageFailed,
			FailureStage:    ExecutionStageFinalizing,
			FailureCategory: FailureCategoryStorageError,
		},
	}

	view := task.ToUserView()
	if view.ExecutionMetrics == nil {
		t.Fatal("execution_metrics missing from user view")
	}
	if got := view.ExecutionMetrics.FailureStage; got != ExecutionStageFinalizing {
		t.Fatalf("failure_stage = %#v, want %#v", got, ExecutionStageFinalizing)
	}
	if got := view.ExecutionMetrics.FailureCategory; got != FailureCategoryStorageError {
		t.Fatalf("failure_category = %#v, want %#v", got, FailureCategoryStorageError)
	}
}

func TestTaskUserViewResultUsesAllowlist(t *testing.T) {
	task := &Task{
		ID:         1,
		TaskID:     "task-safe-result",
		UserID:     42,
		UserPrompt: "draw",
		Status:     StatusCompleted,
		Result: `{
			"task_id":"task-safe-result",
			"status":"success",
			"image_url":"/uploads/tasks/42/task-safe-result/out.png",
			"image_urls":["/uploads/tasks/42/task-safe-result/out.png"],
			"model":"gpt-image-1",
			"prompt":"secret prompt",
			"account_id":7,
			"provider_endpoint_id":8,
			"platform":"openai"
		}`,
		Outputs: []TaskOutput{{
			ID:             10,
			TaskID:         "task-safe-result",
			Role:           "image",
			URL:            "/uploads/tasks/42/task-safe-result/out.png",
			StorageKey:     "uploads/tasks/42/task-safe-result/out.png",
			SourceURL:      "https://provider.example/out.png",
			ChecksumSHA256: "secret-checksum",
			MIMEType:       "image/png",
			Width:          1024,
			Height:         1024,
			SizeBytes:      123,
		}},
	}

	view := task.ToUserView()

	if strings.Contains(view.Result, "prompt") ||
		strings.Contains(view.Result, "account_id") ||
		strings.Contains(view.Result, "provider_endpoint_id") ||
		strings.Contains(view.Result, "platform") {
		t.Fatalf("result leaked non-allowlisted provider metadata: %s", view.Result)
	}
	if !strings.Contains(view.Result, `"image_url":"/uploads/tasks/42/task-safe-result/out.png"`) {
		t.Fatalf("result missing image_url allowlist field: %s", view.Result)
	}
	if len(view.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(view.Outputs))
	}
	if view.Outputs[0].URL != "/uploads/tasks/42/task-safe-result/out.png" {
		t.Fatalf("output url = %#v", view.Outputs[0].URL)
	}
}

func TestSafeTaskResultJSONDropsInvalidRawResult(t *testing.T) {
	if got := SafeTaskResultJSON("not json"); got != "" {
		t.Fatalf("safe result = %#v, want empty string", got)
	}
}

func TestTaskExecutionMetricsCascadesWithTaskDelete(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "CREATE TABLE task_execution_metrics") {
		t.Fatalf("task_execution_metrics table must exist")
	}
	// #809 hard cut: the legacy shell stays for compiling stubs, but its
	// dangling FK to the replaced user-scoped tasks(task_id) is dropped.
	if strings.Contains(schema, "REFERENCES tasks(task_id)") {
		t.Fatalf("legacy shells must not reference the replaced tasks(task_id)")
	}
}

func readDBSchema(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	return string(raw)
}

// merged from routing_observability_test.go (models.go aggregation: ParseAdminRoutingSnapshot/FinalResolvedModel observability projections).
// Routing observability projections (#852): Admin snapshot parsing and
// final-model derivation over attempt facts. Pure unit coverage (no DB):
// success, retry/fallback, final failure, stale running rows, in-flight
// tasks, and missing model data.

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
