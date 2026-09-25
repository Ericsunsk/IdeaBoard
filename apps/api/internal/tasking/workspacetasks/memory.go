package workspacetasks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// WorkspaceTaskStore is the additive persistence seam for workspace-scoped
// tasks. Repository methods take context.Context first and use the
// dbWithContext pattern in the postgres sibling (#809); the in-memory
// sibling below is the test backend (and the fail-closed stand-in until
// the postgres mapping lands).
type WorkspaceTaskStore interface {
	CreateTask(ctx context.Context, task *WorkspaceTask) (*WorkspaceTask, error)
	GetTask(ctx context.Context, workspaceID, taskID string) (*WorkspaceTask, error)
	ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*WorkspaceTask, error)
	CancelTask(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error)
	AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error)
	TransitionForRetry(ctx context.Context, workspaceID, taskID, newToken string) (*WorkspaceTask, error)
	CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error)
	FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error)
	DeleteTask(ctx context.Context, workspaceID, taskID string) error
	CountTasks(ctx context.Context, workspaceID string) (int, error)
}

// InMemoryStore is a mutex-guarded WorkspaceTaskStore for tests. Tasks are
// keyed by (workspace_id, task_id); cross-workspace ids stay invisible
// (ErrNotFound), never 403. Leases mirror the postgres lease_token column:
// held separately from the envelope execution token, set on claim, cleared
// on every terminal/retry flip.
type InMemoryStore struct {
	mu     sync.Mutex
	tasks  map[string]*WorkspaceTask
	leases map[string]string
	now    func() time.Time
}

// NewInMemoryStoreForTests builds an empty store.
func NewInMemoryStoreForTests() *InMemoryStore {
	return &InMemoryStore{tasks: make(map[string]*WorkspaceTask), leases: make(map[string]string), now: time.Now}
}

// SetClockForTests injects a deterministic clock.
func (s *InMemoryStore) SetClockForTests(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
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

func copyTask(in *WorkspaceTask) *WorkspaceTask {
	if in == nil {
		return nil
	}
	out := *in
	out.MediaAssetIDs = append([]string(nil), in.MediaAssetIDs...)
	out.OutputRefs = append([]OutputRef(nil), in.OutputRefs...)
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
	return &out
}

func (s *InMemoryStore) CreateTask(ctx context.Context, task *WorkspaceTask) (*WorkspaceTask, error) {
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return nil, fmt.Errorf("workspacetasks: task identity required: %w", ErrValidation)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(task.WorkspaceID, task.ID)
	if _, exists := s.tasks[key]; exists {
		return nil, fmt.Errorf("workspacetasks: duplicate task: %w", ErrStateConflict)
	}
	now := s.clock()
	stored := copyTask(task)
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.tasks[key] = stored
	return copyTask(stored), nil
}

func (s *InMemoryStore) GetTask(ctx context.Context, workspaceID, taskID string) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, ErrNotFound
	}
	return copyTask(stored), nil
}

func (s *InMemoryStore) ListTasks(ctx context.Context, workspaceID string, limit, offset int) ([]*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []*WorkspaceTask
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
		return ErrNotFound
	}
	delete(s.tasks, key)
	delete(s.leases, key)
	return nil
}

// ClaimTask fences one pending task for the worker: pending -> running
// with the lease token. The lease is tracked separately from the envelope
// execution token (mirroring the postgres lease_token column, which claim
// sets and terminal/retry flips clear without touching the envelope). It
// mirrors the postgres sibling's transition guards: unknown ids stay
// ErrNotFound, claimed/terminal rows converge to ErrStateConflict.
func (s *InMemoryStore) ClaimTask(ctx context.Context, workspaceID, taskID, leaseToken string) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := taskKey(workspaceID, taskID)
	stored, ok := s.tasks[key]
	if !ok {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(leaseToken) == "" {
		return nil, fmt.Errorf("workspacetasks: lease token required: %w", ErrValidation)
	}
	if stored.Status != StatusPending {
		return nil, ErrStateConflict
	}
	if _, held := s.leases[key]; held {
		return nil, ErrStateConflict
	}
	now := s.clock()
	stored.Status = StatusRunning
	s.leases[key] = strings.TrimSpace(leaseToken)
	stored.UpdatedAt = now
	return copyTask(stored), nil
}

func (s *InMemoryStore) releaseLeaseLocked(workspaceID, taskID string) {
	delete(s.leases, taskKey(workspaceID, taskID))
}

func (s *InMemoryStore) CancelTask(ctx context.Context, workspaceID, taskID string, actorUserID int64, reason string) (*WorkspaceTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, false, ErrNotFound
	}
	if stored.IsTerminal() {
		return copyTask(stored), true, nil
	}
	now := s.clock()
	stored.Status = StatusCancelled
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

func (s *InMemoryStore) AddOutputs(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, ErrNotFound
	}
	if len(stored.OutputRefs)+len(outputs) > MaxOutputsPerTask {
		return nil, newValidationError(FieldError{Field: "outputs", Message: "too_many"})
	}
	stored.OutputRefs = append(append([]OutputRef(nil), stored.OutputRefs...), outputs...)
	stored.UpdatedAt = s.clock()
	return copyTask(stored), nil
}

func (s *InMemoryStore) TransitionForRetry(ctx context.Context, workspaceID, taskID, newToken string) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, ErrStateConflict
	}
	if strings.TrimSpace(newToken) == "" {
		return nil, fmt.Errorf("workspacetasks: execution token required: %w", ErrValidation)
	}
	stored.RetryCount++
	stored.ExecutionToken = newToken
	stored.Status = StatusPending
	stored.UpdatedAt = s.clock()
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), nil
}

func (s *InMemoryStore) CompleteTask(ctx context.Context, workspaceID, taskID string, outputs []OutputRef) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, ErrStateConflict
	}
	now := s.clock()
	stored.Status = StatusCompleted
	stored.OutputRefs = append([]OutputRef(nil), outputs...)
	completed := now
	stored.CompletedAt = &completed
	stored.UpdatedAt = now
	s.releaseLeaseLocked(workspaceID, taskID)
	return copyTask(stored), nil
}

func (s *InMemoryStore) FailTask(ctx context.Context, workspaceID, taskID, errorMsg string) (*WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.tasks[taskKey(workspaceID, taskID)]
	if !ok {
		return nil, ErrNotFound
	}
	if stored.IsTerminal() {
		return nil, ErrStateConflict
	}
	now := s.clock()
	stored.Status = StatusFailed
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
