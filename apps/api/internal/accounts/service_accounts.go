package accounts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

func (s *AccountService) CreateAccount(ctx context.Context, req *CreateAccountRequest) (*ApiAccount, error) {
	proxyURL, err := outbound.ValidateConfiguredEndpoint(req.ProxyURL, outbound.AllowHostsFromEnv()...)
	if err != nil {
		return nil, fmt.Errorf("proxy_url 不可用，仅支持可达的公网 http(s) 地址")
	}
	req.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	req.Timeout = NormalizeAccountTimeout(req.Timeout)
	if req.Weight <= 0 {
		req.Weight = 1
	}
	if strings.TrimSpace(req.ModelSet) == "" {
		return nil, fmt.Errorf("model_set 不能为空，请先检测并选择模型")
	}
	apiType, ok := workflow.ValidateProviderAPIType(req.ApiType, proxyURL)
	if !ok {
		return nil, fmt.Errorf("api_type 不支持")
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = apiType
	}

	account := &ApiAccount{
		Name:          req.Name,
		ChannelID:     req.ChannelID,
		Platform:      platform,
		ApiType:       apiType,
		ModelSet:      req.ModelSet,
		ApiKey:        req.ApiKey,
		ProxyURL:      proxyURL,
		MaxConcurrent: req.MaxConcurrent,
		Timeout:       req.Timeout,
		ExtraConfig:   req.ExtraConfig,
		Weight:        req.Weight,
		Status:        1,
	}

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *AccountService) GetAccount(ctx context.Context, id int64) (*ApiAccount, error) {
	return s.accountRepo.GetByID(ctx, id)
}

func (s *AccountService) ListAccounts(ctx context.Context, req *AccountListRequest) ([]*AccountWithLoad, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	accounts, total, err := s.accountRepo.List(ctx, req.Keyword, req.Platform, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}

	accountIDs := make([]int64, len(accounts))
	for i := range accounts {
		accountIDs[i] = accounts[i].ID
	}
	loadInfo := map[int64]*queuetypes.AccountLoad{}
	if s.accountLoadStore != nil {
		loads, loadErr := s.accountLoadStore.GetAccountLoadInfo(ctx, accountIDs)
		if loadErr != nil {
			slog.Warn("load account concurrency info failed", "err", loadErr)
		} else {
			loadInfo = loads
		}
	}
	waitQueueCount := 0
	if s.queueStats != nil {
		queued, _, waitErr := s.queueStats.QueueLengths(ctx)
		if waitErr != nil {
			slog.Warn("load queue lengths failed", "err", waitErr)
		} else {
			waitQueueCount = int(queued)
		}
	}
	result := make([]*AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent := 0
		if load, ok := loadInfo[acc.ID]; ok && load != nil {
			concurrent = load.CurrentConcurrent
		}

		status := "运行中"
		if acc.Status == 0 {
			status = "已停止"
		} else if acc.FailCount >= MaxFailCount {
			status = "故障"
		}

		result[i] = &AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    waitQueueCount,
			RunningStatus:     status,
		}
	}

	return result, total, nil
}

func (s *AccountService) UpdateAccount(ctx context.Context, id int64, req *UpdateAccountRequest) error {
	proxyURL, err := outbound.ValidateConfiguredEndpoint(req.ProxyURL, outbound.AllowHostsFromEnv()...)
	if err != nil {
		return fmt.Errorf("proxy_url 不可用，仅支持可达的公网 http(s) 地址")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.Name = req.Name
	if req.ChannelID > 0 {
		account.ChannelID = req.ChannelID
	}
	if strings.TrimSpace(req.ModelSet) == "" {
		return fmt.Errorf("model_set 不能为空，请先检测并选择模型")
	}
	apiType, ok := workflow.ValidateProviderAPIType(req.ApiType, proxyURL)
	if !ok {
		return fmt.Errorf("api_type 不支持")
	}
	account.ApiType = apiType
	if strings.TrimSpace(req.Platform) == "" {
		account.Platform = account.ApiType
	} else {
		account.Platform = req.Platform
	}
	account.ModelSet = req.ModelSet
	account.ProxyURL = proxyURL
	account.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	account.Timeout = NormalizeAccountTimeout(req.Timeout)
	account.ExtraConfig = req.ExtraConfig
	account.Weight = req.Weight
	account.Status = req.Status

	if req.ApiKey != "" {
		account.ApiKey = req.ApiKey
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return err
	}

	return nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}
	if account.Status != 0 {
		return fmt.Errorf("请先停用账号后再删除")
	}
	return s.accountRepo.Delete(ctx, id)
}

func (s *AccountService) StartAccount(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.MaxConcurrent = normalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(ctx, id, 1); err != nil {
		return err
	}

	return nil
}

func (s *AccountService) StopAccount(ctx context.Context, id int64) error {
	taskIDs, err := s.taskRepo.ListTaskIDsByAccountIDAndStatuses(ctx, id, tasks.StatusPending, tasks.StatusRunning)
	if err != nil {
		slog.Warn("list account running tasks failed", "account_id", id, "err", err)
	} else {
		if s.cancelStore == nil {
			if len(taskIDs) > 0 {
				slog.Warn("task cancellation store not configured", "account_id", id, "task_count", len(taskIDs))
			}
		} else {
			for _, taskID := range taskIDs {
				if cancelErr := s.cancelStore.MarkCancelled(ctx, taskID, taskCancelTTL); cancelErr != nil {
					slog.Warn("mark task cancellation flag failed", "account_id", id, "task_id", taskID, "err", cancelErr)
				}
			}
		}
		if len(taskIDs) > 0 {
			slog.Info("marked account tasks cancelled", "account_id", id, "task_count", len(taskIDs))
		}
	}

	// 1. 将该账号下所有 running/pending 的任务标记为 failed
	failedTaskIDs, err := s.taskRepo.FailRunningByAccountID(ctx, id, "账号已停用")
	if err != nil {
		slog.Warn("mark running tasks failed by account failed", "account_id", id, "err", err)
	} else if len(failedTaskIDs) > 0 {
		slog.Info("marked running tasks failed by account", "account_id", id, "task_count", len(failedTaskIDs))
		for _, taskID := range failedTaskIDs {
			s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
		}
	}

	return s.accountRepo.UpdateStatus(ctx, id, 0)
}

func (s *AccountService) GetAccountQueueInfo(ctx context.Context, id int64) (concurrent int, maxConcurrent int, err error) {
	if s.concurrencyStore == nil {
		return 0, 0, fmt.Errorf("account concurrency store not configured")
	}
	concurrent, err = s.concurrencyStore.GetAccountConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	maxConcurrent, err = s.concurrencyStore.GetAccountMaxConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return concurrent, maxConcurrent, nil
}
