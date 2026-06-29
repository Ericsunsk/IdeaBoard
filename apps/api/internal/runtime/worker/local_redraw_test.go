package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

func mustEncodePNGDataURL(t *testing.T, img image.Image) string {
	t.Helper()
	var payload strings.Builder
	payload.WriteString("data:image/png;base64,")
	var raw strings.Builder
	encoder := base64.NewEncoder(base64.StdEncoding, &raw)
	if err := png.Encode(encoder, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("close encoder: %v", err)
	}
	payload.WriteString(raw.String())
	return payload.String()
}

func mustDecodeStoredUploadImage(t *testing.T, raw string) image.Image {
	t.Helper()
	if !strings.HasPrefix(raw, "/uploads/") {
		t.Fatalf("unexpected stored upload url: %q", raw)
	}
	data, err := os.ReadFile(filepath.FromSlash(strings.TrimPrefix(raw, "/")))
	if err != nil {
		t.Fatalf("read stored upload: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode stored upload: %v", err)
	}
	return img
}

func TestBuildLocalRedrawMaskSamplerUsesLuminanceFallbackAndPadding(t *testing.T) {
	mask := image.NewNRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
		}
	}
	for y := 120; y < 200; y++ {
		for x := 100; x < 150; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	sampler, bbox, err := buildLocalRedrawMaskSampler(mask, image.Rect(0, 0, 400, 300))
	if err != nil {
		t.Fatalf("buildLocalRedrawMaskSampler: %v", err)
	}
	if !sampler.useLuminance {
		t.Fatalf("expected luminance fallback for opaque mask")
	}
	if bbox != image.Rect(100, 120, 150, 200) {
		t.Fatalf("unexpected bbox: %#v", bbox)
	}

	roi, err := expandBBoxWithPadding(bbox, image.Rect(0, 0, 400, 300))
	if err != nil {
		t.Fatalf("expandBBoxWithPadding: %v", err)
	}
	expected := image.Rect(36, 56, 214, 264)
	if roi != expected {
		t.Fatalf("unexpected roi with padding: got=%#v want=%#v", roi, expected)
	}
}

func TestShouldUseLocalRedrawPipelineOnlyForPartialEditWithMask(t *testing.T) {
	params := map[string]interface{}{
		"feature":        "partial-edit",
		"image_urls":     []interface{}{"data:image/png;base64,aGVsbG8="},
		"mask_image_url": "data:image/png;base64,bWFzaw==",
	}
	if !shouldUseLocalRedrawPipeline(params) {
		t.Fatalf("expected partial-edit with mask to use local redraw pipeline")
	}

	params["feature"] = "hd-upscale"
	if shouldUseLocalRedrawPipeline(params) {
		t.Fatalf("expected hd-upscale to stay out of local redraw even with stale mask params")
	}

	params["feature"] = "partial-edit"
	delete(params, "mask_image_url")
	if shouldUseLocalRedrawPipeline(params) {
		t.Fatalf("expected partial-edit without mask to stay out of local redraw pipeline")
	}
}

func TestRewriteLocalRedrawUpstreamInputsKeepsSingleImageURLSemantic(t *testing.T) {
	params := map[string]interface{}{
		"image_urls": []interface{}{
			"data:image/png;base64,original",
			"https://cdn.example/reference-input.png",
		},
		"reference_image_urls": []interface{}{
			"https://cdn.example/ref-a.png",
			" ",
			"https://cdn.example/ref-b.png",
		},
	}

	patch := localRedrawUpstreamPatch{
		ROIDataURL: "data:image/png;base64,roi",
		ROI:        image.Rect(0, 0, 64, 72),
	}
	if err := patch.apply(params); err != nil {
		t.Fatalf("localRedrawUpstreamPatch.apply: %v", err)
	}

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok {
		t.Fatalf("expected image_urls list, got %#v", params["image_urls"])
	}
	want := []interface{}{
		"data:image/png;base64,roi",
	}
	if !reflect.DeepEqual(imageURLs, want) {
		t.Fatalf("unexpected image_urls: got=%#v want=%#v", imageURLs, want)
	}
	referenceURLs, ok := params["reference_image_urls"].([]interface{})
	if !ok {
		t.Fatalf("expected reference_image_urls list, got %#v", params["reference_image_urls"])
	}
	wantRefs := []interface{}{
		"https://cdn.example/ref-a.png",
		" ",
		"https://cdn.example/ref-b.png",
	}
	if !reflect.DeepEqual(referenceURLs, wantRefs) {
		t.Fatalf("unexpected reference_image_urls: got=%#v want=%#v", referenceURLs, wantRefs)
	}
	if got := params["original_image_url"]; got != "data:image/png;base64,roi" {
		t.Fatalf("unexpected original_image_url: %#v", got)
	}
}

