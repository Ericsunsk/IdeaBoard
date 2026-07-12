package taskservice

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/gin-gonic/gin"
)

func seedTaskServiceSSEAccount(t testing.TB, ctx context.Context, db *sql.DB, suffix string) *accounts.ApiAccount {
	t.Helper()
	secretstest.EnableEncryption(t)

	var channelID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`,
		"taskservice-sse-channel-"+suffix,
		"https://example.com/taskservice-sse-"+suffix,
	).Scan(&channelID); err != nil {
		t.Fatalf("create SSE channel failed: %v", err)
	}

	account := &accounts.ApiAccount{
		Name:               "taskservice-sse-account-" + suffix,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "test",
		ProxyURL:           "https://example.com/taskservice-sse-" + suffix,
		Status:             1,
	}
	if err := accounts.NewAccountRepository(db).Create(ctx, account); err != nil {
		t.Fatalf("create SSE account failed: %v", err)
	}
	return account
}

func seedTaskServiceSSEUser(t testing.TB, ctx context.Context, db *sql.DB, id int64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, username, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("taskservice_sse_user_%d", id),
		fmt.Sprintf("SSE%03d", id),
	); err != nil {
		t.Fatalf("seed SSE user %d failed: %v", id, err)
	}
}

type streamTaskFetchResult struct {
	task *tasks.Task
	err  error
}

type streamTaskStore struct {
	mu          sync.Mutex
	userResults []streamTaskFetchResult
}

func newStreamTaskStore(userResults ...streamTaskFetchResult) *streamTaskStore {
	return &streamTaskStore{userResults: userResults}
}

func (s *streamTaskStore) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	return nil, errors.New("unexpected GetByTaskID call")
}

func (s *streamTaskStore) GetByTaskIDAndUserID(context.Context, string, int64) (*tasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.userResults) == 0 {
		return nil, errors.New("unexpected GetByTaskIDAndUserID call")
	}
	result := s.userResults[0]
	s.userResults = s.userResults[1:]
	if result.err != nil {
		return nil, result.err
	}
	if result.task == nil {
		return nil, errors.New("nil task fetch result")
	}
	task := *result.task
	return &task, nil
}

func (s *streamTaskStore) ListByTaskIDs(context.Context, []string) ([]*tasks.Task, error) {
	return nil, errors.New("unexpected ListByTaskIDs call")
}

func (s *streamTaskStore) List(context.Context, int64, int64, string, string, int, int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	return nil, tasks.TaskListCounts{}, errors.New("unexpected List call")
}

func (s *streamTaskStore) GetByUserID(context.Context, int64, int, int) ([]*tasks.Task, int64, error) {
	return nil, 0, errors.New("unexpected GetByUserID call")
}

func (s *streamTaskStore) DeleteByTaskIDAndUserID(context.Context, string, int64) error {
	return errors.New("unexpected DeleteByTaskIDAndUserID call")
}

func (s *streamTaskStore) DeleteByTaskIDs(context.Context, []string) (int64, error) {
	return 0, errors.New("unexpected DeleteByTaskIDs call")
}

func (s *streamTaskStore) MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error) {
	return 0, errors.New("unexpected MarkTaskCancelled call")
}

func (s *streamTaskStore) ListOutputsByTaskIDs(context.Context, []string) (map[string][]tasks.TaskOutput, error) {
	return map[string][]tasks.TaskOutput{}, nil
}

func makeStreamTask(taskID string, userID int64, status string) *tasks.Task {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &tasks.Task{
		TaskID:     taskID,
		UserID:     userID,
		UserPrompt: "user prompt",
		Status:     status,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func newStreamTestRouter(handler *Handler, userID int64) *gin.Engine {
	router := gin.New()
	router.GET("/events/:task_id", func(c *gin.Context) {
		c.Set("user_id", userID)
		handler.StreamUserTask(c)
	})
	return router
}

func captureSlogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()

	previous := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}

func serveStreamWithTimeout(t *testing.T, router *gin.Engine, taskID string, trigger func()) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+taskID, nil)
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	if trigger != nil {
		trigger()
	}

	select {
	case <-done:
		return rec
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting stream response, body=%s", rec.Body.String())
		return rec
	}
}

func waitForTaskSubscriber(t *testing.T, hub *EventHub, taskID string) {
	t.Helper()

	deadline := time.After(1 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if hub.subscriberCount(taskID) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting subscriber for %s", taskID)
		case <-tick.C:
		}
	}
}

func assertStreamErrorEvent(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type=%q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("body missing error event: %s", body)
	}
	if !strings.Contains(body, `"message":"获取任务状态失败"`) {
		t.Fatalf("body missing status fetch failure message: %s", body)
	}
}

func assertStreamFetchWarning(t *testing.T, logs *bytes.Buffer, source string) {
	t.Helper()

	output := logs.String()
	if !strings.Contains(output, "fetch SSE task status failed") {
		t.Fatalf("logs missing fetch warning: %s", output)
	}
	if !strings.Contains(output, "source="+source) {
		t.Fatalf("logs missing source %q: %s", source, output)
	}
}

func TestStreamUserTaskKeepsHTTPNotFoundBeforeSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskID := "task-sse-before-prepare-not-found"
	store := newStreamTaskStore(streamTaskFetchResult{err: sql.ErrNoRows})
	service := NewService(ServiceDeps{TaskRepo: store}, ServiceOptions{EventHub: NewEventHub()})
	router := newStreamTestRouter(NewHandler(service), 11)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events/"+taskID, nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type=%q should not be SSE before initial fetch succeeds", got)
	}
	if !strings.Contains(rec.Body.String(), "任务不存在") {
		t.Fatalf("body missing not found message: %s", rec.Body.String())
	}
}

func TestStreamUserTaskWritesErrorEventWhenInitialSnapshotFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskID := "task-sse-initial-snapshot-fetch-error"
	userID := int64(12)
	task := makeStreamTask(taskID, userID, tasks.StatusRunning)
	store := newStreamTaskStore(
		streamTaskFetchResult{task: task},
		streamTaskFetchResult{err: errors.New("snapshot fetch failed")},
	)
	service := NewService(ServiceDeps{TaskRepo: store}, ServiceOptions{EventHub: NewEventHub()})
	logs := captureSlogOutput(t)
	router := newStreamTestRouter(NewHandler(service), userID)

	rec := serveStreamWithTimeout(t, router, taskID, nil)

	assertStreamErrorEvent(t, rec)
	assertStreamFetchWarning(t, logs, "initial_snapshot")
}

func TestStreamUserTaskWritesErrorEventWhenHeartbeatFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousInterval := taskStreamHeartbeatInterval
	taskStreamHeartbeatInterval = 10 * time.Millisecond
	defer func() {
		taskStreamHeartbeatInterval = previousInterval
	}()

	taskID := "task-sse-heartbeat-fetch-error"
	userID := int64(13)
	task := makeStreamTask(taskID, userID, tasks.StatusRunning)
	store := newStreamTaskStore(
		streamTaskFetchResult{task: task},
		streamTaskFetchResult{task: task},
		streamTaskFetchResult{err: errors.New("heartbeat fetch failed")},
	)
	service := NewService(ServiceDeps{TaskRepo: store}, ServiceOptions{EventHub: NewEventHub()})
	logs := captureSlogOutput(t)
	router := newStreamTestRouter(NewHandler(service), userID)

	rec := serveStreamWithTimeout(t, router, taskID, nil)

	assertStreamErrorEvent(t, rec)
	assertStreamFetchWarning(t, logs, "heartbeat")
}

func TestStreamUserTaskWritesErrorEventWhenEventFetchFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	taskID := "task-sse-event-fetch-error"
	userID := int64(14)
	task := makeStreamTask(taskID, userID, tasks.StatusRunning)
	hub := NewEventHub()
	store := newStreamTaskStore(
		streamTaskFetchResult{task: task},
		streamTaskFetchResult{task: task},
		streamTaskFetchResult{err: errors.New("event fetch failed")},
	)
	service := NewService(ServiceDeps{TaskRepo: store}, ServiceOptions{EventHub: hub})
	logs := captureSlogOutput(t)
	router := newStreamTestRouter(NewHandler(service), userID)

	rec := serveStreamWithTimeout(t, router, taskID, func() {
		waitForTaskSubscriber(t, hub, taskID)
		hub.Publish(taskID, TaskEvent{TaskID: taskID, Status: tasks.StatusRunning})
	})

	assertStreamErrorEvent(t, rec)
	assertStreamFetchWarning(t, logs, "event")
}

func TestStreamUserTaskSendsTerminalTaskEventWithUserView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	ctx := context.Background()
	service := NewService(ServiceDeps{TaskRepo: taskRepo}, ServiceOptions{EventHub: NewEventHub()})
	seedTaskServiceSSEUser(t, ctx, db, 7)

	task := &tasks.Task{
		TaskID:     "task-sse-terminal",
		UserID:     7,
		Prompt:     "internal prompt",
		UserPrompt: "user prompt",
		Status:     tasks.StatusCompleted,
		Result:     `{"status":"success","image_url":"https://example.com/out.png","account_id":11,"provider_endpoint_id":9}`,
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
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
	if strings.Contains(body, "internal prompt") || strings.Contains(body, "account_id") || strings.Contains(body, "provider_endpoint_id") {
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
	seedTaskServiceSSEUser(t, ctx, db, 8)

	task := &tasks.Task{
		TaskID:                     "task-sse-live",
		UserID:                     8,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		Prompt:                     "internal prompt",
		UserPrompt:                 "user prompt",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-sse-live",
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
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
		rows, updateErr := taskRepo.CompleteWithOutputsAndLeaseToken(ctx, task.TaskID, []string{tasks.StatusRunning}, account.ID, account.ProviderEndpointID, loadTaskServiceSSECredentialID(t, db, account.ID), task.LeaseToken, `{"status":"success","image_url":"https://example.com/live.png","account_id":21,"provider_endpoint_id":99}`, now, nil, nil)
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
			if strings.Contains(body, "internal prompt") || strings.Contains(body, "account_id") || strings.Contains(body, "provider_endpoint_id") {
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
	seedTaskServiceSSEUser(t, ctx, db, 9)

	task := &tasks.Task{
		TaskID:                     "task-sse-limit",
		UserID:                     9,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		UserPrompt:                 "user prompt",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-sse-limit",
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
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
	seedTaskServiceSSEUser(t, ctx, db, 10)

	task := &tasks.Task{
		TaskID:                     "task-sse-heartbeat",
		UserID:                     10,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		UserPrompt:                 "user prompt",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-sse-heartbeat",
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
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

func loadTaskServiceSSECredentialID(t testing.TB, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var credentialID int64
	if err := db.QueryRow(`
SELECT id
FROM api_account_secrets
WHERE account_id = $1 AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, accountID).Scan(&credentialID); err != nil {
		t.Fatalf("load SSE account %d credential failed: %v", accountID, err)
	}
	return credentialID
}
