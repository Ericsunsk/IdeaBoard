package taskstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
)

func TestRepositoryCanceledContextAffectsQueryAndWrite(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)

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

	_, err := repo.MarkTaskFailed(ctx, "ctx-cancel-task", []string{tasks.StatusPending}, "context canceled", "", time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("MarkTaskFailed error=%v, want context.Canceled", err)
	}
}
