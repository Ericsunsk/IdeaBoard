package application

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/accounts/ports"
	providerendpointsdomain "ideaboard/api/internal/accountpool/providerendpoints/domain"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
	tasks "ideaboard/api/internal/tasking/domain"
)

const taskCancelTTL = 24 * time.Hour

type Service struct {
	accountRepo      ports.AccountRepository
	taskRepo         ports.AccountTaskStore
	queueStats       ports.QueueStatsStore
	accountLoadStore ports.AccountLoadStore
	concurrencyStore ports.AccountConcurrencyStore
	cancelStore      ports.TaskCancellationStore
	monitorStore     ports.TaskMonitorStore
	taskEvents       events.TaskStatusPublisher
}

type ServiceDeps struct {
	AccountRepo      ports.AccountRepository
	TaskRepo         ports.AccountTaskStore
	QueueStats       ports.QueueStatsStore
	AccountLoadStore ports.AccountLoadStore
	ConcurrencyStore ports.AccountConcurrencyStore
	CancelStore      ports.TaskCancellationStore
	MonitorStore     ports.TaskMonitorStore
}

type ServiceOptions struct {
	TaskEvents events.TaskStatusPublisher
}

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return &Service{
		accountRepo:      deps.AccountRepo,
		taskRepo:         deps.TaskRepo,
		queueStats:       deps.QueueStats,
		accountLoadStore: deps.AccountLoadStore,
		concurrencyStore: deps.ConcurrencyStore,
		cancelStore:      deps.CancelStore,
		monitorStore:     deps.MonitorStore,
		taskEvents:       opts.TaskEvents,
	}
}

func (s *Service) CreateAccount(ctx context.Context, req *domain.CreateAccountRequest) (*domain.ApiAccount, error) {
	if req == nil {
		return nil, validation.New("请求参数错误")
	}
	req.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(req.MaxConcurrent)
	req.Timeout = accountlimits.NormalizeTimeoutSeconds(req.Timeout)
	if req.Weight <= 0 {
		req.Weight = 1
	}
	protocol, capabilities, err := s.accountRepo.ProviderEndpointConfig(ctx, req.ProviderEndpointID)
	if err != nil {
		return nil, fmt.Errorf("ProviderEndpoint 不存在")
	}
	var models []domain.AccountModel
	if len(req.Models) > 0 || !providerendpointsdomain.AllowsModellessAccounts(protocol, capabilities) {
		models, err = domain.NormalizeModelInputs(req.Models)
		if err != nil {
			return nil, err
		}
	}

	account := &domain.ApiAccount{
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

func (s *Service) GetAccount(ctx context.Context, id int64) (*domain.ApiAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if database.IsNotFound(err) {
		return nil, fmt.Errorf("%w", domain.ErrAccountNotFound)
	}
	return account, err
}

func (s *Service) ListAccounts(ctx context.Context, req *domain.AccountListRequest) ([]*domain.AccountWithLoad, int64, error) {
	if req == nil {
		req = &domain.AccountListRequest{}
	}
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

	loadInfo := s.fetchLoads(ctx, accounts)
	waitQueueCount := s.fetchQueueLength(ctx)
	result := make([]*domain.AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent := 0
		if load, ok := loadInfo[acc.ID]; ok && load != nil {
			concurrent = load.CurrentConcurrent
		}

		result[i] = &domain.AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    waitQueueCount,
			RunningStatus:     domain.ResolveRunningStatus(acc.Status, acc.FailCount),
		}
	}

	return result, total, nil
}

func (s *Service) fetchLoads(ctx context.Context, accounts []*domain.ApiAccount) map[int64]*tasks.AccountLoad {
	if s.accountLoadStore == nil || len(accounts) == 0 {
		return nil
	}
	accountIDs := make([]int64, len(accounts))
	for i := range accounts {
		accountIDs[i] = accounts[i].ID
	}
	loads, err := s.accountLoadStore.GetAccountLoadInfo(ctx, accountIDs)
	if err != nil {
		slog.Warn("load account concurrency info failed", "err", err)
		return nil
	}
	return loads
}

func (s *Service) fetchQueueLength(ctx context.Context) int {
	if s.queueStats == nil {
		return 0
	}
	queued, _, err := s.queueStats.QueueLengths(ctx)
	if err != nil {
		slog.Warn("load queue lengths failed", "err", err)
		return 0
	}
	return int(queued)
}

func (s *Service) UpdateAccount(ctx context.Context, id int64, req *domain.UpdateAccountRequest) error {
	if req == nil {
		return validation.New("请求参数错误")
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w", domain.ErrAccountNotFound)
	}
	if err := applyAccountUpdateFields(account, req); err != nil {
		return err
	}
	replaceModels, err := s.applyModelUpdate(ctx, account, req.Models)
	if err != nil {
		return err
	}
	if err := s.persistUpdate(ctx, account, replaceModels); err != nil {
		return err
	}
	return nil
}

func applyAccountUpdateFields(account *domain.ApiAccount, req *domain.UpdateAccountRequest) error {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return validation.New("name 不能为空")
		}
		account.Name = name
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
		account.Weight = domain.NormalizeWeight(*req.Weight)
	}
	if req.Secret != nil && *req.Secret != "" {
		account.ApiKey = *req.Secret
	}
	return nil
}

