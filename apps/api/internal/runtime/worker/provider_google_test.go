package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

func TestCallGoogleGeminiGenerateContent(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotAuth string
	var gotBody map[string]interface{}
	inputDataURL := buildTinyPNGDataURL(t)
	inputBase64 := strings.TrimPrefix(inputDataURL, "data:image/png;base64,")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "google-key",
		Models:   []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
	}
	params := map[string]interface{}{
		"image_urls": []interface{}{inputDataURL},
	}

	got, err := c.callGoogleAIForTest(context.Background(), account, "draw a cat", 2*time.Second, params, "gemini-2.5-flash-image")
	if err != nil {
		t.Fatalf("callGoogleAI error: %v", err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAPIKey != "google-key" {
		t.Fatalf("unexpected x-goog-api-key: %q", gotAPIKey)
	}
	if gotAuth != "" {
		t.Fatalf("did not expect Authorization header, got %q", gotAuth)
	}
	if got.ImageURL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("unexpected image url: %s", got.ImageURL)
	}

	contents := gotBody["contents"].([]interface{})
	parts := contents[0].(map[string]interface{})["parts"].([]interface{})
	if len(parts) != 2 {
		t.Fatalf("expected prompt+inlineData parts, got %d", len(parts))
	}
	inline := parts[1].(map[string]interface{})["inlineData"].(map[string]interface{})
	if inline["mimeType"] != "image/png" || inline["data"] != inputBase64 {
		t.Fatalf("unexpected inlineData: %#v", inline)
	}
}

func TestCallGoogleGeminiGenerateContentAcceptsVersionedBaseURL(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL + "/v1beta",
		ApiKey:   "google-key",
	}

	_, err := c.callGoogleAIForTest(context.Background(), account, "draw a cat", 2*time.Second, nil, "gemini-2.5-flash-image")
	if err != nil {
		t.Fatalf("callGoogleAI error: %v", err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestCallGoogleCloudGeminiGenerateContent(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotQueryKey string
	var gotBody googleGeminiRequest
	outputData := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotQueryKey = r.URL.Query().Get("key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + outputData + `"}}]}}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{ProxyURL: server.URL, ApiKey: "google-cloud-key"}
	got, err := c.callGoogleCloudForTest(
		context.Background(),
		account,
		"draw a poster",
		2*time.Second,
		nil,
		"gemini-3.1-flash-image",
	)
	if err != nil {
		t.Fatalf("callGoogleCloud error: %v", err)
	}
	if gotPath != "/v1/publishers/google/models/gemini-3.1-flash-image:generateContent" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAPIKey != "" {
		t.Fatalf("x-goog-api-key = %q, want empty", gotAPIKey)
	}
	if gotQueryKey != "google-cloud-key" {
		t.Fatalf("key query = %q", gotQueryKey)
	}
	if len(gotBody.Contents) != 1 || gotBody.Contents[0].Role != "user" {
		t.Fatalf("unexpected contents: %#v", gotBody.Contents)
	}
	if strings.Join(gotBody.GenerationConfig.ResponseModalities, ",") != "TEXT,IMAGE" {
		t.Fatalf("response modalities = %#v", gotBody.GenerationConfig.ResponseModalities)
	}
	if len(got.ImageURLs) != 1 || !strings.HasPrefix(got.ImageURLs[0], "data:image/png;base64,") {
		t.Fatalf("image urls = %#v", got.ImageURLs)
	}
}

