// Package retention owns the task data retention boundary: terminal tasks
// older than the retention window are purged after their statistics are
// frozen. Financial facts and statistics aggregates are never deleted.
package retention

import (
	"context"
	"fmt"
	"time"

	retentioncontracts "ideaboard/api/internal/tasking/ports/retention"
)

const (
	// DefaultRetentionDays matches the admin observability window: the
	// task monitor only looks back 7 days, so older raw rows have no readers.
	DefaultRetentionDays = 7
	// DefaultBatchSize bounds one purge pass; the store loops passes until
	// a pass deletes less than a full batch.
	DefaultBatchSize = 1000
)

// PurgeResult is the application-facing retention report.
type PurgeResult = retentioncontracts.PurgedTaskStats

type Store interface {
	PurgeExpiredTerminalTasks(ctx context.Context, cutoff time.Time, batchSize int32) (retentioncontracts.PurgedTaskStats, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// PurgeExpiredTasks deletes terminal tasks whose terminal timestamp is older
// than retentionDays. Only terminal statuses are eligible; pending/running
// rows are never purged no matter their age.
func (s *Service) PurgeExpiredTasks(ctx context.Context, retentionDays int) (PurgeResult, error) {
	if retentionDays <= 0 {
		return PurgeResult{}, fmt.Errorf("retention days must be positive")
	}
	cutoff := s.now().AddDate(0, 0, -retentionDays)
	stats, err := s.store.PurgeExpiredTerminalTasks(ctx, cutoff, DefaultBatchSize)
	if err != nil {
		return PurgeResult{}, err
	}
	return stats, nil
}