func (s *Service) applyModelUpdate(ctx context.Context, account *domain.ApiAccount, inputs *[]domain.AccountModelInput) (bool, error) {
	if inputs == nil {
		return false, nil
	}
	protocol, capabilities, err := s.accountRepo.ProviderEndpointConfig(ctx, account.ProviderEndpointID)
	if err != nil {
		return false, err
	}
	models := []domain.AccountModel(nil)
	if len(*inputs) > 0 || !providerendpointsdomain.AllowsModellessAccounts(protocol, capabilities) {
		models, err = domain.NormalizeModelInputs(*inputs)
		if err != nil {
			return false, err
		}
	}
	account.Models = models
	return true, nil
}

func (s *Service) persistUpdate(ctx context.Context, account *domain.ApiAccount, replaceModels bool) error {
	var err error
	if replaceModels {
		err = s.accountRepo.Update(ctx, account)
	} else {
		err = s.accountRepo.UpdateFields(ctx, account)
	}
	if database.IsNotFound(err) {
		return fmt.Errorf("%w", domain.ErrAccountNotFound)
	}
	return err
}

func (s *Service) PatchAccountModel(ctx context.Context, accountID int64, req *domain.PatchAccountModelRequest) (*domain.ApiAccount, error) {
	if req == nil {
		return nil, validation.New("请求参数错误")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("%w", domain.ErrAccountNotFound)
	}
	modelName := strings.TrimSpace(req.Name)
	if modelName == "" {
		return nil, validation.New("model name 不能为空")
	}

	model, exists := domain.FindModelByName(account.Models, modelName)
	if !exists {
		model = domain.AccountModel{
			Name:      modelName,
			Enabled:   true,
			Weight:    1,
			SortOrder: len(account.Models),
		}
	}
	model.Name = modelName
	if err := applyModelPatchFields(&model, req); err != nil {
		return nil, err
	}

	if err := s.accountRepo.UpsertModel(ctx, accountID, model); err != nil {
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("%w", domain.ErrAccountNotFound)
		}
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *Service) DeleteAccountModel(ctx context.Context, accountID int64, modelName string) (*domain.ApiAccount, error) {
	if _, err := s.accountRepo.GetByID(ctx, accountID); err != nil {
		return nil, fmt.Errorf("%w", domain.ErrAccountNotFound)
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, validation.New("model name 不能为空")
	}
	if err := s.accountRepo.DeleteModel(ctx, accountID, modelName); err != nil {
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("%w", domain.ErrAccountModelNotFound)
		}
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, accountID)
}

func (s *Service) DeleteAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.ArchiveAndFinalizeTasks(ctx, id, "账号已删除")
	if err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", domain.ErrAccountNotFound)
		}
		return err
	}
	s.handleTaskSideEffects(ctx, id, failedTaskIDs)
	return nil
}

func (s *Service) StartAccount(ctx context.Context, id int64) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w", domain.ErrAccountNotFound)
	}

	account.MaxConcurrent = accountlimits.NormalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(ctx, id, domain.AccountStatusEnabled); err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", domain.ErrAccountNotFound)
		}
		return err
	}

	return nil
}

func (s *Service) StopAccount(ctx context.Context, id int64) error {
	failedTaskIDs, err := s.accountRepo.StopAndFinalizeTasks(ctx, id, "账号已停用")
	if err != nil {
		if database.IsNotFound(err) {
			return fmt.Errorf("%w", domain.ErrAccountNotFound)
		}
		return err
	}
	s.handleTaskSideEffects(ctx, id, failedTaskIDs)
	return nil
}

func (s *Service) handleTaskSideEffects(ctx context.Context, accountID int64, failedTaskIDs []string) {
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

func (s *Service) GetQueueInfo(ctx context.Context, id int64) (concurrent int, maxConcurrent int, err error) {
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

func (s *Service) GetMonitorInfo(ctx context.Context) (*tasks.MonitorInfo, error) {
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

func (s *Service) publishTaskStatusEvent(ctx context.Context, taskID, status string) {
	events.PublishTaskStatusLogWarn(ctx, s.taskEvents, taskID, status)
}

func applyModelPatchFields(model *domain.AccountModel, req *domain.PatchAccountModelRequest) error {
	if req.Enabled != nil {
		model.Enabled = *req.Enabled
	}
	if req.Weight != nil {
		if *req.Weight <= 0 {
			return validation.New("model weight 必须大于 0")
		}
		model.Weight = *req.Weight
	}
	if err := applyMaxConcurrentPatch(model, req.MaxConcurrent); err != nil {
		return err
	}
	if req.SortOrder != nil {
		if *req.SortOrder < 0 {
			return validation.New("model sort_order 不能小于 0")
		}
		model.SortOrder = *req.SortOrder
	}
	if req.Supports2K != nil {
		model.Supports2K = *req.Supports2K
	}
	if req.Supports4K != nil {
		model.Supports4K = *req.Supports4K
	}
	return nil
}

func applyMaxConcurrentPatch(model *domain.AccountModel, patch domain.NullableIntPatch) error {
	if !patch.Set {
		return nil
	}
	if !patch.Valid {
		model.MaxConcurrent = nil
		return nil
	}
	if patch.Value <= 0 {
		return validation.New("model max_concurrent 必须大于 0")
	}
	value := patch.Value
	model.MaxConcurrent = &value
	return nil
}
