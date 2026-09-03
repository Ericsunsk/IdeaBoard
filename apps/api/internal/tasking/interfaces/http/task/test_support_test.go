package taskhttp

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/prompts"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

type Service = taskapp.Service
type ServiceDeps = taskapp.ServiceDeps
type ServiceOptions = taskapp.ServiceOptions
type AccountStore = taskapp.AccountStore
type TaskSubmissionStore = taskapp.TaskSubmissionStore
type FeaturePromptStore = taskapp.FeaturePromptStore
type BillingReadiness = taskapp.BillingReadiness
type CanonicalTaskRequest = taskapp.CanonicalTaskRequest
type TaskIntakeDeps = taskapp.TaskIntakeDeps

func NewService(deps ServiceDeps, opts ServiceOptions) *Service {
	return taskapp.NewService(deps, opts)
}
func NewTaskIntake(deps taskapp.TaskIntakeDeps) taskapp.TaskIntake {
	return taskapp.NewTaskIntake(deps)
}

type testSubmitMediaValidator struct{}

func (testSubmitMediaValidator) ValidateSubmitMediaRef(context.Context, int64, string) error {
	return nil
}

type recordingGenerationMediaPreparer struct{}

func (p *recordingGenerationMediaPreparer) PrepareGenerationMedia(_ context.Context, _ int64, _ string, snapshot *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error) {
	prepared := *snapshot
	prepared.Media = append([]tasks.GenerationMediaIntent(nil), snapshot.Media...)
	for index := range prepared.Media {
		prepared.Media[index].AssetID = int64(index + 1)
		prepared.Media[index].Source = ""
	}
	return prepared, nil
}

type recordingTaskInputManifestBuilder struct{}

func (recordingTaskInputManifestBuilder) BuildTaskInputManifest(_ context.Context, _ int64, media *tasks.MediaInputs) (*tasks.TaskInputManifest, error) {
	if media == nil {
		return tasks.NewTaskInputManifest(nil, nil, nil, nil, nil)
	}
	ref := func(value tasks.MediaRef, suffix string) tasks.TaskInputManifestRef {
		return tasks.TaskInputManifestRef{AssetID: "asset_" + suffix, DerivativeID: "derivative_" + suffix, DerivativeTier: tasks.TierProcessingInput, StorageProfileID: "test", ObjectKey: "uploads/" + suffix + ".webp", ObjectGeneration: 1, ChecksumSHA256: strings.Repeat("a", 64), MIMEType: "image/webp", SizeBytes: 1, Role: value.Role, Weight: value.Weight}
	}
	inputs := make([]tasks.TaskInputManifestRef, 0, len(media.Inputs))
	for index, value := range media.Inputs {
		inputs = append(inputs, ref(value, strconv.Itoa(index+1)))
	}
	references := make([]tasks.TaskInputManifestRef, 0, len(media.References))
	for index, value := range media.References {
		references = append(references, ref(value, strconv.Itoa(100+index)))
	}
	return tasks.NewTaskInputManifest(inputs, references, nil, nil, nil)
}

type fakeFeaturePromptStore struct {
	configs map[string]prompts.FeaturePromptRuntimeConfig
	err     error
}

func (s fakeFeaturePromptStore) GetRuntimeConfig(_ context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error) {
	if s.err != nil {
		return prompts.FeaturePromptRuntimeConfig{}, s.err
	}
	if s.configs == nil {
		return prompts.FeaturePromptRuntimeConfig{UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug)}, nil
	}
	if config, ok := s.configs[slug]; ok {
		return config, nil
	}
	return prompts.FeaturePromptRuntimeConfig{UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug)}, nil
}

type recordingSubmissionStore struct {
	calls  []submitCall
	result tasks.TaskSubmissionResult
	err    error
}

type submitCall struct{ canonical CanonicalTaskRequest }

func (s *recordingSubmissionStore) SubmitTask(_ context.Context, canonical CanonicalTaskRequest) (tasks.TaskSubmissionResult, error) {
	if s.err != nil {
		return tasks.TaskSubmissionResult{}, s.err
	}
	if s.result.TaskID != "" {
		return s.result, nil
	}
	s.calls = append(s.calls, submitCall{canonical: canonical})
	return tasks.TaskSubmissionResult{TaskID: "test-task", Created: true}, nil
}

type adminTaskDetailStoreStub struct{ task *tasks.Task }

