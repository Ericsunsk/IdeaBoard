package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/internal/accounts"
)

func TestCallOpenAIUsesEditEndpointForCanvasExtend(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/extended.png"}]}`))
	}))
	defer server.Close()

	c := &Consumer{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{
		"feature":      "canvas-extend",
		"image_urls":   []interface{}{"https://img.example/source.png"},
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
	if _, exists := gotBody["image"]; exists {
		t.Fatalf("did not expect legacy image field in edit request")
	}
	images, ok := gotBody["images"].([]interface{})
	if !ok || len(images) != 1 {
		t.Fatalf("expected one images entry, got %#v", gotBody["images"])
	}
	firstRef, ok := images[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected image ref object, got %#v", images[0])
	}
	if firstRef["image_url"] != "https://img.example/source.png" {
		t.Fatalf("unexpected image_url: %#v", firstRef["image_url"])
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

func TestCallOpenAIPartialEditDataURLDropsOnlyRedundantImageURLs(t *testing.T) {
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
	}

	if _, err := c.callOpenAI(context.Background(), account, "edit prompt", 1024, 1024, 2*time.Second, params); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	image, ok := gotBody["image"].(string)
	if !ok || image != maskDataURL {
		t.Fatalf("expected image field to keep mask data url, got %#v", gotBody["image"])
	}
	if _, exists := gotBody["image_urls"]; exists {
		t.Fatalf("did not expect redundant image_urls field for single-image data-url request")
	}
	originalImageURL, ok := gotBody["original_image_url"].(string)
	if !ok || originalImageURL != sourceDataURL {
		t.Fatalf("expected original_image_url to stay, got %#v", gotBody["original_image_url"])
	}
}

func TestCallOpenAIHDUpscaleMaskedDataURLDropsOnlyRedundantImageURLs(t *testing.T) {
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
	if gotBody["image"] != sourceDataURL {
		t.Fatalf("expected image to stay, got %#v", gotBody["image"])
	}
	if _, exists := gotBody["image_urls"]; exists {
		t.Fatalf("did not expect redundant image_urls field for single-image data-url request")
	}
	originalImageURL, ok := gotBody["original_image_url"].(string)
	if !ok || originalImageURL != sourceDataURL {
		t.Fatalf("expected original_image_url to stay for masked hd-upscale, got %#v", gotBody["original_image_url"])
	}
	maskImageURL, ok := gotBody["mask_image_url"].(string)
	if !ok || maskImageURL != maskDataURL {
		t.Fatalf("expected mask_image_url to stay for masked hd-upscale, got %#v", gotBody["mask_image_url"])
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