func TestGoogleCloudRejectsImagenModel(t *testing.T) {
	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{ProxyURL: "https://aiplatform.googleapis.com", ApiKey: "google-cloud-key"}
	_, err := c.callGoogleCloudForTest(
		context.Background(),
		account,
		"draw a poster",
		2*time.Second,
		nil,
		"imagen-4.0-generate-001",
	)
	if err == nil || !strings.Contains(err.Error(), "仅支持 Gemini 模型") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildGoogleModelEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "root base",
			baseURL: "https://generativelanguage.googleapis.com",
			want:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-image:generateContent",
		},
		{
			name:    "versioned base",
			baseURL: "https://generativelanguage.googleapis.com/v1beta",
			want:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-image:generateContent",
		},
		{
			name:    "gateway prefix versioned base",
			baseURL: "https://api.example.com/google/v1beta/",
			want:    "https://api.example.com/google/v1beta/models/gemini-2.5-flash-image:generateContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGoogleModelEndpoint(tt.baseURL, "gemini-2.5-flash-image", "generateContent")
			if got != tt.want {
				t.Fatalf("endpoint = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCollectGoogleImageRefsIncludesReferenceImageURLs(t *testing.T) {
	refs := collectProviderMediaRefs(map[string]interface{}{
		"image_urls": []interface{}{
			"data:image/png;base64,aGVsbG8=",
		},
		"original_image_url": "data:image/png;base64,aGVsbG8=",
		"mask_image_url":     "https://img.example/mask.png",
		"mask_image_urls": []interface{}{
			"https://img.example/mask-list.png",
			"https://img.example/mask.png",
		},
		"reference_image_urls": []interface{}{
			"https://img.example/ref-a.png",
			"data:image/png;base64,aGVsbG8=",
			"",
			"https://img.example/ref-b.png",
		},
	}).GoogleImages
	want := []string{
		"data:image/png;base64,aGVsbG8=",
		"https://img.example/ref-a.png",
		"https://img.example/ref-b.png",
		"https://img.example/mask-list.png",
		"https://img.example/mask.png",
	}
	if strings.Join(refs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected refs: got=%#v want=%#v", refs, want)
	}
}

func TestReadGoogleProviderParamsDerivesModelRefsAndImagenControls(t *testing.T) {
	geminiReq := newProviderRequest("prompt", 0, 0, map[string]interface{}{
		"model":                "models/gemini-2.5-flash-image",
		"image_urls":           []interface{}{"https://img.example/source.png"},
		"reference_image_urls": []interface{}{"https://img.example/ref.png"},
	}, "")
	geminiParams := readGoogleProviderParams(geminiReq)
	if geminiParams.Model != "gemini-2.5-flash-image" {
		t.Fatalf("gemini model = %q", geminiParams.Model)
	}
	if geminiParams.UseImagen {
		t.Fatalf("gemini model should not use Imagen")
	}
	wantRefs := []string{"https://img.example/source.png", "https://img.example/ref.png"}
	if strings.Join(geminiParams.ImageRefs, "\n") != strings.Join(wantRefs, "\n") {
		t.Fatalf("gemini refs = %#v", geminiParams.ImageRefs)
	}

	imagenReq := newProviderRequest("prompt", 0, 0, map[string]interface{}{
		"output_count": 9,
		"ratio":        "16:9",
	}, "models/imagen-4.0-generate-001")
	imagenParams := readGoogleProviderParams(imagenReq)
	if imagenParams.Model != "imagen-4.0-generate-001" {
		t.Fatalf("imagen model = %q", imagenParams.Model)
	}
	if !imagenParams.UseImagen {
		t.Fatalf("imagen model should use Imagen")
	}
	if imagenParams.SampleCount != 4 {
		t.Fatalf("sample count = %d, want 4", imagenParams.SampleCount)
	}
	if imagenParams.AspectRatio != "16:9" {
		t.Fatalf("aspect ratio = %q, want 16:9", imagenParams.AspectRatio)
	}
}

func TestCallGoogleImagenPredict(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[{"bytesBase64Encoded":"YQ=="},{"image":{"bytesBase64Encoded":"Yg=="}}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{ProxyURL: server.URL, ApiKey: "google-key"}
	params := map[string]interface{}{
		"model":        "imagen-4.0-generate-001",
		"output_count": 9,
		"ratio":        "16:9",
	}

	got, err := c.callGoogleAIForTest(context.Background(), account, "draw skyline", 2*time.Second, params, "")
	if err != nil {
		t.Fatalf("callGoogleAI error: %v", err)
	}
	if gotPath != "/v1beta/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAPIKey != "google-key" {
		t.Fatalf("unexpected x-goog-api-key: %q", gotAPIKey)
	}
	if gotAuth != "" {
		t.Fatalf("did not expect Authorization header, got %q", gotAuth)
	}
	if len(got.ImageURLs) != 2 || !strings.HasPrefix(got.ImageURLs[0], "data:image/png;base64,") || !strings.HasPrefix(got.ImageURLs[1], "data:image/png;base64,") {
		t.Fatalf("unexpected image urls: %#v", got.ImageURLs)
	}

	parameters := gotBody["parameters"].(map[string]interface{})
	if parameters["sampleCount"] != float64(4) {
		t.Fatalf("expected sampleCount capped at 4, got %#v", parameters["sampleCount"])
	}
	if parameters["aspectRatio"] != "16:9" {
		t.Fatalf("unexpected aspectRatio: %#v", parameters["aspectRatio"])
	}
}

func TestCallGoogleImagenPredictAcceptsVersionedBaseURL(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[{"bytesBase64Encoded":"YQ=="}]}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL + "/v1beta",
		ApiKey:   "google-key",
	}
	params := map[string]interface{}{
		"model": "imagen-4.0-generate-001",
	}

	_, err := c.callGoogleAIForTest(context.Background(), account, "draw skyline", 2*time.Second, params, "")
	if err != nil {
		t.Fatalf("callGoogleAI error: %v", err)
	}
	if gotPath != "/v1beta/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestCallDrawingAPIRoutesGooglePlatformToNativeProvider(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotAuth string
	outputData := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + outputData + `"}}]}}]}`))
	}))
	defer server.Close()

	c := newTestProviderExecutorWithLocalStorage(t)
	account := &accounts.ApiAccount{
		ID:       1,
		Name:     "google-native",
		ApiType:  workflow.ProviderGoogleAI,
		ProxyURL: server.URL,
		ApiKey:   "google-key",
		Models:   []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
	}
	task := &tasks.TaskPayload{
		TaskID: "task-google-native",
		UserID: 42,
		Prompt: "draw a cat",
		Params: `{"feature":"pattern-gen"}`,
	}

	output, err := c.callDrawingAPIForTest(context.Background(), account, task, "gemini-2.5-flash-image")
	if err != nil {
		t.Fatalf("callDrawingAPI error: %v", err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotAPIKey != "google-key" {
		t.Fatalf("unexpected x-goog-api-key: %q", gotAPIKey)
	}
	if gotAuth != "" {
		t.Fatalf("did not expect Authorization header, got %q", gotAuth)
	}
	if !strings.Contains(output.Result, `"/uploads/tasks/42/task-google-native/`) {
		t.Fatalf("expected google image_urls result, got %s", output.Result)
	}
}

func TestCallDrawingAPIRoutesGoogleCloudToNativeProvider(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotQueryKey string
	outputData := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotQueryKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + outputData + `"}}]}}]}`))
	}))
	defer server.Close()

	c := newTestProviderExecutorWithLocalStorage(t)
	account := &accounts.ApiAccount{
		ID:       1,
		Name:     "google-cloud",
		ApiType:  workflow.ProviderGoogleCloud,
		ProxyURL: server.URL,
		ApiKey:   "google-cloud-key",
		Models:   []accounts.AccountModel{{Name: "gemini-3.1-flash-image", Enabled: true}},
	}
	task := &tasks.TaskPayload{
		TaskID: "task-google-cloud",
		UserID: 42,
		Prompt: "draw a poster",
		Params: `{"feature":"pattern-gen"}`,
	}

	output, err := c.callDrawingAPIForTest(context.Background(), account, task, "gemini-3.1-flash-image")
	if err != nil {
		t.Fatalf("callDrawingAPI error: %v", err)
	}
	if gotPath != "/v1/publishers/google/models/gemini-3.1-flash-image:generateContent" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAPIKey != "" {
		t.Fatalf("x-goog-api-key = %q, want empty", gotAPIKey)
	}
	if gotQueryKey != "google-cloud-key" {
		t.Fatalf("key query = %q", gotQueryKey)
	}
	if !strings.Contains(output.Result, `"/uploads/tasks/42/task-google-cloud/`) {
		t.Fatalf("expected materialized image result, got %s", output.Result)
	}
}
