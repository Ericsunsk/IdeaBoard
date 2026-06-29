package tasks

import (
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

func TestTaskOutputUsesTaskSortIndex(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "CREATE INDEX idx_task_outputs_task_sort ON task_outputs (task_id, sort_index)") {
		t.Fatalf("task_outputs must keep task_id/sort_index index")
	}
}

func TestTaskOutputCascadesWithTaskDelete(t *testing.T) {
	schema := readDBSchema(t)
	if !strings.Contains(schema, "CONSTRAINT fk_task_outputs_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE") {
		t.Fatalf("task_outputs must cascade when parent task is deleted")
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
			"channel_id":8,
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
		strings.Contains(view.Result, "channel_id") ||
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
