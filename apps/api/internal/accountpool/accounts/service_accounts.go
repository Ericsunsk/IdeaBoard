package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/tasks"
)

var ErrAccountNotFound = errors.New("account not found")
var ErrAccountNameConflict = errors.New("account name conflict")
var ErrAccountModelNotFound = errors.New("account model not found")

func (s *AccountService) CreateAccount(ctx context.Context, req *CreateAccountRequest) (*ApiAccount, error) {
	req.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(req.MaxConcurrent)
	req.Timeout = accountlimits.NormalizeTimeoutSeconds(req.Timeout)
	if req.Weight <= 0 {
		req.Weight = 1
	}
	models, err := normalizeAccountModelInputs(req.Models)
	if err != nil {
		return nil, err
	}

	account := &ApiAccount{
		Name:               strings.TrimSpace(req.Name),
		ProviderEndpointID: req.ProviderEndpointID,
		Models:             models,
		ApiKey:             req.Secret,
		SecretConfigured:   strings.TrimSpace(req.Secret) != "",
		MaxConcurrent:      req.MaxConcurrent,
		Timeout:            req.Timeout,
		ExtraConfig:        req.ExtraConfig,
		Weight:             req.Weight,
		Status:             1,
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

	accounts, total, err := s.accountRepo.List(ctx, req.Keyword, req.ApiType, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}

	accountIDs := make([]int64, len(accounts))
	for i := range accounts {
		accountIDs[i] = accounts[i].ID
	}
	loadInfo := map[int64]*tasks.AccountLoad{}
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
	if req == nil {
		return fmt.Errorf("请求参数错误")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w", ErrAccountNotFound)
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return fmt.Errorf("name 不能为空")
		}
		account.Name = name
	}
	shouldReplaceModels := req.Models != nil
	if shouldReplaceModels {
		models, err := normalizeAccountModelInputs(*req.Models)
		if err != nil {
			return err
		}
		account.Models = models
	}
	if req.MaxConcurrent != nil {
		account.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(*req.MaxConcurrent)
	}
	if req.Timeout != nil {
		account.Timeout = accountlimits.NormalizeTimeoutSeconds(*req.Timeout)
	}
	if req.ExtraConfig != nil {
		account.ExtraConfig = *req.ExtraConfig
	}
	if req.Weight != nil {
		if *req.Weight <= 0 {
			account.Weight = 1
		} else {
			account.Weight = *req.Weight
		}
	}

	if req.Secret != nil && *req.Secret != "" {
		account.ApiKey = *req.Secret
	}

	var updateErr error
	if shouldReplaceModels {
		updateErr = s.accountRepo.Update(ctx, account)
	} else {
		updateErr = s.accountRepo.UpdateFields(ctx, account)
	}
	if updateErr != nil {
		if database.IsNotFound(updateErr) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return updateErr
	}

	return nil
}

func (s *AccountService) PatchAccountModel(ctx context.Context, accountID int64, req *PatchAccountModelRequest) (*ApiAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrAccountNotFound)
	}
	modelName := strings.TrimSpace(req.Name)
	if modelName == "" {
		return nil, fmt.Errorf("model name 不能为空")
	}

	model, exists := findAccountModelByName(account.Models, modelName)
	if !exists {
		model = AccountModel{
			Name:      modelName,
			Enabled:   true,
			Weight:    1,
			SortOrder: len(account.Models),
		}
	}
	model.Name = modelName
	if req.Enabled != nil {
		model.Enabled = *req.Enabled
	}
	if req.Weight != nil {
		if *req.Weight <= 0 {
			return nil, fmt.Errorf("model weight 必须大于 0")
		}
		model.Weight = *req.Weight
	}
	if req.MaxConcurrent.Set {
		if req.MaxConcurrent.Valid {
			if req.MaxConcurrent.Value <= 0 {
				return nil, fmt.Errorf("model max_concurrent 必须大于 0")
			}
			value := req.MaxConcurrent.Value
			model.MaxConcurrent = &value
		} else {
			model.MaxConcurrent = nil
		}
	}
	if req.SortOrder != nil {
		if *req.SortOrder < 0 {
			return nil, fmt.Errorf("model sort_order 不能小于 0")
		}
		model.SortOrder = *req.SortOrder
	}

	if err := s.accountRepo.UpsertModel(ctx, accountID, model); err != nil {
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("%w", ErrAccountNotFound)
		}
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *AccountService) DeleteAccountModel(ctx context.Context, accountID int64, modelName string) (*ApiAccount, error) {
	if _, err := s.accountRepo.GetByID(ctx, accountID); err != nil {
		return nil, fmt.Errorf("%w", ErrAccountNotFound)
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name 不能为空")
	}
	if err := s.accountRepo.DeleteModel(ctx, accountID, modelName); err != nil {
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("%w", ErrAccountModelNotFound)
		}
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func findAccountModelByName(models []AccountModel, modelName string) (AccountModel, bool) {
	modelKey := strings.ToLower(strings.TrimSpace(modelName))
	for _, model := range models {
		if strings.ToLower(strings.TrimSpace(model.Name)) == modelKey {
			return model, true
		}
	}
	return AccountModel{}, false
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
		models = append(models, AccountModel{
			Name:          name,
			Enabled:       enabled,
			Weight:        weight,
			MaxConcurrent: input.MaxConcurrent,
			SortOrder:     len(models),
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("models 不能为空，请先检测并选择模型")
	}
	return models, nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.ArchiveAccountAndFinalizeRunningTasks(ctx, id, "账号已删除")
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

	account.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(ctx, id, 1); err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}

	return nil
}

func (s *AccountService) StopAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.StopAccountAndFinalizeRunningTasks(ctx, id, "账号已停用")
	if err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", ErrAccountNotFound)
		}
		return err
	}
	s.handleAccountTaskSideEffects(ctx, id, failedTaskIDs)
	return nil
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
