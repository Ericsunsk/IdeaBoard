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

func readDBSchema(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	return string(raw)
}
