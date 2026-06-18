package worker

import (
	"image"
	"reflect"
	"testing"

	"ideaboard/api/internal/taskqueue"
	"ideaboard/api/internal/tasks"
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
	raw, err := tasks.MarshalParams(&tasks.WorkflowTaskParams{
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
	if err != nil {
		t.Fatalf("MarshalParams: %v", err)
	}

	got := (&ProviderExecutor{}).parseAndNormalizeTaskParams(&tasks.TaskPayload{
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
	raw, err := tasks.MarshalParams(&tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "style-fusion",
		Model:         "gpt-image-1",
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
	if err != nil {
		t.Fatalf("MarshalParams: %v", err)
	}
	submitted := &tasks.TaskPayload{
		TaskID:     "task-river-contract",
		UserID:     42,
		Prompt:     "融合两张图",
		Params:     raw,
		AccountIDs: []int64{101},
	}
	encoded, err := taskqueue.EncodeRiverPayload(submitted)
	if err != nil {
		t.Fatalf("EncodeRiverPayload: %v", err)
	}
	decoded, err := taskqueue.DecodeRiverPayload(encoded)
	if err != nil {
		t.Fatalf("DecodeRiverPayload: %v", err)
	}

	params := (&ProviderExecutor{}).parseAndNormalizeTaskParams(decoded)
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

func TestProviderTaskParamsPreferCanonicalFeatureAndModel(t *testing.T) {
	params := newProviderTaskParams(&tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "style-fusion",
		Model:         "gpt-image-1",
		Extra: map[string]interface{}{
			"feature": "partial-edit",
			"model":   "other-model",
		},
	})

	if got := params.featureSlug(); got != "style-fusion" {
		t.Fatalf("featureSlug = %q, want style-fusion", got)
	}
	if got := params.requestedModel(); got != "gpt-image-1" {
		t.Fatalf("requestedModel = %q, want gpt-image-1", got)
	}
	if params.Provider["feature"] != "style-fusion" || params.Provider["model"] != "gpt-image-1" {
		t.Fatalf("provider params should keep canonical feature/model, got %#v", params.Provider)
	}
}
