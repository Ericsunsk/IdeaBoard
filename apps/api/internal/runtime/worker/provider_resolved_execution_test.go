package worker

import (
	"context"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/providerendpoints"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func TestResolveProviderExecutionUsesAssignmentSnapshotAfterLiveConfigChanges(t *testing.T) {
	account := &accounts.RuntimeAccountAccess{ID: 17, Name: "live-account-name", APIKey: "secret", Timeout: 45}
	assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/v1", workflow.ProviderGoogleAI, providerendpoints.ProtocolModelAPI, "models/frozen-image")
	task := resolvedExecutionTask(t, workflow.FeatureSlugGeneration)

	exec, err := (&ProviderExecutor{}).resolveProviderExecution(context.Background(), account, task, assignment)
	if err != nil {
		t.Fatalf("resolveProviderExecution: %v", err)
	}
	if exec.providerEndpointID != assignment.ProviderEndpointID ||
		exec.providerEndpointName != assignment.ProviderEndpointName ||
		exec.providerEndpointBaseURL != assignment.ProviderEndpointBaseURL ||
		exec.apiType != assignment.APIType ||
		exec.protocol != assignment.ProviderEndpointProtocol ||
		exec.resolvedModel != assignment.ResolvedModel {
		t.Fatalf("resolved execution did not preserve assignment snapshot: %+v", exec)
	}
	if exec.timeout.Seconds() != 45 || exec.accountName != account.Name || exec.apiKey != account.APIKey {
		t.Fatalf("resolved live access facts = name:%q timeout:%s", exec.accountName, exec.timeout)
	}
	if exec.request.Model != assignment.ResolvedModel {
		t.Fatalf("provider request model = %q, want frozen assignment model", exec.request.Model)
	}
}

func TestResolveProviderExecutionRejectsIncompleteSnapshotBeforeMediaIO(t *testing.T) {
	mediaCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mediaCalls++
	}))
	defer server.Close()

	useMask := true
	task := &tasks.TaskPayload{TaskID: "incomplete-snapshot", Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugInpaint,
		Media: &tasks.MediaInputs{
			Original: &tasks.MediaRef{AssetID: 1, Role: "original"},
			Mask:     &tasks.MediaRef{AssetID: 2, Role: "mask"},
			UseMask:  &useMask,
		},
	})}
	assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/v1", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolModelAPI, "gpt-image-1")
	assignment.ProviderEndpointName = ""

	_, err := (&ProviderExecutor{}).resolveProviderExecution(
		context.Background(),
		&accounts.RuntimeAccountAccess{ID: 17, Name: "account", APIKey: "secret"},
		task,
		assignment,
	)
	if err == nil || !strings.Contains(err.Error(), "snapshot is incomplete") {
		t.Fatalf("error = %v, want incomplete snapshot", err)
	}
	if mediaCalls != 0 {
		t.Fatalf("media calls = %d, want zero", mediaCalls)
	}
}

func TestResolveProviderExecutionEnforcesSnapshotModelPolicy(t *testing.T) {
	executor := &ProviderExecutor{mediaResolver: testRuntimeMediaResolver{urls: map[int64]string{1: "https://media.example/input.png", 2: "https://media.example/mask.png"}}}
	account := &accounts.RuntimeAccountAccess{ID: 17, Name: "account", APIKey: "secret"}

	t.Run("model required", func(t *testing.T) {
		assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/v1", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolModelAPI, "")
		_, err := executor.resolveProviderExecution(context.Background(), account, resolvedExecutionTask(t, workflow.FeatureSlugGeneration), assignment)
		if err == nil || !strings.Contains(err.Error(), "未分配模型") {
			t.Fatalf("error = %v, want missing model", err)
		}
	})

	t.Run("modelless transform", func(t *testing.T) {
		assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/cutout", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolImagePipeline, "")
		exec, err := executor.resolveProviderExecution(context.Background(), account, resolvedExecutionTask(t, workflow.FeatureSlugCutout), assignment)
		if err != nil {
			t.Fatalf("resolve modelless execution: %v", err)
		}
		if exec.resolvedModel != "" || exec.request.Model != "" || exec.adapter != providerRuntimeAdapterCutout {
			t.Fatalf("modelless execution = %+v", exec)
		}

		assignment.ResolvedModel = "unexpected-model"
		if _, err := executor.resolveProviderExecution(context.Background(), account, resolvedExecutionTask(t, workflow.FeatureSlugCutout), assignment); err == nil || !strings.Contains(err.Error(), "modelless") {
			t.Fatalf("error = %v, want modelless model rejection", err)
		}
	})
}

