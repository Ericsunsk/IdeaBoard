package taskservice

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/taskstore"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

func seedTaskServiceSSEAccount(t testing.TB, ctx context.Context, db *sql.DB, suffix string) *accounts.ApiAccount {
	t.Helper()

	var channelID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO channels (type, name, api_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`,
		"taskservice-sse-channel-"+suffix,
		"https://example.com/taskservice-sse-"+suffix,
	).Scan(&channelID); err != nil {
		t.Fatalf("create SSE channel failed: %v", err)
	}

	account := &accounts.ApiAccount{
		Name:      "taskservice-sse-account-" + suffix,
		ChannelID: channelID,
		Platform:  "openai-compatible",
		ApiType:   "openai-compatible",
		Models:    []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:    "test",
		ProxyURL:  "https://example.com/taskservice-sse-" + suffix,
		Status:    1,
	}
	if err := accounts.NewAccountRepository(db).Create(ctx, account); err != nil {
		t.Fatalf("create SSE account failed: %v", err)
	}
	return account
}

func TestStreamUserTaskSendsTerminalTaskEventWithUserView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	ctx := context.Background()
	service := NewService(ServiceDeps{TaskRepo: taskRepo}, ServiceOptions{EventHub: NewEventHub()})

	task := &tasks.Task{
		TaskID:     "task-sse-terminal",
		UserID:     7,
		Prompt:     "internal prompt",
		UserPrompt: "user prompt",
		Status:     tasks.StatusCompleted,
		Result:     `{"status":"success","image_url":"https://example.com/out.png","account_id":11,"channel_id":9}`,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewHandler(service)
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

func TestStreamUserTaskStreamsRunningThenCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	ctx := context.Background()
	hub := NewEventHub()
	service := NewService(ServiceDeps{TaskRepo: taskRepo}, ServiceOptions{EventHub: hub})
	account := seedTaskServiceSSEAccount(t, ctx, db, "live")

	task := &tasks.Task{
		TaskID:            "task-sse-live",
		UserID:            8,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Prompt:            "internal prompt",
		UserPrompt:        "user prompt",
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-sse-live",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewHandler(service)
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
		rows, updateErr := taskRepo.CompleteWithOutputsAndLeaseToken(ctx, task.TaskID, []string{tasks.StatusRunning}, account.ID, account.ChannelID, task.LeaseToken, `{"status":"success","image_url":"https://example.com/live.png","account_id":21,"channel_id":99}`, now, nil, nil)
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
		if strings.Contains(body, `"status":"running"`) && strings.Contains(body, `"status":"completed"`) {
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

func TestStreamUserTaskReturnsJSONWhenConnectionLimitReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	ctx := context.Background()
	hub := newEventHubWithLimits(10, 1)
	service := NewService(ServiceDeps{TaskRepo: taskRepo}, ServiceOptions{EventHub: hub})
	account := seedTaskServiceSSEAccount(t, ctx, db, "limit")

	task := &tasks.Task{
		TaskID:            "task-sse-limit",
		UserID:            9,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		UserPrompt:        "user prompt",
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-sse-limit",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	_, unsubscribe, ok := hub.Subscribe("occupied-task", 9)
	if !ok {
		t.Fatal("pre-subscribe should succeed")
	}
	defer unsubscribe()

	handler := NewHandler(service)
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", int64(9))
		handler.StreamUserTask(c)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+task.TaskID, nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	contentType := rec.Header().Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("content-type=%q should not be SSE for JSON error", contentType)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("content-type=%q, want application/json", contentType)
	}
	if !strings.Contains(rec.Body.String(), "SSE 连接数已达上限") {
		t.Fatalf("body missing limit message: %s", rec.Body.String())
	}
}

func TestStreamUserTaskHeartbeatSendsLatestSnapshotWhenPublishMissed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousInterval := taskStreamHeartbeatInterval
	taskStreamHeartbeatInterval = 20 * time.Millisecond
	defer func() {
		taskStreamHeartbeatInterval = previousInterval
	}()

	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	ctx := context.Background()
	service := NewService(ServiceDeps{TaskRepo: taskRepo}, ServiceOptions{EventHub: NewEventHub()})
	account := seedTaskServiceSSEAccount(t, ctx, db, "heartbeat")

	task := &tasks.Task{
		TaskID:            "task-sse-heartbeat",
		UserID:            10,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		UserPrompt:        "user prompt",
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-sse-heartbeat",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	handler := NewHandler(service)
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", int64(10))
		handler.StreamUserTask(c)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/events/"+task.TaskID, nil)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
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
		time.Sleep(50 * time.Millisecond)
		rows, updateErr := taskRepo.MarkTaskFailed(ctx, task.TaskID, []string{tasks.StatusRunning}, "provider timeout", "", time.Now(), nil)
		if updateErr != nil {
			done <- updateErr
			return
		}
		if rows != 1 {
			done <- fmt.Errorf("unexpected updated rows: %d", rows)
			return
		}
		done <- nil
	}()

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
		if strings.Contains(body, `"status":"running"`) && strings.Contains(body, `"status":"failed"`) {
			if strings.Contains(body, "internal prompt") {
				t.Fatalf("stream leaked internal task fields: %s", body)
			}
			if err := <-done; err != nil {
				t.Fatalf("update failed: %v", err)
			}
			return
		}
	}
}
