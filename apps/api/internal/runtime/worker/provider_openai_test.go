package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
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

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/workflow"
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

type testPublicURLProvider struct {
	setting settings.PublicURLSetting
}

func (p testPublicURLProvider) GetPublicURL(context.Context) (settings.PublicURLSetting, error) {
	return p.setting, nil
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

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

func TestCallOpenAIRemoveBgForcesTransparentPNGOutput(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/no-bg.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	params := map[string]interface{}{
		"feature":       "remove-bg",
		"image_urls":    []interface{}{buildTinyPNGDataURL(t)},
		"output_format": "jpeg",
	}

	if _, err := c.callOpenAI(context.Background(), account, "remove background", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edit endpoint, got %s", gotPath)
	}
	if gotMultipart.Fields["output_format"][0] != "png" {
		t.Fatalf("output_format = %#v, want png", gotMultipart.Fields["output_format"])
	}
	if gotMultipart.Fields["background"][0] != "transparent" {
		t.Fatalf("background = %#v, want transparent", gotMultipart.Fields["background"])
	}
}

func TestCallOpenAIRemoveBgRejectsModelWithoutTransparentBackground(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected upstream request: %s", r.URL.Path)
	}))
	defer server.Close()

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-2", Enabled: true}},
	}
	params := map[string]interface{}{
		"feature":    "remove-bg",
		"image_urls": []interface{}{buildTinyPNGDataURL(t)},
	}

	if _, err := c.callOpenAI(context.Background(), account, "remove background", 1024, 1024, 2*time.Second, params, "gpt-image-2"); err == nil {
		t.Fatalf("expected transparent background capability error")
	}
}

