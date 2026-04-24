package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/internal/accounts"
)

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

func TestCallOpenAIUsesEditEndpointForCanvasExtend(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotMultipart multipartRequestCapture
	sourcePNG := mustDecodeBase64(t, "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCwkAAAAASUVORK5CYII=")

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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":      "canvas-extend",
		"image_urls":   []interface{}{server.URL + "/source.png"},
		"extend_right": 22,
	}

	got, err := c.callOpenAI(context.Background(), account, "extend prompt", 1024, 1024, 2*time.Second, params)
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if got != "https://cdn.example/extended.png" {
		t.Fatalf("unexpected image url: %s", got)
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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature": "pattern-gen",
	}

	got, err := c.callOpenAI(context.Background(), account, "pattern prompt", 1024, 1024, 2*time.Second, params)
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if got != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("unexpected base64 data url: %s", got)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected generations endpoint, got %s", gotPath)
	}
	if _, exists := gotBody["images"]; exists {
		t.Fatalf("did not expect images field in generation request")
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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":    "style-innovate",
		"image_urls": []interface{}{"https://img.example/source.png"},
	}

	got, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params)
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if got != "https://cdn.example/style.png" {
		t.Fatalf("unexpected image url: %s", got)
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

func TestCallOpenAIStyleInnovateDataURLDropsRedundantImageCopies(t *testing.T) {
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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	dataURL := "data:image/png;base64,aGVsbG8="
	params := map[string]interface{}{
		"feature":            "style-innovate",
		"image_urls":         []interface{}{dataURL},
		"original_image_url": dataURL,
		"variation_strength": 5,
	}

	if _, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params); err != nil {
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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	maskDataURL := "data:image/png;base64,bWFzaw=="
	sourceDataURL := "data:image/png;base64,c291cmNl"
	params := map[string]interface{}{
		"feature":            "partial-edit",
		"image_urls":         []interface{}{maskDataURL},
		"original_image_url": sourceDataURL,
		"mask_image_url":     maskDataURL,
	}

	if _, err := c.callOpenAI(context.Background(), account, "edit prompt", 1024, 1024, 2*time.Second, params); err != nil {
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

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	sourceDataURL := "data:image/png;base64,c291cmNl"
	maskDataURL := "data:image/png;base64,bWFzaw=="
	params := map[string]interface{}{
		"feature":            "hd-upscale",
		"image_urls":         []interface{}{sourceDataURL},
		"original_image_url": sourceDataURL,
		"mask_image_url":     maskDataURL,
		"use_mask":           true,
	}

	if _, err := c.callOpenAI(context.Background(), account, "upscale prompt", 1024, 1024, 2*time.Second, params); err != nil {
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

func TestShouldUseOpenAIEditEndpointOnlyForKnownEditFeatures(t *testing.T) {
	if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":    "canvas-extend",
		"image_urls": []interface{}{"https://img.example/source.png"},
	}) {
		t.Fatalf("expected canvas-extend to use edit endpoint")
	}

	if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":    "hd-upscale",
		"image_urls": []interface{}{"https://img.example/source.png"},
		"use_mask":   true,
	}) {
		t.Fatalf("expected masked hd-upscale to use edit endpoint")
	}

	if shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":            "fabric-on-body",
		"image_urls":         []interface{}{"https://img.example/source.png"},
		"mask_image_url":     "https://img.example/mask.png",
		"use_mask":           true,
		"original_image_url": "https://img.example/source.png",
	}) {
		t.Fatalf("did not expect masked fabric-on-body to use edit endpoint")
	}
}

func TestNormalizeUpstreamMediaParamsAbsolutizesRelativeUploadPaths(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	params := map[string]interface{}{
		"feature":            "style-innovate",
		"image_urls":         []interface{}{"/uploads/2026/04/23/source.png"},
		"original_image_url": "/uploads/2026/04/23/source.png",
		"mask_image_url":     "/uploads/2026/04/23/mask.png",
		"variation_strength": 5,
	}

	c := &Consumer{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	if imageURLs[0] != "https://app.example.com/uploads/2026/04/23/source.png" {
		t.Fatalf("unexpected normalized image_urls[0]: %#v", imageURLs[0])
	}
	if got := params["original_image_url"]; got != "https://app.example.com/uploads/2026/04/23/source.png" {
		t.Fatalf("unexpected normalized original_image_url: %#v", got)
	}
	if got := params["mask_image_url"]; got != "https://app.example.com/uploads/2026/04/23/mask.png" {
		t.Fatalf("unexpected normalized mask_image_url: %#v", got)
	}
	if got := params["variation_strength"]; got != 5 {
		t.Fatalf("variation_strength should remain unchanged, got %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsInlinesRelativeUploadPathsWithoutPublicBaseURL(t *testing.T) {
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

	params := map[string]interface{}{
		"feature":            "style-innovate",
		"image_urls":         []interface{}{"/uploads/2026/04/23/source.png"},
		"original_image_url": "/uploads/2026/04/23/source.png",
	}

	c := &Consumer{}
	c.normalizeUpstreamMediaParams(params)

	imageURLs, ok := params["image_urls"].([]interface{})
	if !ok || len(imageURLs) != 1 {
		t.Fatalf("unexpected image_urls: %#v", params["image_urls"])
	}
	imageDataURL, ok := imageURLs[0].(string)
	if !ok || !strings.HasPrefix(imageDataURL, "data:image/png;base64,") {
		t.Fatalf("unexpected inlined image_urls[0]: %#v", imageURLs[0])
	}
	originalImageURL, ok := params["original_image_url"].(string)
	if !ok || !strings.HasPrefix(originalImageURL, "data:image/png;base64,") {
		t.Fatalf("unexpected inlined original_image_url: %#v", params["original_image_url"])
	}
}
