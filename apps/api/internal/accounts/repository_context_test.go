package accounts

import (
	"context"
	"errors"
	"testing"

	"api/internal/tasks"
)

func TestTaskRepositoryCanceledContextAffectsQueryAndWrite(t *testing.T) {
	db := newQueueServiceTestDB(t)
	repo := NewTaskRepository(db)

	if err := repo.Create(context.Background(), &tasks.Task{
		TaskID:    "ctx-cancel-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.GetByTaskID(ctx, "ctx-cancel-task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByTaskID error=%v, want context.Canceled", err)
	}

	_, err := repo.UpdateByTaskIDAndStatuses(ctx, "ctx-cancel-task", []string{tasks.StatusPending}, map[string]interface{}{
		"status": tasks.StatusRunning,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UpdateByTaskIDAndStatuses error=%v, want context.Canceled", err)
	}
}
