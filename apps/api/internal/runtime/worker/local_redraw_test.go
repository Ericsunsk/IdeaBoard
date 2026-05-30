package worker

import (
	"api/internal/runtime/queuetypes"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"api/internal/accounts"
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

func mustDecodeDataURLImage(t *testing.T, raw string) image.Image {
	t.Helper()
	if !strings.HasPrefix(raw, "data:image/png;base64,") {
		t.Fatalf("unexpected data url prefix: %q", raw)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("decode data url: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("decode png: %v", err)
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

func TestShouldUseOpenAIEditEndpointForLocalRedrawHDUpscale(t *testing.T) {
	params := map[string]interface{}{
		"feature":               "hd-upscale",
		"image_urls":            []interface{}{"data:image/png;base64,aGVsbG8="},
		localRedrawEnabledParam: true,
	}
	if !shouldUseOpenAIEditEndpoint(params) {
		t.Fatalf("expected masked hd-upscale local redraw to keep edit endpoint")
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

	if err := rewriteLocalRedrawUpstreamInputs(
		params,
		"partial-edit",
		"data:image/png;base64,roi",
		image.Rect(0, 0, 64, 72),
	); err != nil {
		t.Fatalf("rewriteLocalRedrawUpstreamInputs: %v", err)
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
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	params := map[string]interface{}{
		"feature":            "partial-edit",
		"image_urls":         []interface{}{originalDataURL},
		"original_image_url": originalDataURL,
		"mask_image_url":     maskDataURL,
		"use_mask":           true,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	task := &queuetypes.TaskPayload{
		TaskID: "task-local-redraw",
		Prompt: "test prompt",
		Params: string(paramsJSON),
	}

	resultJSON, err := c.callDrawingAPI(context.Background(), account, task)
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
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	imageURL, _ := result["image_url"].(string)
	finalImage := mustDecodeDataURLImage(t, imageURL)

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