func TestResolveProviderExecutionRejectsProtocolURLAndAPITypeBeforeProviderIO(t *testing.T) {
	executor := &ProviderExecutor{}
	account := &accounts.RuntimeAccountAccess{ID: 17, Name: "account", APIKey: "secret"}
	task := resolvedExecutionTask(t, workflow.FeatureSlugGeneration)

	t.Run("protocol operation mismatch", func(t *testing.T) {
		assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/transform", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolImagePipeline, "gpt-image-1")
		if _, err := executor.resolveProviderExecution(context.Background(), account, task, assignment); err == nil || !strings.Contains(err.Error(), "does not support runtime operation") {
			t.Fatalf("error = %v, want protocol mismatch", err)
		}
	})

	t.Run("unsafe URL", func(t *testing.T) {
		assignment := resolvedExecutionAssignment("http://169.254.169.254/latest/meta-data", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolModelAPI, "gpt-image-1")
		if _, err := executor.resolveProviderExecution(context.Background(), account, task, assignment); err == nil || !strings.Contains(err.Error(), "URL 不可用") {
			t.Fatalf("error = %v, want unsafe URL rejection", err)
		}
	})

	t.Run("no API type inference", func(t *testing.T) {
		assignment := resolvedExecutionAssignment("https://generativelanguage.googleapis.com/v1beta", "", providerendpoints.ProtocolModelAPI, "gemini-image")
		if _, err := executor.resolveProviderExecution(context.Background(), account, task, assignment); err == nil || !strings.Contains(err.Error(), "snapshot is incomplete") {
			t.Fatalf("error = %v, want missing API type instead of URL inference", err)
		}
		assignment.APIType = "future-provider"
		if _, err := executor.resolveProviderExecution(context.Background(), account, task, assignment); err == nil || !strings.Contains(err.Error(), "API type is unsupported") {
			t.Fatalf("error = %v, want explicit unsupported API type", err)
		}
	})
}

func TestResolveProviderExecutionDoesNotMutateInputs(t *testing.T) {
	account := &accounts.RuntimeAccountAccess{ID: 17, Name: " account ", APIKey: " secret ", Timeout: 30}
	assignment := resolvedExecutionAssignment("http://127.0.0.1:18080/v1", workflow.ProviderOpenAICompatible, providerendpoints.ProtocolModelAPI, "gpt-image-1")
	task := resolvedExecutionTask(t, workflow.FeatureSlugGeneration)
	accountBefore := *account
	assignmentBefore := assignment
	taskBefore := *task

	exec, err := (&ProviderExecutor{}).resolveProviderExecution(context.Background(), account, task, assignment)
	if err != nil {
		t.Fatalf("resolveProviderExecution: %v", err)
	}
	adapterAccount := exec.adapterAccount()
	adapterAccount.ProxyURL = "https://mutated.example"
	adapterAccount.ApiKey = "mutated"

	if !reflect.DeepEqual(*account, accountBefore) || assignment != assignmentBefore || !reflect.DeepEqual(*task, taskBefore) {
		t.Fatalf("resolver mutated inputs: account=%+v assignment=%+v task=%+v", account, assignment, task)
	}
	if exec.providerEndpointBaseURL != assignmentBefore.ProviderEndpointBaseURL || exec.apiKey != strings.TrimSpace(accountBefore.APIKey) {
		t.Fatalf("adapter mutation escaped resolved execution: %+v", exec)
	}
}

func resolvedExecutionAssignment(baseURL string, apiType string, protocol string, resolvedModel string) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{
		AccountID:                17,
		ProviderEndpointID:       23,
		ProviderEndpointName:     "snapshot-endpoint",
		ProviderEndpointBaseURL:  baseURL,
		ProviderEndpointProtocol: protocol,
		APIType:                  apiType,
		ResolvedModel:            resolvedModel,
	}
}

func resolvedExecutionTask(t *testing.T, feature string) *tasks.TaskPayload {
	t.Helper()
	params := &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Feature: feature}
	switch feature {
	case workflow.FeatureSlugCutout, workflow.FeatureSlugUpscale:
		params.Media = &tasks.MediaInputs{Inputs: []tasks.MediaRef{{AssetID: 1, Role: "input"}}}
	}
	return &tasks.TaskPayload{
		TaskID: "resolved-execution-" + feature,
		Prompt: "draw",
		Params: marshalWorkflowTaskParamsForTest(t, params),
	}
}
