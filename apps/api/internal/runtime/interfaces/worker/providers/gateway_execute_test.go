package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aigatewayports "ideaboard/api/internal/aigateway/ports"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/dialects"
	providerinfra "ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

func gatewayTestFactory(serverURL string) runtimeports.GatewayClientFactory {
	return func(context.Context) (runtimeports.GatewayClient, error) {
		return providerinfra.NewGatewayClient(providerinfra.GatewayClientConfig{BaseURL: serverURL, Token: "test-token"})
	}
}

type stubRouteOverrideReader struct {
	path   string
	params string
	found  bool
	err    error
	called bool
	// last lookup key: the structured pool model (provider plus bare
	// model), never a combined-string inference.
	gotProvider string
	gotModel    string
}

func (s *stubRouteOverrideReader) LookupRouteOverride(_ context.Context, provider, model string) (string, string, bool, error) {
	s.called = true
	s.gotProvider, s.gotModel = provider, model
	return s.path, s.params, s.found, s.err
}

func TestCallGatewayAPIAppliesCatalogRouteOverride(t *testing.T) {
	// C2 回归：目录 custom_path/custom_params 必须到达执行，且 customParams
	// 参与 chat 判定（旧分裂：预览 chat、执行 generations）。
	rawImagePayload := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + rawImagePayload + `"}}]}}]}`))
	}))
	defer server.Close()

	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-catalog/result.png",
				Output: tasks.TaskOutput{
					TaskID: "task-catalog",
					Role:   "image",
					URL:    "/uploads/tasks/42/task-catalog/result.png",
				},
			}},
		},
	}
	reader := &stubRouteOverrideReader{path: "custom/endpoint/v2", params: `{"type":"chat"}`, found: true}
	executor := &ProviderExecutor{
		resultMaterializer: materializer,
		gatewayFactory:     gatewayTestFactory(server.URL),
		modelRoutes:        reader,
	}
	task := &tasks.TaskPayload{
		TaskID: "task-catalog",
		UserID: 42,
		Prompt: "catalog override",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}

	_, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "openai/dall-e-3", ExecutionToken: "tok-1", AttemptNo: 1,
	})
	if err != nil {
		t.Fatalf("CallGatewayAPI returned error: %v", err)
	}
	if !reader.called {
		t.Fatal("catalog route override reader was not consulted")
	}
	if reader.gotProvider != "openai" || reader.gotModel != "dall-e-3" {
		t.Fatalf("override lookup key = %q/%q, want structured openai/dall-e-3", reader.gotProvider, reader.gotModel)
	}
	if gotPath != "/openai/custom/endpoint/v2" {
		t.Fatalf("gateway path = %q, want catalog override route", gotPath)
	}
	var chatBody struct {
		Modalities []string `json:"modalities"`
	}
	if err := json.Unmarshal(gotBody, &chatBody); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(chatBody.Modalities) != 2 {
		t.Fatalf("expected chat payload via catalog customParams, body = %s", gotBody)
	}
}

func TestCallGatewayAPIFailsClosedOnRouteOverrideError(t *testing.T) {
	reader := &stubRouteOverrideReader{err: errors.New("catalog down")}
	executor := &ProviderExecutor{
		gatewayFactory: gatewayTestFactory("http://127.0.0.1:1"),
		modelRoutes:    reader,
	}
	task := &tasks.TaskPayload{
		TaskID: "task-route-err",
		UserID: 42,
		Prompt: "x",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}
	_, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "openai/dall-e-3", ExecutionToken: "tok-1", AttemptNo: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "读取模型路由配置失败") {
		t.Fatalf("expected route-config failure, got %v", err)
	}
}

