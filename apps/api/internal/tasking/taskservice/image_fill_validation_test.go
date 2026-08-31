package taskservice

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestValidateImageFillDimensions(t *testing.T) {
	tests := []struct {
		name        string
		feature     string
		controls    *tasks.GenerationControls
		outpaint    *tasks.OutpaintParams
		wantMessage string
	}{
		{
			name:    "outpaint accepts aligned dimensions within limits",
			feature: workflow.FeatureSlugOutpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 1001, OriginalHeight: 1001,
			},
			outpaint: &tasks.OutpaintParams{TargetWidth: 1201, TargetHeight: 1001},
		},
		{
			name:    "inpaint accepts matching four megapixel dimensions",
			feature: workflow.FeatureSlugInpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 2048, OriginalHeight: 2048,
				CustomWidth: 2048, CustomHeight: 2048,
			},
		},
		{
			name:        "requires all dimensions",
			feature:     workflow.FeatureSlugOutpaint,
			controls:    &tasks.GenerationControls{OriginalWidth: 1024, OriginalHeight: 1024},
			wantMessage: "扩图需要提供目标尺寸和原图偏移",
		},
		{
			name:    "rejects smaller outpaint target",
			feature: workflow.FeatureSlugOutpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 1024, OriginalHeight: 768,
			},
			outpaint:    &tasks.OutpaintParams{TargetWidth: 1000, TargetHeight: 768},
			wantMessage: "扩图目标尺寸不能小于原图尺寸",
		},
		{
			name:    "rejects negative image offset",
			feature: workflow.FeatureSlugOutpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 1024, OriginalHeight: 768,
			},
			outpaint:    &tasks.OutpaintParams{TargetWidth: 1280, TargetHeight: 768, ImageOffsetX: -1},
			wantMessage: "扩图偏移不能让原图超出目标画布",
		},
		{
			name:    "requires inpaint dimensions to match",
			feature: workflow.FeatureSlugInpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 1024, OriginalHeight: 768,
				CustomWidth: 1024, CustomHeight: 776,
			},
			wantMessage: "局部重绘的目标尺寸必须与原图尺寸一致",
		},
		{
			name:    "rejects edge over limit",
			feature: workflow.FeatureSlugOutpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 1024, OriginalHeight: 1024,
			},
			outpaint:    &tasks.OutpaintParams{TargetWidth: 4097, TargetHeight: 1024},
			wantMessage: "目标图片超过图像填充服务的 4096 边长或约 4MP 限制",
		},
		{
			name:    "checks pixel limit after eight pixel alignment",
			feature: workflow.FeatureSlugInpaint,
			controls: &tasks.GenerationControls{
				OriginalWidth: 2041, OriginalHeight: 2055,
				CustomWidth: 2041, CustomHeight: 2055,
			},
			wantMessage: "原图超过图像填充服务的 4096 边长或约 4MP 限制",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImageFillDimensions(tt.feature, &tasks.WorkflowTaskParams{Controls: tt.controls, Outpaint: tt.outpaint})
			if tt.wantMessage == "" {
				if err != nil {
					t.Fatalf("validateImageFillDimensions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("validateImageFillDimensions() error = %v, want %q", err, tt.wantMessage)
			}
		})
	}
}

func TestTaskIntakeRejectsImageFillDimensionsBeforeProviderLookup(t *testing.T) {
	accountsStore := &countingImageFillAccountStore{}
	intake := NewTaskIntake(TaskIntakeDeps{
		Accounts: accountsStore,
		Media:    testSubmitMediaValidator{},
	})

	_, err := intake.Compile(t.Context(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "补全右侧背景",
		Feature:        workflow.FeatureSlugOutpaint,
		ImageURLs:      []string{"https://example.com/source.png"},
		ClientActionID: "invalid-fill-size",
		Params: map[string]interface{}{
			"original_image_width":  1024,
			"original_image_height": 1024,
			"outpaint": map[string]interface{}{
				"target_width": 5000, "target_height": 1024,
				"image_offset_x": 0, "image_offset_y": 0,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "4096 边长或约 4MP 限制") {
		t.Fatalf("Compile() error = %v", err)
	}
	if accountsStore.calls != 0 {
		t.Fatalf("provider lookup calls = %d, want 0", accountsStore.calls)
	}
}

func TestTaskIntakeReadinessRejectsImageFillDimensionsBeforeProviderLookup(t *testing.T) {
	accountsStore := &countingImageFillAccountStore{}
	intake := NewTaskIntake(TaskIntakeDeps{Accounts: accountsStore})

	_, err := intake.CompileReadiness(t.Context(), 42, &tasks.SubmitTaskRequest{
		Feature: workflow.FeatureSlugInpaint,
		Params: map[string]interface{}{
			"original_image_width":  1024,
			"original_image_height": 768,
			"custom_width":          1024,
			"custom_height":         776,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "必须与原图尺寸一致") {
		t.Fatalf("CompileReadiness() error = %v", err)
	}
	if accountsStore.calls != 0 {
		t.Fatalf("provider lookup calls = %d, want 0", accountsStore.calls)
	}
}

func TestSubmitTaskRejectsImageFillDimensionsBeforeSubmissionTransaction(t *testing.T) {
	accountsStore := &countingImageFillAccountStore{}
	submissionStore := &recordingSubmissionStore{}
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo:     accountsStore,
		SubmissionStore: submissionStore,
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})

	_, err := service.SubmitTask(t.Context(), 42, &tasks.SubmitTaskRequest{
		Prompt:         "补全右侧背景",
		Feature:        workflow.FeatureSlugOutpaint,
		ImageURLs:      []string{"https://example.com/source.png"},
		ClientActionID: "invalid-size-before-transaction",
		Params: map[string]interface{}{
			"original_image_width":  1024,
			"original_image_height": 1024,
			"outpaint": map[string]interface{}{
				"target_width": 5000, "target_height": 1024,
				"image_offset_x": 0, "image_offset_y": 0,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "4096 边长或约 4MP 限制") {
		t.Fatalf("SubmitTask() error = %v", err)
	}
	if accountsStore.calls != 0 {
		t.Fatalf("provider lookup calls = %d, want 0", accountsStore.calls)
	}
	if len(submissionStore.calls) != 0 {
		t.Fatalf("submission transaction calls = %d, want 0", len(submissionStore.calls))
	}
}

type countingImageFillAccountStore struct {
	calls int
}

func (s *countingImageFillAccountStore) GetAllEnabled(context.Context) ([]*accounts.ApiAccount, error) {
	s.calls++
	return []*accounts.ApiAccount{{
		ID:           101,
		Capabilities: []string{string(workflow.RoutingCapabilityFill)},
	}}, nil
}
