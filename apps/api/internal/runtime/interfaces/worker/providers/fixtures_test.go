package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func buildTinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode tiny png: %v", err)
	}
	return buf.Bytes()
}

func marshalWorkflowTaskParamsForTest(t testing.TB, params *tasks.WorkflowTaskParams) string {
	t.Helper()
	if workflow.IsAIImageFeature(params.Service) && len(params.ExecutionPlan) == 0 {
		imageInputCount := 0
		hasMaskInput := false
		if params.Media != nil {
			imageInputCount = len(params.Media.Inputs) + len(params.Media.References)
			if params.Media.Original != nil {
				imageInputCount++
			}
			hasMaskInput = params.Media.Mask != nil
		}
		var requestedOutputCount *int
		if params.Controls != nil && params.Controls.OutputCount > 0 {
			count := params.Controls.OutputCount
			requestedOutputCount = &count
		}
		plan, err := workflow.CompileFeatureExecutionPlan(workflow.CompileFeatureExecutionInput{
			Feature: params.Service,
			Params: workflow.FeatureExecutionParams{
				RequestedOutputCount: requestedOutputCount,
				ImageInputCount:      imageInputCount,
				HasMaskInput:         hasMaskInput,
			},
		})
		if err != nil {
			t.Fatalf("compile workflow execution plan: %v", err)
		}
		params.ExecutionPlan, err = workflow.EncodeFeatureExecutionPlan(plan)
		if err != nil {
			t.Fatalf("encode workflow execution plan: %v", err)
		}
		if params.Controls == nil {
			params.Controls = &tasks.GenerationControls{}
		}
		params.Controls.OutputCount = plan.RequestedOutputCount().Int()
		if params.Generation == nil {
			params.Generation = &tasks.GenerationRequestSnapshot{}
		}
		params.Generation.SchemaVersion = tasks.CurrentParamsSchemaVersion
		params.Generation.Service = plan.Feature()
		params.Generation.Primitive = string(plan.RuntimeInputs().GenerationPrimitive())
		params.Generation.RequestedOutputCount = plan.RequestedOutputCount().Int()
		if params.Generation.TemplateFingerprint == "" {
			params.Generation.TemplateFingerprint = "worker-test"
		}
		params.Generation.Media = generationMediaFromParamsForTest(params.Media)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal workflow task params: %v", err)
	}
	return string(payload)
}

func generationMediaFromParamsForTest(media *tasks.MediaInputs) []tasks.GenerationMediaIntent {
	if media == nil {
		return nil
	}
	result := make([]tasks.GenerationMediaIntent, 0, len(media.Inputs)+len(media.References)+2)
	appendRef := func(ref tasks.MediaRef, fallbackRole string) {
		if ref.AssetID <= 0 && strings.TrimSpace(ref.PublicAssetID) == "" {
			return
		}
		role := ref.Role
		if role == "" {
			role = fallbackRole
		}
		result = append(result, tasks.GenerationMediaIntent{
			InputIndex: len(result), Role: role, AssetID: ref.AssetID, PublicAssetID: strings.TrimSpace(ref.PublicAssetID), Weight: ref.Weight,
		})
	}
	for _, ref := range media.Inputs {
		appendRef(ref, "input")
	}
	for _, ref := range media.References {
		appendRef(ref, "reference")
	}
	if media.Original != nil {
		appendRef(*media.Original, "original")
	}
	if media.Mask != nil {
		appendRef(*media.Mask, "mask")
	}
	return result
}

type testRuntimeMediaResolver struct {
	urls map[int64]string
}

func (r testRuntimeMediaResolver) ResolveRuntimeMedia(_ context.Context, params *tasks.WorkflowTaskParams) (*tasks.RuntimeMediaInputs, error) {
	if params == nil || params.Media == nil {
		return nil, nil
	}
	resolve := func(ref tasks.MediaRef) tasks.RuntimeMediaRef {
		return tasks.RuntimeMediaRef{URL: r.urls[ref.AssetID], Role: ref.Role, Weight: ref.Weight}
	}
	result := &tasks.RuntimeMediaInputs{UseMask: params.Media.UseMask}
	for _, ref := range params.Media.Inputs {
		result.Inputs = append(result.Inputs, resolve(ref))
	}
	for _, ref := range params.Media.References {
		result.References = append(result.References, resolve(ref))
	}
	if params.Media.Original != nil {
		ref := resolve(*params.Media.Original)
		result.Original = &ref
	}
	if params.Media.Mask != nil {
		ref := resolve(*params.Media.Mask)
		result.Mask = &ref
	}
	return result, nil
}