func TestCallGatewayAPIStoresParsedOutputEnvelope(t *testing.T) {
	rawImagePayload := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set(aigatewayports.GatewayHeaderModel, "openrouter/google/actual-image-model")
		w.Header().Set(aigatewayports.GatewayHeaderStep, "provider-1")
		w.Header().Set(aigatewayports.GatewayHeaderCacheStatus, "HIT")
		w.Header().Set(aigatewayports.GatewayHeaderLogID, "log-1")
		w.Header().Set(aigatewayports.GatewayHeaderEventID, "event-1")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + rawImagePayload + `"}}]}}]}`))
	}))
	defer server.Close()

	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-prompt/result.png",
				Output: tasks.TaskOutput{
					TaskID: "task-prompt",
					Role:   "image",
					URL:    "/uploads/tasks/42/task-prompt/result.png",
				},
			}},
		},
	}
	executor := &ProviderExecutor{
		resultMaterializer: materializer,
		gatewayFactory:     gatewayTestFactory(server.URL),
	}
	task := &tasks.TaskPayload{
		TaskID: "task-prompt",
		UserID: 42,
		Prompt: "repeatable textile pattern",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}

	output, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "openrouter/google/gemini-3.1-flash-lite-image", ExecutionToken: "tok-1", AttemptNo: 1,
	})
	if err != nil {
		t.Fatalf("CallGatewayAPI returned error: %v", err)
	}
	if gotPath != "/openrouter/api/v1/chat/completions" {
		t.Fatalf("gateway path = %q, want openrouter chat route", gotPath)
	}
	var chatBody struct {
		Model      string   `json:"model"`
		Modalities []string `json:"modalities"`
	}
	if err := json.Unmarshal(gotBody, &chatBody); err != nil {
		t.Fatalf("decode chat body: %v", err)
	}
	if chatBody.Model != "google/gemini-3.1-flash-lite-image" {
		t.Fatalf("chat model = %q", chatBody.Model)
	}
	if len(chatBody.Modalities) != 2 || chatBody.Modalities[1] != "image" {
		t.Fatalf("modalities = %q", chatBody.Modalities)
	}
	if len(output.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(output.Outputs))
	}
	if output.ResolvedModel != "openrouter/google/actual-image-model" {
		t.Fatalf("resolved model = %q", output.ResolvedModel)
	}
	if output.GatewayStep != "provider-1" || !output.CacheHit || output.ReceiptLogID != "log-1" || output.ReceiptEventID != "event-1" {
		t.Fatalf("gateway receipt = %+v", output)
	}
	if len(materializer.remoteSourceURLs) != 1 || !strings.HasPrefix(materializer.remoteSourceURLs[0], "data:image/png;base64,") {
		t.Fatalf("unexpected materializer source URLs: %#v", materializer.remoteSourceURLs)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != tasks.ResultStatusSuccess {
		t.Fatalf("status = %#v, want success", result["status"])
	}
	if result["parsed_output_status"] != parsedOutputStatusValid {
		t.Fatalf("parsed_output_status = %#v, want valid", result["parsed_output_status"])
	}
	if result["image_url"] != "/uploads/tasks/42/task-prompt/result.png" {
		t.Fatalf("image_url = %#v", result["image_url"])
	}
	if result["model"] != "openrouter/google/gemini-3.1-flash-lite-image" {
		t.Fatalf("model = %#v", result["model"])
	}
	rawPreview, _ := result["raw_provider_output_preview"].(string)
	if rawPreview == "" {
		t.Fatal("raw_provider_output_preview should be present")
	}
	if strings.Contains(rawPreview, rawImagePayload) {
		t.Fatalf("raw preview should redact image payload, got %s", rawPreview)
	}
}

