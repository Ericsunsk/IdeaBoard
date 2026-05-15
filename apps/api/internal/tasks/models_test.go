package tasks

import (
	"reflect"
	"strings"
	"testing"
)

func TestTaskParamsColumnSupportsSubmitLimit(t *testing.T) {
	field, ok := reflect.TypeOf(Task{}).FieldByName("Params")
	if !ok {
		t.Fatalf("Task.Params field not found")
	}
	if !strings.Contains(field.Tag.Get("gorm"), "type:mediumtext") {
		t.Fatalf("Task.Params must use mediumtext to support submit params limit, got %q", field.Tag.Get("gorm"))
	}
}

func TestTaskResultColumnSupportsProviderMetadata(t *testing.T) {
	field, ok := reflect.TypeOf(Task{}).FieldByName("Result")
	if !ok {
		t.Fatalf("Task.Result field not found")
	}
	if !strings.Contains(field.Tag.Get("gorm"), "type:mediumtext") {
		t.Fatalf("Task.Result must use mediumtext to support provider metadata, got %q", field.Tag.Get("gorm"))
	}
}

func TestTaskOutputUsesTaskSortIndex(t *testing.T) {
	field, ok := reflect.TypeOf(TaskOutput{}).FieldByName("SortIndex")
	if !ok {
		t.Fatalf("TaskOutput.SortIndex field not found")
	}
	if !strings.Contains(field.Tag.Get("gorm"), "idx_task_outputs_task_sort") {
		t.Fatalf("TaskOutput.SortIndex must be indexed with task_id, got %q", field.Tag.Get("gorm"))
	}
}
