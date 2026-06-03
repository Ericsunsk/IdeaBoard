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
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"api/internal/accounts"
	"api/internal/workflow"
)

const tinyWebPBase64 = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"

type multipartFilePart struct {
	Filename    string
	ContentType string
	Data        []byte
}

type multipartRequestCapture struct {
	Fields map[string][]string
	Files  map[string][]multipartFilePart
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	return decoded
}

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

func buildTinyPNGDataURL(t *testing.T) string {
	t.Helper()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
}

func buildTinyWebPBytes(t *testing.T) []byte {
	t.Helper()
	return mustDecodeBase64(t, tinyWebPBase64)
}

func readMultipartRequest(t *testing.T, r *http.Request) multipartRequestCapture {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse media type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("expected multipart/form-data, got %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		t.Fatalf("missing multipart boundary")
	}

	reader := multipart.NewReader(r.Body, boundary)
	capture := multipartRequestCapture{
		Fields: map[string][]string{},
		Files:  map[string][]multipartFilePart{},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart part body: %v", err)
		}
		name := part.FormName()
		if part.FileName() == "" {
			capture.Fields[name] = append(capture.Fields[name], string(data))
			continue
		}
		capture.Files[name] = append(capture.Files[name], multipartFilePart{
			Filename:    part.FileName(),
			ContentType: part.Header.Get("Content-Type"),
			Data:        data,
		})
	}
	return capture
}

func TestCallOpenAIUsesEditEndpointForImageExtend(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotMultipart multipartRequestCapture
	sourcePNG := buildTinyPNGBytes(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/source.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(sourcePNG)
			return
		}
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/extended.png"}]}`))
	}))
	defer server.Close()
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":      "image-extend",
		"image_urls":   []interface{}{server.URL + "/source.png"},
		"extend_right": 22,
	}

	gotResult, err := c.callOpenAI(context.Background(), account, "extend prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1")
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotResult.ImageURL != "https://cdn.example/extended.png" {
		t.Fatalf("unexpected image url: %s", gotResult.ImageURL)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edit endpoint, got %s", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content-type, got %q", gotContentType)
	}
	if gotMultipart.Fields["prompt"][0] != "extend prompt" {
		t.Fatalf("unexpected prompt field: %#v", gotMultipart.Fields["prompt"])
	}
	if gotMultipart.Fields["model"][0] != "gpt-image-1" {
		t.Fatalf("unexpected model field: %#v", gotMultipart.Fields["model"])
	}
	if gotMultipart.Fields["extend_right"][0] != "22" {
		t.Fatalf("unexpected extend_right field: %#v", gotMultipart.Fields["extend_right"])
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image multipart file, got %#v", images)
	}
	if images[0].ContentType != "image/png" {
		t.Fatalf("unexpected image content-type: %s", images[0].ContentType)
	}
	if string(images[0].Data) != string(sourcePNG) {
		t.Fatalf("unexpected image payload bytes")
	}
}

func TestCallOpenAIRetainsGenerationEndpointForStandardRequests(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aGVsbG8="}]}`))
	}))
	defer server.Close()
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature": "pattern-gen",
	}

	gotResult, err := c.callOpenAI(context.Background(), account, "pattern prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1")
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotResult.ImageURL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("unexpected base64 data url: %s", gotResult.ImageURL)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected generations endpoint, got %s", gotPath)
	}
	if _, exists := gotBody["images"]; exists {
		t.Fatalf("did not expect images field in generation request")
	}
}

func TestCallOpenAIIgnoresOutputCountAndReturnsMultipleImagesWhenUpstreamDoes(t *testing.T) {
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/a.png"},{"url":"https://cdn.example/b.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":      "pattern-gen",
		"output_count": 2,
	}

	gotResult, err := c.callOpenAI(context.Background(), account, "pattern prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1")
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if _, exists := gotBody["n"]; exists {
		t.Fatalf("did not expect n field, got %#v", gotBody["n"])
	}
	if gotResult.ImageURL != "https://cdn.example/a.png" {
		t.Fatalf("unexpected first image url: %s", gotResult.ImageURL)
	}
	if len(gotResult.ImageURLs) != 2 ||
		gotResult.ImageURLs[0] != "https://cdn.example/a.png" ||
		gotResult.ImageURLs[1] != "https://cdn.example/b.png" {
		t.Fatalf("unexpected image urls: %#v", gotResult.ImageURLs)
	}
}

