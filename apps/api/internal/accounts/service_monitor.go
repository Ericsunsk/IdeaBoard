package accounts

import (
	"context"
	"fmt"

	"ideaboard/api/internal/runtime/queuetypes"
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
