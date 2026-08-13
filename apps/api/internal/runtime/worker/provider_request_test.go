package worker

import (
	"context"
	"image"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestNewProviderRequestCollectsProviderMediaRefs(t *testing.T) {
	params := map[string]interface{}{
		"image_url":            "https://img.example/single.png",
		"image_urls":           []interface{}{"https://img.example/source.png", "https://img.example/shared.png"},
		"reference_image_urls": []interface{}{"https://img.example/ref.png", "https://img.example/source.png"},
		"mask_image_urls":      []interface{}{"https://img.example/mask-list.png", "https://img.example/shared.png"},
		"original_image_url":   "https://img.example/source.png",
		"mask_image_url":       "https://img.example/mask.png",
	}

	req := newProviderRequest("prompt", 640, 480, params, "gemini-image")

	wantOpenAI := []string{
		"https://img.example/source.png",
		"https://img.example/shared.png",
		"https://img.example/ref.png",
	}
	if !reflect.DeepEqual(req.MediaRefs.OpenAIImages, wantOpenAI) {
		t.Fatalf("unexpected OpenAI refs: got=%#v want=%#v", req.MediaRefs.OpenAIImages, wantOpenAI)
	}

	wantGoogle := []string{
		"https://img.example/source.png",
		"https://img.example/shared.png",
		"https://img.example/ref.png",
		"https://img.example/mask-list.png",
		"https://img.example/mask.png",
	}
	if !reflect.DeepEqual(req.MediaRefs.GoogleImages, wantGoogle) {
		t.Fatalf("unexpected Google refs: got=%#v want=%#v", req.MediaRefs.GoogleImages, wantGoogle)
	}
	if req.Size != "640x480" {
		t.Fatalf("unexpected size: %s", req.Size)
	}
}

func TestNewProviderRequestMarksInvalidOutputCount(t *testing.T) {
	req := newProviderRequest("prompt", 640, 480, map[string]interface{}{
		"output_count": 5,
	}, "gpt-image-1")
	if req.OutputCount != 0 {
		t.Fatalf("output count = %d, want invalid marker 0", req.OutputCount)
	}
}

func TestProviderRequestAfterLocalRedrawKeepsOpenAIROIInput(t *testing.T) {
	params := map[string]interface{}{
		"image_urls": []interface{}{
			"data:image/png;base64,original",
			"https://cdn.example/reference-input.png",
		},
		"reference_image_urls": []interface{}{
			"https://cdn.example/ref-a.png",
		},
	}
	patch := localRedrawUpstreamPatch{
		ROIDataURL: "data:image/png;base64,roi",
		ROI:        image.Rect(0, 0, 64, 72),
	}
	if err := patch.apply(params); err != nil {
		t.Fatalf("localRedrawUpstreamPatch.apply: %v", err)
	}

	req := newProviderRequest("prompt", 64, 72, params, "gpt-image-1")
	want := []string{
		"data:image/png;base64,roi",
		"https://cdn.example/ref-a.png",
	}
	if !reflect.DeepEqual(req.MediaRefs.OpenAIImages, want) {
		t.Fatalf("unexpected OpenAI refs after local redraw: got=%#v want=%#v", req.MediaRefs.OpenAIImages, want)
	}
}

func TestParseAndNormalizeTaskParamsFlattensCanonicalWorkflowParams(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "image-extend",
		Media: &tasks.MediaInputs{
			Inputs:   []tasks.MediaRef{{URL: "https://img.example/source.png", Role: "input"}},
			Original: &tasks.MediaRef{URL: "https://img.example/source.png", Role: "original"},
		},
		Controls: &tasks.GenerationControls{
			CustomWidth:  640,
			CustomHeight: 480,
		},
		Extend: &tasks.ImageExtendParams{
			TopPercent:    0,
			RightPercent:  20,
			BottomPercent: 0,
			LeftPercent:   0,
		},
	})
	got := (&ProviderExecutor{}).parseAndNormalizeTaskParams(context.Background(), &tasks.TaskPayload{
		TaskID: "task-canonical-params",
		Params: raw,
	})

	if got.featureSlug() != "image-extend" {
		t.Fatalf("feature slug = %q", got.featureSlug())
	}
	if !reflect.DeepEqual(got.Provider["image_urls"], []string{"https://img.example/source.png"}) {
		t.Fatalf("image_urls = %#v", got.Provider["image_urls"])
	}
	if got.Provider["custom_width"] != float64(640) || got.Provider["custom_height"] != float64(480) {
		t.Fatalf("custom size = %#v x %#v", got.Provider["custom_width"], got.Provider["custom_height"])
	}
	if got.Provider["extend_right"] != float64(20) {
		t.Fatalf("extend_right = %#v, want 20", got.Provider["extend_right"])
	}
	if _, exists := got.Provider["media"]; exists {
		t.Fatalf("provider map should not expose canonical media object: %#v", got.Provider)
	}
}