func TestCallOpenAIGenerationMapsImageURLsToStringImageField(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/style.png"}]}`))
	}))
	defer server.Close()
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":    "style-variation",
		"image_urls": []interface{}{"https://img.example/source.png"},
	}

	gotResult, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1")
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotResult.ImageURL != "https://cdn.example/style.png" {
		t.Fatalf("unexpected image url: %s", gotResult.ImageURL)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected generations endpoint, got %s", gotPath)
	}
	image, ok := gotBody["image"].(string)
	if !ok {
		t.Fatalf("expected image field as string, got %#v", gotBody["image"])
	}
	if image != "https://img.example/source.png" {
		t.Fatalf("unexpected image field value: %s", image)
	}
	imageURLs, ok := gotBody["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("expected one image_urls entry, got %#v", gotBody["image_urls"])
	}
	if imageURLs[0] != "https://img.example/source.png" {
		t.Fatalf("unexpected image_urls entry: %#v", imageURLs[0])
	}
}

func TestCallOpenAIStyleFusionUsesEditEndpointAndTwoImageParts(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture
	imageA := buildTinyPNGBytes(t)
	imageB := buildTinyPNGBytes(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageA)
			return
		case "/b.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageB)
			return
		}
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/fusion.png"}]}`))
	}))
	defer server.Close()
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":    "style-fusion",
		"image_urls": []interface{}{server.URL + "/a.png", server.URL + "/b.png"},
	}

	if _, err := c.callOpenAI(context.Background(), account, "fusion prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edit endpoint, got %s", gotPath)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 2 {
		t.Fatalf("expected two image files, got %#v", images)
	}
	if string(images[0].Data) != string(imageA) || string(images[1].Data) != string(imageB) {
		t.Fatalf("unexpected image part payload order")
	}
}

func TestCallOpenAIStyleVariationDataURLDropsRedundantImageCopies(t *testing.T) {
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/style.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	dataURL := "data:image/png;base64,aGVsbG8="
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{dataURL},
		"original_image_url": dataURL,
		"variation_strength": 5,
	}

	if _, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	image, ok := gotBody["image"].(string)
	if !ok || image != dataURL {
		t.Fatalf("expected image field to keep data url, got %#v", gotBody["image"])
	}
	if _, exists := gotBody["image_urls"]; exists {
		t.Fatalf("did not expect redundant image_urls field for data-url request")
	}
	if _, exists := gotBody["original_image_url"]; exists {
		t.Fatalf("did not expect redundant original_image_url field for data-url request")
	}
}

func TestCallOpenAIPartialEditUsesEditEndpointAndMultipartFiles(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/edit.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	maskDataURL := buildTinyPNGDataURL(t)
	sourceDataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":            "partial-edit",
		"image_urls":         []interface{}{maskDataURL},
		"original_image_url": sourceDataURL,
		"mask_image_url":     maskDataURL,
	}

	if _, err := c.callOpenAI(context.Background(), account, "edit prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edit endpoint, got %s", gotPath)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	masks := gotMultipart.Files["mask"]
	if len(masks) != 1 {
		t.Fatalf("expected one mask file, got %#v", masks)
	}
	if gotMultipart.Fields["original_image_url"][0] != sourceDataURL {
		t.Fatalf("expected original_image_url field to stay, got %#v", gotMultipart.Fields["original_image_url"])
	}
}

func TestCallOpenAIPartialEditNormalizesWebPDataURLToPNGMultipart(t *testing.T) {
	var gotMultipart multipartRequestCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/edit.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	sourceWebPDataURL := "data:image/webp;base64," + tinyWebPBase64
	maskDataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":            "partial-edit",
		"image_urls":         []interface{}{sourceWebPDataURL},
		"original_image_url": sourceWebPDataURL,
		"mask_image_url":     maskDataURL,
	}

	if _, err := c.callOpenAI(context.Background(), account, "edit prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	if images[0].ContentType != "image/png" {
		t.Fatalf("expected image/png, got %s", images[0].ContentType)
	}
	if !strings.HasSuffix(images[0].Filename, ".png") {
		t.Fatalf("expected .png filename, got %s", images[0].Filename)
	}
	if !bytes.HasPrefix(images[0].Data, []byte{0x89, 0x50, 0x4e, 0x47}) {
		headerLen := 8
		if len(images[0].Data) < headerLen {
			headerLen = len(images[0].Data)
		}
		t.Fatalf("expected png bytes, got header=%x", images[0].Data[:headerLen])
	}
}

