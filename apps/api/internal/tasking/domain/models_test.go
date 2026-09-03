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
	if !strings.Contains(schema, "params TEXT NOT NULL") {
		t.Fatalf("tasks.params must use TEXT to support submit params limit")
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
	if !strings.Contains(schema, "CREATE INDEX idx_task_outputs_task_sort ON task_outputs (task_id, result_index)") {
		t.Fatalf("task_outputs must keep task_id/result_index index")
	}
	if strings.Contains(schema, "sort_index") {
		t.Fatalf("task_outputs must not retain sort_index")
	}
}

func TestTaskOutputCascadesWithTaskDelete(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "CONSTRAINT fk_task_outputs_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE") {
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
		"output_kind VARCHAR(32) NOT NULL DEFAULT 'asset'",
		"CONSTRAINT ck_task_outputs_output_kind CHECK (output_kind IN ('asset', 'remote'))",
		"CONSTRAINT ck_task_outputs_output_kind_shape CHECK",
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
		URL:          "/api/user/media-assets/asset_public/original/content",
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
	if !strings.Contains(schema, "CONSTRAINT fk_task_execution_metrics_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE") {
		t.Fatalf("task_execution_metrics must cascade when parent task is deleted")
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
