package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
)

type fakeStore struct {
	cutoff    time.Time
	batchSize int32
	stats     retentioncontracts.PurgedTaskStats
	err       error
	calls     int
}

func (f *fakeStore) PurgeExpiredTerminalTasks(_ context.Context, cutoff time.Time, batchSize int32) (retentioncontracts.PurgedTaskStats, error) {
	f.calls++
	f.cutoff = cutoff
	f.batchSize = batchSize
	return f.stats, f.err
}

func TestPurgeExpiredTasksUsesRetentionWindow(t *testing.T) {
	store := &fakeStore{stats: retentioncontracts.PurgedTaskStats{TasksDeleted: 3}}
	service := NewService(store)
	fixedNow := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	result, err := service.PurgeExpiredTasks(context.Background(), 7)
	if err != nil {
		t.Fatalf("PurgeExpiredTasks() error = %v", err)
	}
	if result.TasksDeleted != 3 {
		t.Fatalf("TasksDeleted = %d, want 3", result.TasksDeleted)
	}
	wantCutoff := fixedNow.AddDate(0, 0, -7)
	if !store.cutoff.Equal(wantCutoff) {
		t.Fatalf("cutoff = %v, want %v", store.cutoff, wantCutoff)
	}
	if store.batchSize != DefaultBatchSize {
		t.Fatalf("batchSize = %d, want %d", store.batchSize, DefaultBatchSize)
	}
}

func TestPurgeExpiredTasksRejectsNonPositiveWindow(t *testing.T) {
	service := NewService(&fakeStore{})
	for _, days := range []int{0, -1} {
		if _, err := service.PurgeExpiredTasks(context.Background(), days); err == nil {
			t.Fatalf("PurgeExpiredTasks(%d) error = nil, want error", days)
		}
	}
}

func TestPurgeExpiredTasksPropagatesStoreError(t *testing.T) {
	wantErr := errors.New("purge failed")
	service := NewService(&fakeStore{err: wantErr})
	if _, err := service.PurgeExpiredTasks(context.Background(), 7); !errors.Is(err, wantErr) {
		t.Fatalf("PurgeExpiredTasks() error = %v, want %v", err, wantErr)
	}
}
