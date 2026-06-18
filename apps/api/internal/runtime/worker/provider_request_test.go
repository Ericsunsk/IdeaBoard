package worker

import (
	"image"
	"reflect"
	"testing"

	"ideaboard/api/internal/runtime/queuetypes"
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

	got := (&ProviderExecutor{}).parseAndNormalizeTaskParams(&queuetypes.TaskPayload{
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