func TestCallOpenAIPartialEditNormalizesWebPURLToPNGMultipart(t *testing.T) {
	var gotMultipart multipartRequestCapture
	webpBytes := buildTinyWebPBytes(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/source.webp" {
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write(webpBytes)
			return
		}
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/edit.png"}]}`))
	}))
	defer server.Close()
	t.Setenv("APP_PUBLIC_BASE_URL", server.URL)

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	sourceURL := server.URL + "/source.webp"
	maskDataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":            "partial-edit",
		"image_urls":         []interface{}{sourceURL},
		"original_image_url": sourceURL,
		"mask_image_url":     maskDataURL,
	}

	if _, err := c.callOpenAI(context.Background(), account, "edit prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	if images[0].ContentType != "image/png" {
		t.Fatalf("expected image/png, got %s", images[0].ContentType)
	}
	if !strings.HasSuffix(images[0].Filename, ".png") {
		t.Fatalf("expected .png filename, got %s", images[0].Filename)
	}
	if !bytes.HasPrefix(images[0].Data, []byte{0x89, 0x50, 0x4e, 0x47}) {
		headerLen := 8
		if len(images[0].Data) < headerLen {
			headerLen = len(images[0].Data)
		}
		t.Fatalf("expected png bytes, got header=%x", images[0].Data[:headerLen])
	}
}

func TestCallOpenAIUsesFirstModelWhenAccountHasMultiModelSet(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/ok.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1,gpt-image-2",
	}
	params := map[string]interface{}{"feature": "pattern-gen"}

	resolved := resolveRequestedModel(account.ModelSet, params)
	if _, err := c.callOpenAI(context.Background(), account, "prompt", 1024, 1024, 2*time.Second, params, resolved); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotBody["model"] != "gpt-image-1" {
		t.Fatalf("expected first model, got %#v", gotBody["model"])
	}
}

func TestCallDrawingAPIResultIncludesResolvedModel(t *testing.T) {
	outputData := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + outputData + `"}]}`))
	}))
	defer server.Close()

	c := newTestProviderExecutorWithLocalStorage(t)
	account := &accounts.ApiAccount{
		ID:       1,
		Name:     "test",
		Platform: "openai-compatible",
		ApiType:  workflow.ProviderOpenAICompatible,
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	task := &queuetypes.TaskPayload{
		TaskID: "task-model-field",
		Prompt: "prompt",
		Params: `{"feature":"pattern-gen"}`,
	}
	resultJSON, err := c.callDrawingAPI(context.Background(), account, task)
	if err != nil {
		t.Fatalf("callDrawingAPI returned error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["model"] != "gpt-image-1" {
		t.Fatalf("expected result.model gpt-image-1, got %#v", result["model"])
	}
}

func TestCallOpenAIHDUpscaleMaskedUsesMultipartFiles(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/upscale-masked.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	sourceDataURL := buildTinyPNGDataURL(t)
	maskDataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":            "hd-upscale",
		"image_urls":         []interface{}{sourceDataURL},
		"original_image_url": sourceDataURL,
		"mask_image_url":     maskDataURL,
		"use_mask":           true,
	}

	if _, err := c.callOpenAI(context.Background(), account, "upscale prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edit endpoint, got %s", gotPath)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	if gotMultipart.Fields["original_image_url"][0] != sourceDataURL {
		t.Fatalf("expected original_image_url field to stay, got %#v", gotMultipart.Fields["original_image_url"])
	}
	masks := gotMultipart.Files["mask"]
	if len(masks) != 1 {
		t.Fatalf("expected one mask file, got %#v", masks)
	}
}

func TestShouldUseOpenAIEditEndpointForKnownEditFeatures(t *testing.T) {
	if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":    "image-extend",
		"image_urls": []interface{}{"https://img.example/source.png"},
	}) {
		t.Fatalf("expected image-extend to use edit endpoint")
	}

	if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":    "hd-upscale",
		"image_urls": []interface{}{"https://img.example/source.png"},
		"use_mask":   true,
	}) {
		t.Fatalf("expected masked hd-upscale to use edit endpoint")
	}

	for _, feature := range []string{"style-fusion", "remove-bg"} {
		if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
			"feature":    feature,
			"image_urls": []interface{}{"https://img.example/source.png", "https://img.example/ref.png"},
		}) {
			t.Fatalf("expected %s to use edit endpoint", feature)
		}
	}
}

