package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"
)

var ErrAccountNotFound = errors.New("account not found")

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
	models, err := normalizeAccountModelInputs(req.Models)
	if err != nil {
		return nil, err
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
		Name:             req.Name,
		ChannelID:        req.ChannelID,
		Platform:         platform,
		ApiType:          apiType,
		Models:           models,
		ApiKey:           req.Secret,
		SecretConfigured: strings.TrimSpace(req.Secret) != "",
		ProxyURL:         proxyURL,
		MaxConcurrent:    req.MaxConcurrent,
		Timeout:          req.Timeout,
		ExtraConfig:      req.ExtraConfig,
		Weight:           req.Weight,
		Status:           1,
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
		return fmt.Errorf("%w", ErrAccountNotFound)
	}

	account.Name = req.Name
	models, err := normalizeAccountModelInputs(req.Models)
	if err != nil {
		return err
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
	account.Models = models
	account.ProxyURL = proxyURL
	account.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	account.Timeout = NormalizeAccountTimeout(req.Timeout)
	account.ExtraConfig = req.ExtraConfig
	if req.Weight <= 0 {
		account.Weight = 1
	} else {
		account.Weight = req.Weight
	}

	if req.Secret != "" {
		account.ApiKey = req.Secret
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}

	return nil
}

func normalizeAccountModelInputs(inputs []AccountModelInput) ([]AccountModel, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("models 不能为空，请先检测并选择模型")
	}
	seen := make(map[string]struct{}, len(inputs))
	models := make([]AccountModel, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		weight := input.Weight
		if weight <= 0 {
			weight = 1
		}
		featureSlugs, err := normalizeFeatureSlugs(input.FeatureSlugs)
		if err != nil {
			return nil, err
		}
		models = append(models, AccountModel{
			Name:          name,
			Enabled:       enabled,
			Weight:        weight,
			MaxConcurrent: input.MaxConcurrent,
			FeatureSlugs:  featureSlugs,
			SortOrder:     len(models),
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("models 不能为空，请先检测并选择模型")
	}
	return models, nil
}

func normalizeFeatureSlugs(slugs []string) ([]string, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(slugs))
	result := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		normalized, err := workflow.ValidateWorkflowFeatureSlug(slug)
		if err != nil {
			return nil, fmt.Errorf("feature_slugs 包含不支持的节点: %w", err)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.ArchiveAccountAndFinalizeRunningTasks(ctx, id, s.makeRunningTaskFinalizer(id, "账号已删除"))
	if err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}
	s.handleAccountTaskSideEffects(ctx, id, failedTaskIDs)
	return nil
}

func (s *AccountService) StartAccount(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w", ErrAccountNotFound)
	}

	account.MaxConcurrent = normalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(ctx, id, 1); err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}

	return nil
}

func (s *AccountService) StopAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.StopAccountAndFinalizeRunningTasks(ctx, id, s.makeRunningTaskFinalizer(id, "账号已停用"))
	if err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}
	s.handleAccountTaskSideEffects(ctx, id, failedTaskIDs)
	return nil
}

func (s *AccountService) makeRunningTaskFinalizer(accountID int64, reason string) RunningTaskFinalizerTx {
	return func(ctx context.Context, q *dbsqlc.Queries, completedAt time.Time) ([]string, error) {
		if s.taskRepo == nil {
			return nil, nil
		}
		return s.taskRepo.FailRunningByAccountIDTx(ctx, q, accountID, reason, completedAt)
	}
}

func (s *AccountService) handleAccountTaskSideEffects(ctx context.Context, accountID int64, failedTaskIDs []string) {
	taskIDs := append([]string{}, failedTaskIDs...)
	if s.taskRepo != nil {
		pendingTaskIDs, err := s.taskRepo.ListTaskIDsByAccountIDAndStatuses(ctx, accountID, tasks.StatusPending, tasks.StatusRunning)
		if err != nil {
			slog.Warn("list account pending tasks failed", "account_id", accountID, "err", err)
		} else {
			taskIDs = appendUniqueTaskIDs(taskIDs, pendingTaskIDs...)
		}
	}
	if len(taskIDs) > 0 {
		if s.cancelStore == nil {
			slog.Warn("task cancellation store not configured", "account_id", accountID, "task_count", len(taskIDs))
		} else {
			for _, taskID := range taskIDs {
				if cancelErr := s.cancelStore.MarkCancelled(ctx, taskID, taskCancelTTL); cancelErr != nil {
					slog.Warn("mark task cancellation flag failed", "account_id", accountID, "task_id", taskID, "err", cancelErr)
				}
			}
			slog.Info("marked account tasks cancelled", "account_id", accountID, "task_count", len(taskIDs))
		}
	}
	if len(failedTaskIDs) > 0 {
		slog.Info("marked running tasks failed by account", "account_id", accountID, "task_count", len(failedTaskIDs))
		for _, taskID := range failedTaskIDs {
			s.publishTaskStatusEvent(ctx, taskID, tasks.StatusFailed)
		}
	}
}

func appendUniqueTaskIDs(taskIDs []string, more ...string) []string {
	seen := make(map[string]struct{}, len(taskIDs)+len(more))
	result := make([]string, 0, len(taskIDs)+len(more))
	for _, taskID := range append(taskIDs, more...) {
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		result = append(result, taskID)
	}
	return result
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
