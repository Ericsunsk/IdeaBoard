package accounts

import (
	"testing"

	"api/internal/tasks"
)

func TestApplyPreviewForTask(t *testing.T) {
	svc := &AccountService{}
	transform := func(raw string) string {
		return raw + "?thumb=1"
	}

	task := &tasks.Task{
		Result: `{"image_url":"https://img.example.com/a.png","image_urls":["https://img.example.com/a.png","https://img.example.com/b.png"]}`,
	}

	svc.applyPreviewForTask(task, transform)

	want := `{"image_url":"https://img.example.com/a.png?thumb=1","image_urls":["https://img.example.com/a.png?thumb=1","https://img.example.com/b.png?thumb=1"]}`
	if task.Result != want {
		t.Fatalf("got %s, want %s", task.Result, want)
	}
}
