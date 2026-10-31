package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
)

// TaskReadStore exposes the minimal read projection needed by task use cases.
type TaskReadStore = taskports.TaskReadStore

// TaskLifecycleStore exposes task creation and state transitions.
type TaskLifecycleStore = taskports.TaskLifecycleStore

// TaskClaimStore exposes the worker-only lease fence (pending -> running).
type TaskClaimStore = taskports.TaskClaimStore

// TaskOutputStore owns task output reference persistence independently from
// task state transitions.
type TaskOutputStore interface {
	AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error)
	FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error)
}

// InMemoryStore is a mutex-guarded implementation of the task persistence
// seams for tests. Tasks are keyed by (workspace_id, task_id).
type InMemoryStore struct {
	mu     sync.Mutex
	tasks  map[string]*tasks.WorkspaceTask
	leases map[string]string
	now    func() time.Time
}

// MemoryStore is an alias for InMemoryStore.
type MemoryStore = InMemoryStore

var _ taskports.TaskReadStore = (*InMemoryStore)(nil)
var _ taskports.TaskLifecycleStore = (*InMemoryStore)(nil)
var _ taskports.TaskClaimStore = (*InMemoryStore)(nil)
var _ TaskOutputStore = (*InMemoryStore)(nil)

// NewInMemoryStoreForTests builds an empty store.
func NewInMemoryStoreForTests() *InMemoryStore {
	return &InMemoryStore{tasks: make(map[string]*tasks.WorkspaceTask), leases: make(map[string]string), now: time.Now}
}

// NewMemoryStore builds an empty memory store.
func NewMemoryStore() *InMemoryStore {
	return NewInMemoryStoreForTests()
}

// SetClockForTests injects a deterministic clock.
func (s *InMemoryStore) SetClockForTests(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// RestoreTaskForTest restores the pre-image of one task row.
func (s *InMemoryStore) RestoreTaskForTest(workspaceID, taskID string, before *tasks.WorkspaceTask) {
	if s == nil || before == nil {
		return
	}
	snapshot := *before
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskKey(workspaceID, taskID)] = &snapshot
}

func (s *InMemoryStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func taskKey(workspaceID, taskID string) string {
	return strings.TrimSpace(workspaceID) + "\x00" + strings.TrimSpace(taskID)
}

func copyTask(in *tasks.WorkspaceTask) *tasks.WorkspaceTask {
	if in == nil {
		return nil
	}
	out := *in
	out.MediaAssetIDs = append([]string(nil), in.MediaAssetIDs...)
	out.OutputRefs = append([]tasks.OutputRef(nil), in.OutputRefs...)
	if in.Params != nil {
		params := make(map[string]any, len(in.Params))
		for k, v := range in.Params {
			params[k] = v
		}
		out.Params = params
	}
	if in.CompletedAt != nil {
		completed := *in.CompletedAt
		out.CompletedAt = &completed
	}
	out.RoutingSnapshot = append([]byte(nil), in.RoutingSnapshot...)
	return &out
}

func (s *InMemoryStore) CreateTask(ctx context.Context, task *tasks.WorkspaceTask) (*tasks.WorkspaceTask, error) {
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspacetasks: task identity required: %w", tasks.ErrValidation)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(task.WorkspaceID, task.ID)
	if _, exists := s.tasks[key]; exists {
		return nil, fmt.Errorf("workspacetasks: duplicate task: %w", tasks.ErrStateConflict)
	}
	now := s.clock()
	stored := copyTask(task)
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.tasks[key] = stored
	return copyTask(stored), nil
}

func (s *InMemoryStore) GetTask(ctx context.Context, workspaceID, taskID string) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	return copyTask(stored), nil
}

func (s *InMemoryStore) ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []*tasks.WorkspaceTask
	for _, stored := range s.tasks {
		if stored.WorkspaceID == strings.TrimSpace(workspaceID) {
			all = append(all, copyTask(stored))
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	if offset < 0 {
		offset = 0
	}
	if offset > len(all) {
		offset = len(all)
	}
	if limit <= 0 {
		limit = 20
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (s *InMemoryStore) CountTasks(ctx context.Context, workspaceID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, stored := range s.tasks {
		if stored.WorkspaceID == strings.TrimSpace(workspaceID) {
			count++
		}
	}
	return count, nil
}

func (s *InMemoryStore) DeleteTask(ctx context.Context, workspaceID, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(workspaceID, taskID)
	if _, ok := s.tasks[key]; !ok {
		return tasks.ErrNotFound
	}
	delete(s.tasks, key)
	delete(s.leases, key)
	return nil
}

func (s *InMemoryStore) ClaimTask(ctx context.Context, workspaceID, taskID, leaseToken string) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(workspaceID, taskID)
	stored, ok := s.tasks[key]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	if strings.TrimSpace(leaseToken) == "" {
		return nil, fmt.Errorf("workspacetasks: lease token required: %w", tasks.ErrValidation)
	}
	leaseToken = strings.TrimSpace(leaseToken)
	if stored.Status == tasks.StatusRunning {
		if held, ok := s.leases[key]; !ok || held != leaseToken {
			return nil, tasks.ErrStateConflict
		}
	} else if stored.Status != tasks.StatusPending {
		return nil, tasks.ErrStateConflict
	} else if _, held := s.leases[key]; held {
		return nil, tasks.ErrStateConflict
	}
	now := s.clock()
	stored.Status = tasks.StatusRunning
	s.leases[key] = strings.TrimSpace(leaseToken)
	stored.UpdatedAt = now
	return copyTask(stored), nil
}

func (s *InMemoryStore) releaseLeaseLocked(workspaceID, taskID string) {
	delete(s.leases, taskKey(workspaceID, taskID))
}

func (s *InMemoryStore) CancelTask(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*tasks.WorkspaceTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, false, tasks.ErrNotFound
	}
	if stored.IsTerminal() {
		return copyTask(stored), true, nil
	}
	now := s.clock()
	stored.Status = tasks.StatusCancelled
	stored.CancelReason = strings.TrimSpace(reason)
	if stored.CancelReason == "" {
		stored.CancelReason = "cancelled"
	}
	completed := now
	stored.CompletedAt = &completed
	stored.UpdatedAt = now
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), false, nil
}

