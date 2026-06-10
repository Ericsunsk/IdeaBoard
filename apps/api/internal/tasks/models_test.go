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
	if got := view["cancel_reason"]; got != "用户取消" {
		t.Fatalf("cancel_reason = %#v, want 用户取消", got)
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
	raw, err := os.ReadFile(filepath.Join("..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	return string(raw)
}