func (s adminTaskDetailStoreStub) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	return s.task, nil
}
func (s adminTaskDetailStoreStub) GetByTaskIDAndUserID(context.Context, string, int64) (*tasks.Task, error) {
	return s.task, nil
}
func (s adminTaskDetailStoreStub) List(context.Context, int64, int64, string, string, int, int) ([]*tasks.Task, tasks.TaskListCounts, error) {
	return nil, tasks.TaskListCounts{}, nil
}
func (s adminTaskDetailStoreStub) GetByUserID(context.Context, int64, int, int) ([]*tasks.Task, int64, error) {
	return nil, 0, nil
}
func (s adminTaskDetailStoreStub) MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error) {
	return 0, nil
}
func (s adminTaskDetailStoreStub) ListOutputsByTaskIDs(context.Context, []string) (map[string][]tasks.TaskOutput, error) {
	return map[string][]tasks.TaskOutput{}, nil
}

type fakeSubmitAccountStore struct {
	accounts []*accounts.ApiAccount
	err      error
}

func (s *fakeSubmitAccountStore) GetAllEnabled(context.Context) ([]*accounts.ApiAccount, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make([]*accounts.ApiAccount, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account == nil {
			continue
		}
		copyAccount := *account
		copyAccount.Capabilities = append([]string(nil), account.Capabilities...)
		hasEnabledModel := false
		for _, model := range account.Models {
			hasEnabledModel = hasEnabledModel || model.Enabled
		}
		if hasEnabledModel {
			hasGeneration := false
			for _, capability := range copyAccount.Capabilities {
				hasGeneration = hasGeneration || capability == "generation"
			}
			if !hasGeneration {
				copyAccount.Capabilities = append(copyAccount.Capabilities, "generation")
			}
		}
		result = append(result, &copyAccount)
	}
	return result, nil
}

type fakeCancelStore struct{}

func (fakeCancelStore) MarkCancelled(context.Context, string, time.Duration) error { return nil }
func (fakeCancelStore) IsCancelled(context.Context, string) (bool, error)          { return false, nil }
func (fakeCancelStore) ClearCancelled(context.Context, string) error               { return nil }

func createTaskServiceTestTask(t testing.TB, db *sql.DB, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.Params == "" || task.Params == "{}" {
		task.Params = `{"schema_version":5}`
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,display_name,storage_uid,role,status) VALUES($1,$2,$3,'user',1) ON CONFLICT(id) DO NOTHING`, task.UserID, fmt.Sprintf("taskhttp-fixture-user-%d", task.UserID), fmt.Sprintf("TH%04d", task.UserID%10000)); err != nil {
		return fmt.Errorf("seed task fixture user: %w", err)
	}
	if task.Status == tasks.StatusRunning && task.AccountID == nil {
		var endpointID, accountID, credentialID int64
		if err := db.QueryRowContext(ctx, `INSERT INTO provider_endpoints(api_type,name,base_url,status) VALUES('openai-compatible',$1,'https://example.com/taskhttp-fixture',1) RETURNING id`, "taskhttp-fixture-"+task.TaskID).Scan(&endpointID); err != nil {
			return err
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO api_accounts(name,provider_endpoint_id,max_concurrent,timeout,extra_config,status,weight) VALUES($1,$2,1,120,'',1,1) RETURNING id`, "taskhttp-fixture-"+task.TaskID, endpointID).Scan(&accountID); err != nil {
			return err
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO api_account_secrets(account_id,secret_material_kind,secret_payload,key_version,fingerprint,status) VALUES($1,'encrypted','fixture-secret','test-key',$2,'active') RETURNING id`, accountID, fmt.Sprintf("taskhttp-fixture-%d", accountID)).Scan(&credentialID); err != nil {
			return err
		}
		task.AccountID = &accountID
		task.AssignedProviderEndpointID = &endpointID
		task.AssignedCredentialID = &credentialID
		task.LeaseToken = "lease-" + task.TaskID
	}
	if err := repo.Create(ctx, task); err != nil {
		return fmt.Errorf("create task fixture: %w", err)
	}
	return nil
}

var _ AccountStore = (*fakeSubmitAccountStore)(nil)
var _ TaskSubmissionStore = (*recordingSubmissionStore)(nil)
var _ FeaturePromptStore = (*fakeFeaturePromptStore)(nil)
var _ taskmedia.Validator = testSubmitMediaValidator{}
