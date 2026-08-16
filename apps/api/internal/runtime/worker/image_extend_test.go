package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"testing"

	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestComposeImageExtendResultPreservesOriginalAtFrozenGeometry(t *testing.T) {
	original := solidImage(2, 2, color.NRGBA{R: 220, A: 255})
	generated := solidImage(4, 2, color.NRGBA{B: 220, A: 255})

	result, err := composeImageExtendResult(original, generated, workflow.ImageExtendPromptContract{RightPercent: 100})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds().Dx() != 4 || result.Bounds().Dy() != 2 {
		t.Fatalf("result bounds=%v", result.Bounds())
	}
	if got := result.NRGBAAt(1, 1); got.R != 220 || got.B != 0 {
		t.Fatalf("original region = %#v, want preserved original", got)
	}
	if got := result.NRGBAAt(3, 1); got.B != 220 || got.R != 0 {
		t.Fatalf("extended region = %#v, want generated output", got)
	}
}

func TestFinalizeImageExtendResultsKeepsMaterializedPartialSuccess(t *testing.T) {
	original := solidImage(2, 2, color.NRGBA{R: 220, A: 255})
	generated := solidImage(4, 2, color.NRGBA{B: 220, A: 255})
	materializer := &imageExtendMaterializer{}
	executor := &ProviderExecutor{resultMaterializer: materializer}

	finalization := executor.finalizeImageExtendResults(
		context.Background(),
		42,
		"task-image-extend",
		&imageExtendContext{
			original: original,
			geometry: workflow.ImageExtendPromptContract{RightPercent: 100},
		},
		[]string{"data:image/png;base64,not-base64", imageDataURL(t, generated)},
		0,
	)
	if finalization.state != providerResultAlreadyMaterialized {
		t.Fatalf("state=%d, want already materialized", finalization.state)
	}
	urls, outputs, err := finalization.completedOutputs()
	if err != nil {
		t.Fatalf("finalizeImageExtendResults error: %v", err)
	}
	if !reflect.DeepEqual(urls, []string{"/uploads/tasks/42/task-image-extend/0.png"}) {
		t.Fatalf("urls=%#v", urls)
	}
	if len(outputs) != 1 || outputs[0].ResultIndex != 1 {
		t.Fatalf("outputs=%#v, want preserved provider result index", outputs)
	}
	if len(materializer.data) != 1 {
		t.Fatalf("materialized count=%d, want 1", len(materializer.data))
	}
	if materializer.remoteCalls != 0 {
		t.Fatalf("remote materializer calls=%d, want no fallback", materializer.remoteCalls)
	}
	result, err := png.Decode(bytes.NewReader(materializer.data[0]))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(result.At(1, 1)).(color.NRGBA); got.R != 220 || got.B != 0 {
		t.Fatalf("persisted original region=%#v", got)
	}
}

func TestFinalizeImageExtendResultsReturnsTerminalFailureWhenAllSlotsFail(t *testing.T) {
	materializer := &imageExtendMaterializer{}
	executor := &ProviderExecutor{resultMaterializer: materializer}

	finalization := executor.finalizeImageExtendResults(
		context.Background(),
		42,
		"task-image-extend-fail",
		&imageExtendContext{original: solidImage(2, 2, color.NRGBA{A: 255}), geometry: workflow.ImageExtendPromptContract{RightPercent: 100}},
		[]string{"data:image/png;base64,not-base64"},
		0,
	)
	if finalization.state != providerResultTerminalFailure {
		t.Fatalf("state=%d, want terminal failure", finalization.state)
	}
	if _, _, err := finalization.completedOutputs(); err == nil {
		t.Fatal("expected terminal failure")
	}
	if materializer.remoteCalls != 0 {
		t.Fatalf("remote materializer calls=%d, want no fallback", materializer.remoteCalls)
	}
}

func TestPrepareImageExtendPipelineReadsFrozenOriginalAndGeometry(t *testing.T) {
	original := solidImage(2, 3, color.NRGBA{R: 220, A: 255})
	executor := &ProviderExecutor{}
	prepared, err := executor.prepareImageExtendPipeline(context.Background(), nil, map[string]interface{}{
		"feature":    "ignored-by-runtime-parser",
		"image_urls": []string{imageDataURL(t, original)},
		"extend": map[string]interface{}{
			"top_percent": 0, "right_percent": 25, "bottom_percent": 0, "left_percent": 0,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if prepared == nil || prepared.original.Bounds().Dx() != 2 || prepared.geometry.RightPercent != 25 {
		t.Fatalf("prepared=%#v", prepared)
	}
}

func TestPrepareImageExtendPipelineRejectsMissingRuntimeInputs(t *testing.T) {
	executor := &ProviderExecutor{}
	if prepared, err := executor.prepareImageExtendPipeline(context.Background(), nil, map[string]interface{}{}, 0); err == nil || prepared != nil {
		t.Fatalf("prepared=%#v err=%v, want strict missing geometry failure", prepared, err)
	}
}

type imageExtendMaterializer struct {
	data        [][]byte
	remoteCalls int
}

func (m *imageExtendMaterializer) MaterializeRemoteResults(context.Context, int64, string, []string, taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	m.remoteCalls++
	return taskmedia.MaterializedResults{}, nil
}

func (m *imageExtendMaterializer) MaterializeBytesResult(_ context.Context, userID int64, taskID string, _ string, _ string, data []byte, _ taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	m.data = append(m.data, append([]byte(nil), data...))
	url := "/uploads/tasks/42/task-image-extend/0.png"
	return taskmedia.MaterializedResults{Items: []taskmedia.MaterializedResult{{
		ImageURL: url,
		Output:   tasks.TaskOutput{TaskID: taskID, Role: "image", URL: url, ResultIndex: 0},
	}}}, nil
}

func solidImage(width, height int, fill color.NRGBA) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.SetNRGBA(x, y, fill)
		}
	}
	return result
}

func imageDataURL(t *testing.T, source image.Image) string {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}