func TestCallGatewayAPIOpenRouterResolvesRelativeUploads(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)
	executor.publicOrigins = config.PublicOriginsConfig{Web: "https://api.example.com"}
	executor.gatewayFactory = gatewayTestFactory("http://127.0.0.1:1")
	source := buildTinyPNGBytes(t)
	sourcePath := filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "source.png")
	relativeRef := "/" + filepath.ToSlash(sourcePath)
	executor.mediaResolver = testRuntimeMediaResolver{urls: map[int64]string{1: relativeRef, 2: relativeRef}}
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatalf("write source upload: %v", err)
	}

	rawParams := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Service:       "variations",
		Media: &tasks.MediaInputs{
			Inputs:   []tasks.MediaRef{{AssetID: 1, Role: "input"}},
			Original: &tasks.MediaRef{AssetID: 2, Role: "original"},
		},
		Variation: &tasks.VariationsParams{Strength: 40},
	})
	task := &tasks.TaskPayload{TaskID: "task-openrouter-storage-input", Prompt: "prompt", Params: rawParams}
	parsedParams, err := executor.parseAndNormalizeTaskParams(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	parsedParams, err = executor.resolveRuntimeProviderTaskParams(context.Background(), parsedParams)
	if err != nil {
		t.Fatal(err)
	}
	providerReq, err := parsedParams.providerRequest(task.Prompt, "google/gemini-3.1-flash-lite-image")
	if err != nil {
		t.Fatal(err)
	}
	imageURLs, err := resolveDialectImageURLs(executor, providerReq)
	if err != nil {
		t.Fatalf("resolveDialectImageURLs returned error: %v", err)
	}
	route, err := dialects.Resolve(workflow.RuntimeOperationRender, "openrouter", "google/gemini-3.1-flash-lite-image", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	dialect := route.Dialect
	body, err := dialect.Encode(dialects.Request{
		Prompt:    task.Prompt,
		Model:     "google/gemini-3.1-flash-lite-image",
		Operation: workflow.RuntimeOperationRender,
		Images:    imageURLs,
	})
	if err != nil {
		t.Fatalf("dialect Encode returned error: %v", err)
	}
	var gotRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL *struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		} `json:"messages"`
		Modalities []string `json:"modalities"`
	}
	if err := json.Unmarshal(body, &gotRequest); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if gotRequest.Model != "google/gemini-3.1-flash-lite-image" {
		t.Fatalf("model = %q", gotRequest.Model)
	}
	if len(gotRequest.Modalities) != 2 || gotRequest.Modalities[1] != "image" {
		t.Fatalf("modalities = %q", gotRequest.Modalities)
	}
	if len(gotRequest.Messages) != 1 || len(gotRequest.Messages[0].Content) < 2 {
		t.Fatalf("expected text plus image_url parts, got %#v", gotRequest.Messages)
	}
	if gotRequest.Messages[0].Content[0].Type != "text" || gotRequest.Messages[0].Content[0].Text != "prompt" {
		t.Fatalf("text part = %#v", gotRequest.Messages[0].Content[0])
	}
	imagePart := gotRequest.Messages[0].Content[1]
	if imagePart.Type != "image_url" || imagePart.ImageURL == nil {
		t.Fatalf("image part = %#v", imagePart)
	}
	if !strings.HasPrefix(imagePart.ImageURL.URL, "https://api.example.com/uploads/") {
		t.Fatalf("image url = %q, want public uploads URL", imagePart.ImageURL.URL)
	}
}

func TestCallGatewayAPIRejectsUnknownModel(t *testing.T) {
	executor := &ProviderExecutor{gatewayFactory: gatewayTestFactory("http://127.0.0.1:1")}
	task := &tasks.TaskPayload{
		TaskID: "task-bad-model",
		Prompt: "x",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}
	_, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "no-slash-model", ExecutionToken: "tok", AttemptNo: 1,
	})
	if err == nil {
		t.Fatal("unqualified model accepted, want error")
	}
}