func (s *InMemoryStore) AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	if len(stored.OutputRefs)+len(outputs) > tasks.MaxOutputsPerTask {
		return nil, tasks.NewValidationError(tasks.FieldError{Field: "outputs", Message: "too_many"})
	}
	stored.OutputRefs = append(append([]tasks.OutputRef(nil), stored.OutputRefs...), outputs...)
	stored.UpdatedAt = s.clock()
	return copyTask(stored), nil
}

func (s *InMemoryStore) TransitionForRetry(ctx context.Context, workspaceID, taskID, newToken string) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, tasks.ErrStateConflict
	}
	if strings.TrimSpace(newToken) == "" {
		return nil, fmt.Errorf("workspacetasks: execution token required: %w", tasks.ErrValidation)
	}
	stored.RetryCount++
	stored.ExecutionToken = newToken
	stored.Status = tasks.StatusPending
	stored.UpdatedAt = s.clock()
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), nil
}

func (s *InMemoryStore) CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, tasks.ErrStateConflict
	}
	now := s.clock()
	stored.Status = tasks.StatusCompleted
	stored.OutputRefs = append([]tasks.OutputRef(nil), outputs...)
	completed := now
	stored.CompletedAt = &completed
	stored.UpdatedAt = now
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), nil
}

func (s *InMemoryStore) FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, tasks.ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, tasks.ErrStateConflict
	}
	now := s.clock()
	stored.Status = tasks.StatusFailed
	stored.ErrorMsg = strings.TrimSpace(errorMsg)
	if stored.ErrorMsg == "" {
		stored.ErrorMsg = "task failed"
	}
	completed := now
	stored.CompletedAt = &completed
	stored.UpdatedAt = now
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), nil
}

// recordedAttempt is one in-memory attempt fact.
type recordedAttempt struct {
	start     tasks.AttemptStart
	finish    *tasks.AttemptFinish
	attemptNo int32
}

// InMemoryAttemptRecorder is a mutex-guarded AttemptRecorder for tests.
type InMemoryAttemptRecorder struct {
	mu       sync.Mutex
	attempts map[string][]*recordedAttempt
}

var _ taskports.AttemptRecorder = (*InMemoryAttemptRecorder)(nil)

// NewInMemoryAttemptRecorderForTests builds an empty recorder.
func NewInMemoryAttemptRecorderForTests() *InMemoryAttemptRecorder {
	return &InMemoryAttemptRecorder{attempts: make(map[string][]*recordedAttempt)}
}

// StartAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) StartAttempt(_ context.Context, start tasks.AttemptStart) (int32, error) {
	if r == nil {
		return 0, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.attempts[start.TaskID]
	attemptNo := int32(len(rows) + 1)
	r.attempts[start.TaskID] = append(r.attempts[start.TaskID], &recordedAttempt{start: start, attemptNo: attemptNo})
	return attemptNo, nil
}

// FinishAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) FinishAttempt(_ context.Context, finish tasks.AttemptFinish) (int64, error) {
	if r == nil {
		return 0, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.attempts[finish.TaskID] {
		if row.start.ExecutionToken == finish.ExecutionToken && row.finish == nil {
			closed := finish
			row.finish = &closed
			return 1, nil
		}
	}
	return 0, nil
}

// HasModelFallbackAttempt implements AttemptRecorder.
func (r *InMemoryAttemptRecorder) HasModelFallbackAttempt(_ context.Context, taskID string) (bool, error) {
	if r == nil {
		return false, errors.New("workspacetasks: attempt recorder is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.attempts[taskID] {
		if row.start.RetryLayer == tasks.AttemptRetryLayerModelFallback {
			return true, nil
		}
	}
	return false, nil
}

// StartedAttempts returns all started attempt facts recorded for taskID.
func (r *InMemoryAttemptRecorder) StartedAttempts(taskID string) []tasks.AttemptStart {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.attempts[taskID]
	out := make([]tasks.AttemptStart, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.start)
	}
	return out
}

// FinishedAttempts returns all finished attempt facts recorded for taskID.
func (r *InMemoryAttemptRecorder) FinishedAttempts(taskID string) []tasks.AttemptFinish {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.attempts[taskID]
	out := make([]tasks.AttemptFinish, 0, len(rows))
	for _, row := range rows {
		if row.finish != nil {
			out = append(out, *row.finish)
		}
	}
	return out
}