func TestRiverPayloadCanonicalParamsBecomeProviderRequest(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "style-fusion",
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{
				{URL: "https://img.example/a.png", Role: "input", Weight: 0.75},
				{URL: "https://img.example/b.png", Role: "input", Weight: 0.25},
			},
			References: []tasks.MediaRef{
				{URL: "https://img.example/ref.png", Role: "reference"},
			},
		},
		Controls: &tasks.GenerationControls{
			CustomWidth:  512,
			CustomHeight: 768,
		},
		Fusion: &tasks.StyleFusionParams{
			Mode:         "weighted_reference",
			InputWeights: []float64{0.75, 0.25},
		},
	})
	submitted := &tasks.TaskPayload{
		TaskID: "task-river-contract",
		UserID: 42,
		Prompt: "融合两张图",
		Params: raw,
	}
	decoded := roundTripDrawingTaskForTest(t, submitted)

	params := (&ProviderExecutor{}).parseAndNormalizeTaskParams(context.Background(), decoded)
	req := params.providerRequest(decoded.Prompt, "gpt-image-1")

	if req.Prompt != submitted.Prompt || req.Model != "gpt-image-1" || req.Size != "512x768" {
		t.Fatalf("provider request = prompt:%q model:%q size:%q", req.Prompt, req.Model, req.Size)
	}
	if !reflect.DeepEqual(req.MediaRefs.OpenAIImages, []string{
		"https://img.example/a.png",
		"https://img.example/b.png",
		"https://img.example/ref.png",
	}) {
		t.Fatalf("OpenAI refs=%#v", req.MediaRefs.OpenAIImages)
	}
	if !reflect.DeepEqual(req.Params["fusion_input_weights"], []float64{0.75, 0.25}) {
		t.Fatalf("fusion_input_weights=%#v", req.Params["fusion_input_weights"])
	}
	if req.Params["feature"] != "style-fusion" {
		t.Fatalf("feature=%#v, want style-fusion", req.Params["feature"])
	}
	if _, exists := req.Params["media"]; exists {
		t.Fatalf("provider request should not expose canonical media object: %#v", req.Params)
	}
}

func TestPrepareProviderExecutionRejectsInvalidPatternGenTaskContract(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugPatternGen,
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{{URL: "https://img.example/source.png", Role: "input"}},
		},
	})
	_, err := (&ProviderExecutor{}).prepareProviderExecution(
		context.Background(),
		&accounts.ApiAccount{
			Name:    "pattern-gen-provider",
			ApiKey:  "key",
			ApiType: workflow.ProviderOpenAICompatible,
		},
		&tasks.TaskPayload{
			TaskID: "task-invalid-pattern-gen",
			UserID: 42,
			Prompt: "repeatable textile pattern",
			Params: raw,
		},
		taskqueue.TaskAssignment{},
	)
	if err == nil {
		t.Fatal("expected pattern-gen provider validation error")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details, got %T (%v)", err, err)
	}
	if stage != tasks.ExecutionStageAssigned || category != tasks.FailureCategoryInputInvalid {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageAssigned, tasks.FailureCategoryInputInvalid)
	}
	if !strings.Contains(err.Error(), "图案生成不需要输入图片，请移除图片输入后重试") {
		t.Fatalf("error=%q, want actionable pattern-gen media message", err.Error())
	}
}

func TestPrepareProviderExecutionAllowsPatternGenProviderHintsAndControls(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugPatternGen,
		Controls: &tasks.GenerationControls{
			Ratio:       "4:3",
			OutputCount: 2,
		},
		Extra: map[string]interface{}{
			"provider_hint": "keep",
		},
	})
	exec, err := (&ProviderExecutor{}).prepareProviderExecution(
		context.Background(),
		&accounts.ApiAccount{
			Name:    "pattern-gen-provider",
			ApiKey:  "key",
			ApiType: workflow.ProviderOpenAICompatible,
			Models: []accounts.AccountModel{
				{Name: "gpt-image-1", Enabled: true},
			},
		},
		&tasks.TaskPayload{
			TaskID: "task-pattern-gen-provider-hints",
			UserID: 42,
			Prompt: "repeatable textile pattern",
			Params: raw,
		},
		taskqueue.TaskAssignment{ResolvedModel: "gpt-image-1"},
	)
	if err != nil {
		t.Fatalf("prepareProviderExecution returned error: %v", err)
	}
	if exec.Request.Params["provider_hint"] != "keep" {
		t.Fatalf("provider_hint = %#v, want keep", exec.Request.Params["provider_hint"])
	}
	if exec.Request.Params["output_count"] != 2 {
		t.Fatalf("output_count = %#v, want 2", exec.Request.Params["output_count"])
	}
}

func TestProviderTaskParamsPreferCanonicalFeature(t *testing.T) {
	params := newProviderTaskParams(&tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "style-fusion",
		Extra: map[string]interface{}{
			"feature": "partial-edit",
		},
	})

	if got := params.featureSlug(); got != "style-fusion" {
		t.Fatalf("featureSlug = %q, want style-fusion", got)
	}
	if params.Provider["feature"] != "style-fusion" {
		t.Fatalf("provider params should keep canonical feature, got %#v", params.Provider)
	}
}