func TestCallDrawingAPILocalRedrawReplacesUpstreamInputAndCompositesByMask(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 320; x++ {
			original.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	mask := image.NewNRGBA(image.Rect(0, 0, 320, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 320; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
		}
	}
	for y := 120; y < 200; y++ {
		for x := 120; x < 200; x++ {
			mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	upstreamResult := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			upstreamResult.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	originalDataURL := mustEncodePNGDataURL(t, original)
	maskDataURL := mustEncodePNGDataURL(t, mask)
	upstreamResultData, err := encodeImagePNG(upstreamResult)
	if err != nil {
		t.Fatalf("encodeImagePNG: %v", err)
	}

	var gotMultipart multipartRequestCapture
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/edits":
			gotMultipart = readMultipartRequest(t, r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"url":"` + server.URL + `/out.png"}]}`))
			return
		case "/out.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(upstreamResultData)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	useMask := true
	paramsJSON, err := tasks.MarshalParams(&tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "partial-edit",
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{{
				URL:  originalDataURL,
				Role: "input",
			}},
			Original: &tasks.MediaRef{URL: originalDataURL, Role: "original"},
			Mask:     &tasks.MediaRef{URL: maskDataURL, Role: "mask"},
			UseMask:  &useMask,
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	c := newTestProviderExecutorWithLocalStorage(t)
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	task := &tasks.TaskPayload{
		TaskID: "task-local-redraw",
		Prompt: "test prompt",
		Params: paramsJSON,
	}

	output, err := c.callDrawingAPI(context.Background(), account, task)
	if err != nil {
		t.Fatalf("callDrawingAPI: %v", err)
	}

	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one upstream image part, got %#v", images)
	}
	if len(gotMultipart.Files["mask"]) != 0 {
		t.Fatalf("expected no mask part in local redraw mode, got %#v", gotMultipart.Files["mask"])
	}
	if _, exists := gotMultipart.Fields["use_mask"]; exists {
		t.Fatalf("expected use_mask to be removed in local redraw mode")
	}

	uploadedROI, err := png.Decode(bytes.NewReader(images[0].Data))
	if err != nil {
		t.Fatalf("decode uploaded ROI: %v", err)
	}
	if uploadedROI.Bounds().Dx() != 208 || uploadedROI.Bounds().Dy() != 208 {
		t.Fatalf("unexpected uploaded ROI size: %dx%d", uploadedROI.Bounds().Dx(), uploadedROI.Bounds().Dy())
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	imageURL, _ := result["image_url"].(string)
	finalImage := mustDecodeStoredUploadImage(t, imageURL)

	outsideMask := color.NRGBAModel.Convert(finalImage.At(40, 40)).(color.NRGBA)
	if outsideMask != (color.NRGBA{R: 0, G: 0, B: 255, A: 255}) {
		t.Fatalf("outside mask changed unexpectedly: %#v", outsideMask)
	}
	insideMask := color.NRGBAModel.Convert(finalImage.At(150, 150)).(color.NRGBA)
	if insideMask.R < 200 || insideMask.G > 20 || insideMask.B > 20 {
		t.Fatalf("inside mask should be replaced by upstream redraw, got %#v", insideMask)
	}
	outsideMaskInsideROI := color.NRGBAModel.Convert(finalImage.At(80, 80)).(color.NRGBA)
	if outsideMaskInsideROI != (color.NRGBA{R: 0, G: 0, B: 255, A: 255}) {
		t.Fatalf("non-masked pixels inside ROI should stay unchanged, got %#v", outsideMaskInsideROI)
	}
}

func TestFinalizeLocalRedrawResultUsesResultMaterializer(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	mask := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	upstream := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			original.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			upstream.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}

	outputs := []tasks.TaskOutput{{
		TaskID:    "task-local-redraw-materializer",
		Role:      "image",
		URL:       "/uploads/tasks/42/task-local-redraw-materializer/result.png",
		SourceURL: "upstream://image",
		SortIndex: 0,
	}}
	materializer := &captureResultMaterializer{
		bytesResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-local-redraw-materializer/result.png",
				Output:   outputs[0],
			}},
		},
	}
	c := &ProviderExecutor{resultMaterializer: materializer}

	imageURL, gotOutputs, stored, err := c.finalizeLocalRedrawResult(
		context.Background(),
		42,
		"task-local-redraw-materializer",
		&localRedrawContext{
			featureSlug: "partial-edit",
			original:    original,
			maskSampler: maskCoverageSampler{
				mask:         mask,
				maskBounds:   mask.Bounds(),
				dstWidth:     2,
				dstHeight:    2,
				useLuminance: true,
			},
			roi: image.Rect(0, 0, 2, 2),
		},
		mustEncodePNGDataURL(t, upstream),
		0,
	)
	if err != nil {
		t.Fatalf("finalizeLocalRedrawResult returned error: %v", err)
	}
	if imageURL != materializer.bytesResult.Items[0].ImageURL {
		t.Fatalf("imageURL=%q, want %q", imageURL, materializer.bytesResult.Items[0].ImageURL)
	}
	if !reflect.DeepEqual(gotOutputs, outputs) {
		t.Fatalf("outputs=%#v, want %#v", gotOutputs, outputs)
	}
	if !stored {
		t.Fatalf("stored=false, want true")
	}
	if materializer.bytesUserID != 42 || materializer.bytesTaskID != "task-local-redraw-materializer" {
		t.Fatalf("materializer target=%d/%s", materializer.bytesUserID, materializer.bytesTaskID)
	}
	if !strings.HasPrefix(materializer.bytesFilename, "local-redraw-partial-edit-") || !strings.HasSuffix(materializer.bytesFilename, ".png") {
		t.Fatalf("unexpected materialized filename: %q", materializer.bytesFilename)
	}
	if !strings.HasPrefix(materializer.bytesSourceURL, "data:image/png;base64,") {
		t.Fatalf("unexpected source URL passed to materializer: %q", materializer.bytesSourceURL)
	}
	finalImage, err := png.Decode(bytes.NewReader(materializer.bytesData))
	if err != nil {
		t.Fatalf("decode materialized bytes: %v", err)
	}
	gotPixel := color.NRGBAModel.Convert(finalImage.At(0, 0)).(color.NRGBA)
	if gotPixel.R < 200 || gotPixel.B > 20 {
		t.Fatalf("materialized bytes should contain composited redraw result, got %#v", gotPixel)
	}
}

func TestFinalizeLocalRedrawResultRequiresMaterializer(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	mask := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	upstream := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			original.SetNRGBA(x, y, color.NRGBA{B: 255, A: 255})
			mask.SetNRGBA(x, y, color.NRGBA{A: 255})
			upstream.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}

	c := &ProviderExecutor{}
	imageURL, outputs, stored, err := c.finalizeLocalRedrawResult(
		context.Background(),
		42,
		"task-local-redraw-no-store",
		&localRedrawContext{
			featureSlug: "partial-edit",
			original:    original,
			maskSampler: maskCoverageSampler{
				mask:       mask,
				maskBounds: mask.Bounds(),
				dstWidth:   2,
				dstHeight:  2,
			},
			roi: image.Rect(0, 0, 2, 2),
		},
		mustEncodePNGDataURL(t, upstream),
		0,
	)

	if err == nil {
		t.Fatal("expected missing materializer to fail local redraw finalization")
	}
	if imageURL != "" {
		t.Fatalf("imageURL=%q, want empty string", imageURL)
	}
	if len(outputs) != 0 {
		t.Fatalf("outputs=%#v, want empty", outputs)
	}
	if stored {
		t.Fatalf("stored=true, want false")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details, got %T (%v)", err, err)
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryStorageError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryStorageError)
	}
	if strings.Contains(err.Error(), "data:image/png;base64") {
		t.Fatalf("error should not include data URL payload: %v", err)
	}
}