func TestOpenAIImageModelCapabilitiesFor(t *testing.T) {
	if openAIImageModelCapabilitiesFor(" GPT-IMAGE-2 ").SupportsTransparentBackground {
		t.Fatalf("expected gpt-image-2 to reject transparent backgrounds")
	}
	if !openAIImageModelCapabilitiesFor("gpt-image-1").SupportsTransparentBackground {
		t.Fatalf("expected gpt-image-1 to support transparent backgrounds")
	}
	if !openAIImageModelCapabilitiesFor("custom-openai-compatible-image-model").SupportsTransparentBackground {
		t.Fatalf("expected unknown OpenAI-compatible models to keep transparent background enabled")
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

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

func TestCallOpenAIStyleVariationUsesEditEndpointAndMultipartImage(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture
	imageBytes := buildTinyPNGBytes(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/source.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
			return
		}
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/style.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	params := map[string]interface{}{
		"feature":    "style-variation",
		"image_urls": []interface{}{server.URL + "/source.png"},
		"variation": map[string]interface{}{
			"strength": 42,
		},
	}

	gotResult, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1")
	if err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotResult.ImageURL != "https://cdn.example/style.png" {
		t.Fatalf("unexpected image url: %s", gotResult.ImageURL)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edits endpoint, got %s", gotPath)
	}
	images := gotMultipart.Files["image"]
	if len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	if string(images[0].Data) != string(imageBytes) {
		t.Fatalf("unexpected image part payload")
	}
	if got := gotMultipart.Fields["variation_strength"]; len(got) != 1 || got[0] != "42" {
		t.Fatalf("expected variation_strength field, got %#v", got)
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

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

func TestCallOpenAIStyleVariationDataURLUsesMultipartWithoutLegacyImageFields(t *testing.T) {
	var gotPath string
	var gotMultipart multipartRequestCapture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMultipart = readMultipartRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/style.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	dataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":    "style-variation",
		"image_urls": []interface{}{dataURL},
		"variation": map[string]interface{}{
			"strength": 5,
		},
	}

	if _, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("expected edits endpoint, got %s", gotPath)
	}
	if images := gotMultipart.Files["image"]; len(images) != 1 {
		t.Fatalf("expected one image file, got %#v", images)
	}
	for _, field := range []string{"image", "image_urls", "original_image_url"} {
		if _, exists := gotMultipart.Fields[field]; exists {
			t.Fatalf("did not expect legacy %s field in multipart request: %#v", field, gotMultipart.Fields)
		}
	}
	if got := gotMultipart.Fields["variation_strength"]; len(got) != 1 || got[0] != "5" {
		t.Fatalf("expected variation_strength field, got %#v", got)
	}
}

func TestCallOpenAIStyleVariationFlattensNestedContractParams(t *testing.T) {
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
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	params := map[string]interface{}{
		"feature": "style-variation",
		"variation": map[string]interface{}{
			"strength": 5,
		},
	}

	if _, err := c.callOpenAI(context.Background(), account, "style prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotBody["variation_strength"] != float64(5) {
		t.Fatalf("expected provider variation_strength field, got %#v", gotBody["variation_strength"])
	}
	if _, exists := gotBody["variation"]; exists {
		t.Fatalf("did not expect nested variation contract in provider body")
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
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: server.URL}},
	}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
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

func TestCallOpenAIUsesFirstModelWhenAccountHasMultipleModels(t *testing.T) {
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
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}, {Name: "gpt-image-2", Enabled: true}},
	}
	params := map[string]interface{}{"feature": "pattern-gen"}

	resolved, ok := accounts.ResolveAccountModelForParams(account, "", params)
	if !ok {
		t.Fatalf("expected account model to resolve")
	}
	if _, err := c.callOpenAI(context.Background(), account, "prompt", 1024, 1024, 2*time.Second, params, resolved.Name); err != nil {
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
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	task := &tasks.TaskPayload{
		TaskID: "task-model-field",
		Prompt: "prompt",
		Params: `{"feature":"pattern-gen"}`,
	}
	output, err := c.callDrawingAPI(context.Background(), account, task)
	if err != nil {
		t.Fatalf("callDrawingAPI returned error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["model"] != "gpt-image-1" {
		t.Fatalf("expected result.model gpt-image-1, got %#v", result["model"])
	}
}

func TestCallDrawingAPIUsesAssignmentResolvedModel(t *testing.T) {
	outputData := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
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
		Models: []accounts.AccountModel{
			{Name: "gpt-image-1", Enabled: true},
			{Name: "imagen-4", Enabled: true},
		},
	}
	task := &tasks.TaskPayload{
		TaskID: "task-assignment-model",
		Prompt: "prompt",
		Params: `{"feature":"pattern-gen"}`,
	}
	output, err := c.callDrawingAPIWithAssignment(context.Background(), account, task, taskqueue.TaskAssignment{ResolvedModel: "imagen-4"})
	if err != nil {
		t.Fatalf("callDrawingAPIWithAssignment returned error: %v", err)
	}
	if gotBody["model"] != "imagen-4" {
		t.Fatalf("request model=%#v, want imagen-4", gotBody["model"])
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["model"] != "imagen-4" {
		t.Fatalf("result model=%#v, want imagen-4", result["model"])
	}
}

func TestCallOpenAIHDUpscaleUsesGenerationEndpointAndIgnoresMaskParams(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/upscale.png"}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	sourceDataURL := buildTinyPNGDataURL(t)
	maskDataURL := buildTinyPNGDataURL(t)
	params := map[string]interface{}{
		"feature":    "hd-upscale",
		"image_urls": []interface{}{sourceDataURL},
		"hd": map[string]interface{}{
			"clarity_strength": 75,
		},
		"mask_image_url":  maskDataURL,
		"mask_image_urls": []interface{}{maskDataURL},
		"use_mask":        true,
	}

	if _, err := c.callOpenAI(context.Background(), account, "upscale prompt", 1024, 1024, 2*time.Second, params, "gpt-image-1"); err != nil {
		t.Fatalf("callOpenAI returned error: %v", err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected generations endpoint, got %s", gotPath)
	}
	if gotBody["image"] != sourceDataURL {
		t.Fatalf("expected source image field, got %#v", gotBody["image"])
	}
	if gotBody["output_quality"] != float64(90) {
		t.Fatalf("expected output_quality field, got %#v", gotBody["output_quality"])
	}
	for _, key := range []string{"mask", "images", "mask_image_url", "mask_image_urls", "use_mask", "upscale_factor", "output_format", "upscale_mode"} {
		if _, exists := gotBody[key]; exists {
			t.Fatalf("did not expect %s in hd-upscale generation request body: %#v", key, gotBody[key])
		}
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
		"feature":    "partial-edit",
		"image_urls": []interface{}{"https://img.example/source.png"},
		"use_mask":   true,
	}) {
		t.Fatalf("expected partial-edit to use edit endpoint")
	}

	if shouldUseOpenAIEditEndpoint(map[string]interface{}{
		"feature":        "hd-upscale",
		"image_urls":     []interface{}{"https://img.example/source.png"},
		"mask_image_url": "https://img.example/mask.png",
		"use_mask":       true,
	}) {
		t.Fatalf("expected hd-upscale to stay on generation endpoint even with stale mask params")
	}

	for _, feature := range []string{"style-fusion", "remove-bg", "style-variation", "sketch-to-image"} {
		if !shouldUseOpenAIEditEndpoint(map[string]interface{}{
			"feature":    feature,
			"image_urls": []interface{}{"https://img.example/source.png", "https://img.example/ref.png"},
		}) {
			t.Fatalf("expected %s to use edit endpoint", feature)
		}
	}
}

func TestReadOpenAIProviderParamsDerivesEditMaskAndHDQuality(t *testing.T) {
	maskURL := "https://img.example/mask.png"
	req := newProviderRequest("prompt", 1024, 1024, map[string]interface{}{
		"feature":        "partial-edit",
		"image_urls":     []interface{}{"https://img.example/source.png"},
		"mask_image_url": maskURL,
	}, "gpt-image-1")

	params := readOpenAIProviderParams(req)
	if !params.useEditEndpoint() {
		t.Fatalf("expected partial-edit image request to use edit endpoint")
	}
	if params.MaskImageRef != maskURL {
		t.Fatalf("mask ref = %q, want %q", params.MaskImageRef, maskURL)
	}
	if len(params.EditImageRefs) != 1 || params.EditImageRefs[0] != "https://img.example/source.png" {
		t.Fatalf("edit refs = %#v", params.EditImageRefs)
	}

	hdReq := newProviderRequest("prompt", 1024, 1024, map[string]interface{}{
		"feature": "hd-upscale",
		"hd": map[string]interface{}{
			"clarity_strength": 75,
		},
	}, "gpt-image-1")
	hdParams := readOpenAIProviderParams(hdReq)
	if hdParams.HDOutputQuality == nil || *hdParams.HDOutputQuality != 90 {
		t.Fatalf("HD output quality = %#v, want 90", hdParams.HDOutputQuality)
	}
	if hdParams.useEditEndpoint() {
		t.Fatalf("hd-upscale should stay on generation endpoint")
	}
}

func TestOpenAIRequestFinalizerDropsRedundantFields(t *testing.T) {
	reqBody := openAIImageRequestBody{
		Image:     "data:image/png;base64,abc",
		ImageURLs: []string{"data:image/png;base64,abc"},
		Extra: map[string]interface{}{
			"original_image_url": "data:image/png;base64,abc",
		},
	}
	openAIRequestFinalizer{FeatureSlug: workflow.FeatureSlugStyleVariation}.apply(&reqBody)

	if reqBody.Image != "data:image/png;base64,abc" {
		t.Fatalf("image = %q", reqBody.Image)
	}
	if reqBody.ImageURLs != nil {
		t.Fatalf("image_urls should be dropped, got %#v", reqBody.ImageURLs)
	}
	if _, exists := reqBody.Extra["original_image_url"]; exists {
		t.Fatalf("original_image_url should be dropped, got %#v", reqBody.Extra)
	}

	reqBody = openAIImageRequestBody{
		Image:     "https://img.example/first.png",
		ImageURLs: []string{"https://img.example/first.png", "https://img.example/second.png"},
		Extra:     map[string]interface{}{},
	}
	openAIRequestFinalizer{FeatureSlug: workflow.FeatureSlugStyleFusion}.apply(&reqBody)
	if reqBody.Image != "" {
		t.Fatalf("multi image fallback should be dropped, got %q", reqBody.Image)
	}
}

func TestCollectOpenAIEditImageRefsIncludesReferenceImageURLs(t *testing.T) {
	refs := collectOpenAIEditImageRefs(map[string]interface{}{
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
	want := []string{
		"https://img.example/source.png",
		"https://img.example/ref-a.png",
		"https://img.example/ref-b.png",
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("unexpected refs: got=%#v want=%#v", refs, want)
	}
}

func TestNormalizeUpstreamMediaParamsAbsolutizesStructuredUploadPaths(t *testing.T) {
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{"/uploads/workspace/OWNR1A/board-1/content/source.png"},
		"original_image_url": "/uploads/workspace/OWNR1A/board-1/content/source.png",
		"mask_image_url":     "/uploads/workspace/OWNR1A/board-1/content/mask.png",
		"variation_strength": 5,
	}

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: "https://app.example.com"}},
	}
	c.normalizeUpstreamMediaParams(context.Background(), params)

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
	params := map[string]interface{}{
		"feature":            "style-variation",
		"original_image_url": "/uploads/workspace/OWNR1A/board-1/content/source.png",
	}

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{
			PublicBaseURL:      "https://app.example.com",
			PublicAssetBaseURL: "https://api.example.com",
		}},
	}
	c.normalizeUpstreamMediaParams(context.Background(), params)

	if got := params["original_image_url"]; got != "https://api.example.com/uploads/workspace/OWNR1A/board-1/content/source.png" {
		t.Fatalf("unexpected normalized original_image_url: %#v", got)
	}
}

func TestNormalizeUpstreamMediaParamsRejectsLegacyGenericUploadPaths(t *testing.T) {
	ref := "/uploads/2026/04/23/source.png"
	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{ref},
		"original_image_url": ref,
	}

	c := &ProviderExecutor{
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{PublicBaseURL: "https://app.example.com"}},
	}
	c.normalizeUpstreamMediaParams(context.Background(), params)

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
	c.normalizeUpstreamMediaParams(context.Background(), params)

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
	c.normalizeUpstreamMediaParams(context.Background(), params)

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
