package accounts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/tasks"

	"github.com/gin-gonic/gin"
)

func TestStreamUserTaskSendsTerminalTaskEventWithUserView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewAccountService(nil, taskRepo, nil, nil, nil)
	service.SetTaskEventHub(NewTaskEventHub())

	task := &tasks.Task{
		TaskID:     "task-sse-terminal",
		UserID:     7,
		ChannelID:  9,
		AccountID:  11,
		Prompt:     "internal prompt",
		UserPrompt: "user prompt",
		Status:     tasks.StatusCompleted,
		Result:     `{"status":"success","image_url":"https://example.com/out.png","account_id":11,"channel_id":9}`,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewAccountHandler(service)
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", uint64(7))
		handler.StreamUserTask(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+task.TaskID, nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type=%q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: task") {
		t.Fatalf("body missing task event: %s", body)
	}
	if !strings.Contains(body, `"status":"completed"`) {
		t.Fatalf("body missing completed status: %s", body)
	}
	if strings.Contains(body, "internal prompt") || strings.Contains(body, "account_id") || strings.Contains(body, "channel_id") {
		t.Fatalf("body leaked internal task fields: %s", body)
	}
}
