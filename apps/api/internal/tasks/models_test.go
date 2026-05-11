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
