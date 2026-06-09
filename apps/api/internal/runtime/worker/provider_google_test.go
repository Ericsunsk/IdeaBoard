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

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/runtime/queuetypes"
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

	got, err := c.callGoogleAI(context.Background(), account, "draw a cat", 2*time.Second, params, "gemini-2.5-flash-image")
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

func TestCollectGoogleImageRefsIncludesReferenceImageURLs(t *testing.T) {
	refs := collectGoogleImageRefs(map[string]interface{}{
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
	})
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

	got, err := c.callGoogleAI(context.Background(), account, "draw skyline", 2*time.Second, params, "")
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
		Platform: "custom display name",
		ApiType:  workflow.ProviderGoogleAI,
		ProxyURL: server.URL,
		ApiKey:   "google-key",
		Models:   []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
	}
	task := &queuetypes.TaskPayload{
		TaskID: "task-google-native",
		UserID: 42,
		Prompt: "draw a cat",
		Params: `{"feature":"pattern-gen"}`,
	}

	output, err := c.callDrawingAPI(context.Background(), account, task)
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