func TestCollectOpenAIEditImageInputsIncludesReferenceImageURLs(t *testing.T) {
	refs := collectOpenAIEditImageInputs(map[string]interface{}{
		"image_urls": []interface{}{
			"https://img.example/source.png",
		},
		"reference_image_urls": []interface{}{
			"https://img.example/ref-a.png",
			"https://img.example/source.png",
			" ",
			"https://img.example/ref-b.png",
		},
	})
	want := []map[string]string{
		{"image_url": "https://img.example/source.png"},
		{"image_url": "https://img.example/ref-a.png"},
		{"image_url": "https://img.example/ref-b.png"},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", refs, want)
	}
}

func TestNormalizeUpstreamMediaParamsAbsolutizesStructuredUploadPaths(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{"/uploads/workspace/OWNR1A/board-1/content/source.png"},
		"original_image_url": "/uploads/workspace/OWNR1A/board-1/content/source.png",
		"mask_image_url":     "/uploads/workspace/OWNR1A/board-1/content/mask.png",
		"variation_strength": 5,
	}

	c := &ProviderExecutor{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	if imageURLs[0] != "https://app.example.com/uploads/workspace/OWNR1A/board-1/content/source.png" {
		t.Fatalf("unexpected normalized image_urls[0]: %#v", imageURLs[0])
	}
	if got := params["original_image_url"]; got != "https://app.example.com/uploads/workspace/OWNR1A/board-1/content/source.png" {
		t.Fatalf("unexpected normalized original_image_url: %#v", got)
	}
	if got := params["mask_image_url"]; got != "https://app.example.com/uploads/workspace/OWNR1A/board-1/content/mask.png" {
		t.Fatalf("unexpected normalized mask_image_url: %#v", got)
	}
	if got := params["variation_strength"]; got != 5 {
		t.Fatalf("variation_strength should remain unchanged, got %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsPrefersAssetBaseURLForStructuredUploadPaths(t *testing.T) {
	t.Setenv("APP_PUBLIC_ASSET_BASE_URL", "https://api.example.com")
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	params := map[string]interface{}{
		"feature":            "style-variation",
		"original_image_url": "/uploads/workspace/OWNR1A/board-1/content/source.png",
	}

	c := &ProviderExecutor{}
	c.normalizeUpstreamMediaParams(params)

	if got := params["original_image_url"]; got != "https://api.example.com/uploads/workspace/OWNR1A/board-1/content/source.png" {
		t.Fatalf("unexpected normalized original_image_url: %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsRejectsLegacyGenericUploadPaths(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	ref := "/uploads/2026/04/23/source.png"
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{ref},
		"original_image_url": ref,
	}

	c := &ProviderExecutor{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	if got := imageURLs[0]; got != ref {
		t.Fatalf("expected legacy upload path to remain unresolved, got %#v", got)
	}
	if got := params["original_image_url"]; got != ref {
		t.Fatalf("expected legacy upload path to remain unresolved, got %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsDoesNotInlineUploadPathsWithoutPublicBaseURL(t *testing.T) {
	tempDir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})

	filePath := filepath.Join("uploads", "2026", "04", "23", "source.png")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir uploads dir: %v", err)
	}
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCwkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if err := os.WriteFile(filePath, pngBytes, 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	ref := "/uploads/2026/04/23/source.png"
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{ref},
		"original_image_url": ref,
	}

	c := &ProviderExecutor{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	if got := imageURLs[0]; got != ref {
		t.Fatalf("expected upload path to remain unresolved, got %#v", got)
	}
	if got := params["original_image_url"]; got != ref {
		t.Fatalf("expected upload path to remain unresolved, got %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsDoesNotInlineWorkspaceUploadWithoutPublicBaseURL(t *testing.T) {
	tempDir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})

	filePath := filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "content", "source.png")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir uploads dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("workspace image"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	ref := "/uploads/workspace/OWNR1A/board-1/content/source.png"
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{ref},
		"original_image_url": ref,
	}

	c := &ProviderExecutor{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	if got := imageURLs[0]; got != ref {
		t.Fatalf("expected workspace upload to remain relative, got %#v", got)
	}
	if got := params["original_image_url"]; got != ref {
		t.Fatalf("expected workspace upload to remain relative, got %#v", got)
	}
}
