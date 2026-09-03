package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/testdb"
)

func TestRepositoryCanceledContextAffectsQueryAndWrite(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)

	if err := repo.Create(context.Background(), &tasks.Task{
		TaskID: "ctx-cancel-task",
		UserID: 1,
		Status: tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.GetByTaskID(ctx, "ctx-cancel-task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetByTaskID error=%v, want context.Canceled", err)
	}

	_, err := repo.convergePendingFailureForTest(ctx, "ctx-cancel-task", "context canceled", "", time.Now(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ConvergeTask error=%v, want context.Canceled", err)
	}
}
