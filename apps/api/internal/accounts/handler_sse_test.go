package accounts

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"

	"github.com/gin-gonic/gin"
)

func TestStreamUserTaskSendsTerminalTaskEventWithUserView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newAccountsTestDB(t)
	taskRepo := NewTaskRepository(db)
	ctx := context.Background()
	service := NewAccountService(AccountServiceDeps{TaskRepo: taskRepo}, AccountServiceOptions{TaskEventHub: NewTaskEventHub()})

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
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewAccountHandler(service)
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", int64(7))
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

func TestStreamUserTaskStreamsPendingThenCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newAccountsTestDB(t)
	taskRepo := NewTaskRepository(db)
	ctx := context.Background()
	hub := NewTaskEventHub()
	service := NewAccountService(AccountServiceDeps{TaskRepo: taskRepo}, AccountServiceOptions{TaskEventHub: hub})

	task := &tasks.Task{
		TaskID:     "task-sse-live",
		UserID:     8,
		ChannelID:  99,
		AccountID:  21,
		Prompt:     "internal prompt",
		UserPrompt: "user prompt",
		Status:     tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewAccountHandler(service)
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", int64(8))
		handler.StreamUserTask(c)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events/"+task.TaskID, nil)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	done := make(chan error, 1)
	go func() {
		time.Sleep(80 * time.Millisecond)
		now := time.Now()
		rows, updateErr := taskRepo.CompleteWithOutputs(ctx, task.TaskID, []string{tasks.StatusPending}, `{"status":"success","image_url":"https://example.com/live.png","account_id":21,"channel_id":99}`, now, nil)
		if updateErr != nil {
			done <- updateErr
			return
		}
		if rows != 1 {
			done <- fmt.Errorf("unexpected updated rows: %d", rows)
			return
		}
		hub.Publish(task.TaskID, TaskEvent{TaskID: task.TaskID, Status: tasks.StatusCompleted})
		done <- nil
	}()

	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader := bufio.NewReader(resp.Body)
	var payload strings.Builder
	for {
		select {
		case <-waitCtx.Done():
			t.Fatalf("timeout waiting sse payload: %s", payload.String())
		default:
		}
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read stream failed: %v, payload=%s", readErr, payload.String())
		}
		payload.WriteString(line)
		body := payload.String()
		if strings.Contains(body, `"status":"pending"`) && strings.Contains(body, `"status":"completed"`) {
			if strings.Contains(body, "internal prompt") || strings.Contains(body, "account_id") || strings.Contains(body, "channel_id") {
				t.Fatalf("stream leaked internal task fields: %s", body)
			}
			if err := <-done; err != nil {
				t.Fatalf("update/publish failed: %v", err)
			}
			return
		}
	}
}