func TestCallGatewayAPIUnknownSlugUsesCustomNamespace(t *testing.T) {
	// Unknown registry slugs keep the synthetic custom-<slug> namespace
	// (Control API contract); the native body carries the bare model.
	rawImagePayload := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
	var gotPath string
	var gotBody struct {
		Model string `json:"model"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + rawImagePayload + `"}}]}}]}`))
	}))
	defer server.Close()

	executor := &ProviderExecutor{
		resultMaterializer: &captureResultMaterializer{
			remoteResult: taskmedia.MaterializedResults{
				Items: []taskmedia.MaterializedResult{{
					ImageURL: "/uploads/tasks/42/task-custom/result.png",
					Output: tasks.TaskOutput{
						TaskID: "task-custom",
						Role:   "image",
						URL:    "/uploads/tasks/42/task-custom/result.png",
					},
				}},
			},
		},
		gatewayFactory: gatewayTestFactory(server.URL),
	}
	task := &tasks.TaskPayload{
		TaskID: "task-custom",
		UserID: 42,
		Prompt: "unknown provider model",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}
	if _, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "myco/custom-model", ExecutionToken: "tok-custom", AttemptNo: 1,
	}); err != nil {
		t.Fatalf("CallGatewayAPI returned error: %v", err)
	}
	if gotPath != "/custom-myco/v1/images/generations" {
		t.Fatalf("gateway path = %q, want /custom-myco/v1/images/generations", gotPath)
	}
	if gotBody.Model != "custom-model" {
		t.Fatalf("native body model = %q, want bare custom-model", gotBody.Model)
	}
}

func TestCallGatewayAPICutoutSupportsJSONAndBinary(t *testing.T) {
	// 1. Test JSON response for cutout
	serverJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"image": {"url": "https://example.com/cutout_result.png"}}`))
	}))
	defer serverJSON.Close()

	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-cutout/result.png",
				Output: tasks.TaskOutput{
					TaskID: "task-cutout",
					Role:   "image",
					URL:    "/uploads/tasks/42/task-cutout/result.png",
				},
			}},
		},
	}
	resolver := testRuntimeMediaResolver{urls: map[int64]string{1: "https://example.com/input.png"}}
	executorJSON := &ProviderExecutor{
		mediaResolver:      resolver,
		resultMaterializer: materializer,
		gatewayFactory:     gatewayTestFactory(serverJSON.URL),
	}
	taskJSON := &tasks.TaskPayload{
		TaskID: "task-cutout",
		UserID: 42,
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugCutout,
			Media: &tasks.MediaInputs{
				Inputs: []tasks.MediaRef{{AssetID: 1, Role: "input"}},
			},
		}),
	}

	result, err := executorJSON.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task:           taskJSON,
		Model:          "fal/birefnet@fal-ai/birefnet",
		ExecutionToken: "tok-2",
		AttemptNo:      1,
	})
	if err != nil {
		t.Fatalf("JSON cutout failed: %v", err)
	}
	if len(result.Outputs) != 1 || result.Outputs[0].URL != "/uploads/tasks/42/task-cutout/result.png" {
		t.Fatalf("expected materialized cutout output, got %#v", result.Outputs)
	}

	// 2. Test binary stream response for upscale
	tinyPNG := buildTinyPNGBytes(t)
	serverBinary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG)
	}))
	defer serverBinary.Close()

	executorBinary := &ProviderExecutor{
		mediaResolver:      resolver,
		resultMaterializer: materializer,
		gatewayFactory:     gatewayTestFactory(serverBinary.URL),
	}
	taskUpscale := &tasks.TaskPayload{
		TaskID: "task-upscale",
		UserID: 42,
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugUpscale,
			Media: &tasks.MediaInputs{
				Inputs: []tasks.MediaRef{{AssetID: 1, Role: "input"}},
			},
			Upscale: &tasks.UpscaleParams{
				Scale: 2.0,
			},
		}),
	}
	resultBinary, err := executorBinary.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task:           taskUpscale,
		Model:          "fal/aura-sr@fal-ai/aura-sr",
		ExecutionToken: "tok-3",
		AttemptNo:      1,
	})
	if err != nil {
		t.Fatalf("Binary upscale failed: %v", err)
	}
	if len(resultBinary.Outputs) != 1 || resultBinary.Outputs[0].URL != "/uploads/tasks/42/task-cutout/result.png" {
		t.Fatalf("expected materialized upscale output, got %#v", resultBinary.Outputs)
	}
}
