package accounts

import (
	"context"
	"fmt"

	"api/internal/runtime/queuetypes"
	"api/internal/tasks"
)

func (s *AccountService) GetMonitorInfo(ctx context.Context) (*queuetypes.MonitorInfo, error) {
	if s.monitorStore == nil {
		return nil, fmt.Errorf("monitor store not configured")
	}
	accounts, err := s.accountRepo.GetAllEnabled(ctx)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]int64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}
	return s.monitorStore.MonitorInfo(ctx, accountIDs)
}

func (s *AccountService) GetTaskStats(ctx context.Context) (map[string]int64, error) {
	pending, _ := s.taskRepo.CountByStatus(ctx, tasks.StatusPending)
	running, _ := s.taskRepo.CountByStatus(ctx, tasks.StatusRunning)
	completed, _ := s.taskRepo.CountByStatus(ctx, tasks.StatusCompleted)
	failed, _ := s.taskRepo.CountByStatus(ctx, tasks.StatusFailed)

	return map[string]int64{
		tasks.StatusPending:   pending,
		tasks.StatusRunning:   running,
		tasks.StatusCompleted: completed,
		tasks.StatusFailed:    failed,
	}, nil
}
