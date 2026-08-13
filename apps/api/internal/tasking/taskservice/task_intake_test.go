package taskservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestTaskIntakeCanonicalizesEveryAIImageWorkflowThroughItsPublicInterface(t *testing.T) {
	var intake = NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	tests := []struct {
		feature string
		images  []string
		params  map[string]interface{}
	}{
		{feature: workflow.FeatureSlugSketchToImage},
		{
			feature: workflow.FeatureSlugStyleVariation,
			params:  map[string]interface{}{"variation": map[string]interface{}{"strength": 0.5}},
		},
		{feature: workflow.FeatureSlugPartialEdit},
		{feature: workflow.FeatureSlugRemoveBg},
		{
			feature: workflow.FeatureSlugStyleFusion,
			images:  []string{"https://example.com/a.png", "https://example.com/b.png"},
			params: map[string]interface{}{
				"fusion": map[string]interface{}{"input_weights": []float64{3, 1}},
			},
		},
		{feature: workflow.FeatureSlugPatternGen},
		{
			feature: workflow.FeatureSlugHDUpscale,
			params:  map[string]interface{}{"hd": map[string]interface{}{"clarity_strength": 0.5}},
		},
		{
			feature: workflow.FeatureSlugImageExtend,
			params: map[string]interface{}{"extend": map[string]interface{}{
				"top_percent": 0, "right_percent": 25, "bottom_percent": 0, "left_percent": 0,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			requestParams := map[string]interface{}{"output_count": 2}
			for key, value := range tt.params {
				requestParams[key] = value
			}
			canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt:         "保留主体",
				Feature:        tt.feature,
				ImageURLs:      tt.images,
				Params:         requestParams,
				ClientActionID: "intake-" + tt.feature,
			})
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if canonical.Feature() != tt.feature {
				t.Fatalf("Feature() = %q, want %q", canonical.Feature(), tt.feature)
			}
			if canonical.UserID() != 42 {
				t.Fatalf("UserID() = %d, want authorized user 42", canonical.UserID())
			}

			billingRequest := canonical.BillingRequest()
			if billingRequest == nil || billingRequest.Feature != tt.feature || billingRequest.Quantity != 2 {
				t.Fatalf("BillingRequest() = %#v", billingRequest)
			}

			var params map[string]interface{}
			if err := json.Unmarshal([]byte(canonical.Params()), &params); err != nil {
				t.Fatalf("decode canonical params: %v", err)
			}
			if _, exists := params["model"]; exists {
				t.Fatalf("canonical params must not snapshot model: %#v", params)
			}
			if _, exists := params["account_ids"]; exists {
				t.Fatalf("canonical params must not snapshot account ids: %#v", params)
			}
		})
	}
}

func TestTaskIntakeRejectsOutputCountOutsideCanonicalContract(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	for _, outputCount := range []interface{}{0, 5, 8, 1.5, "2"} {
		t.Run(fmt.Sprint(outputCount), func(t *testing.T) {
			_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
				Prompt:         "generate",
				Feature:        workflow.FeatureSlugPatternGen,
				ClientActionID: "invalid-output-count",
				Params:         map[string]interface{}{"output_count": outputCount},
			})
			if err == nil || !strings.Contains(err.Error(), "output_count 仅支持整数 1-4") {
				t.Fatalf("output_count=%v error=%v", outputCount, err)
			}
		})
	}
}

func TestTaskIntakePersistsProviderIndependentGenerationSnapshot(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101, Models: []accounts.AccountModel{{Name: "runtime-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})
	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt: "draw a chair", Feature: workflow.FeatureSlugSketchToImage,
		ClientActionID: "snapshot-intake", Params: map[string]interface{}{"output_count": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	params, err := tasks.UnmarshalParams(canonical.Params())
	if err != nil {
		t.Fatal(err)
	}
	if params.Generation == nil {
		t.Fatal("expected generation snapshot in canonical params")
	}
	snapshot := params.Generation
	if snapshot.Feature != workflow.FeatureSlugSketchToImage || snapshot.UserPrompt != "draw a chair" || snapshot.CompiledPrompt != canonical.Prompt() {
		t.Fatalf("generation snapshot=%+v canonical prompt=%q", snapshot, canonical.Prompt())
	}
	if snapshot.RequestedOutputCount != 2 || snapshot.TemplateFingerprint == "" || snapshot.TemplateVersion != "" {
		t.Fatalf("generation snapshot missing frozen facts=%+v", snapshot)
	}
	if canonical.GenerationSnapshot() == nil {
		t.Fatal("expected canonical request generation snapshot")
	}
}

func TestTaskIntakeAllowsMissingSystemPrompt(t *testing.T) {
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Media: testSubmitMediaValidator{},
	})

	canonical, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugPatternGen,
		ClientActionID: "missing-system-prompt",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if canonical.Prompt() != "repeatable textile pattern" {
		t.Fatalf("Prompt = %q, want user prompt unchanged", canonical.Prompt())
	}
}

func TestTaskIntakePreservesPromptCauseBehindStableError(t *testing.T) {
	cause := errors.New("database connection refused")
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "runtime-selected-model", Enabled: true}},
		}}},
		Prompts: fakeFeaturePromptStore{err: cause},
		Media:   testSubmitMediaValidator{},
	})

	_, err := intake.Compile(context.Background(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "repeatable textile pattern",
		Feature:        workflow.FeatureSlugPatternGen,
		ClientActionID: "prompt-cause",
	})
	if !errors.Is(err, cause) {
		t.Fatalf("Prepare() error = %v, want wrapped cause", err)
	}
	var submissionErr *SubmissionError
	if !errors.As(err, &submissionErr) {
		t.Fatalf("Prepare() error = %T, want SubmissionError", err)
	}
	if submissionErr.Code != "prompt_unavailable" || submissionErr.SafeMessage != "系统提示词暂不可用" {
		t.Fatalf("SubmissionError = %#v", submissionErr)
	}
}
