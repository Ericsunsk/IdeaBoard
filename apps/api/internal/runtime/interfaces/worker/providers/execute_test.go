package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workertransfer"
	"ideaboard/api/internal/media/workspacemedia"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"
	"ideaboard/api/internal/prompts"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	providerinfra "ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

// fakeWorkerTransfer uses the real slot implementation with in-memory asset
// and object adapters, so executor tests cannot bypass acceptance validation.
type fakeWorkerTransfer struct {
	workertransfer.WorkerTransfer
	origin string
	secret string
	assets map[string]fakeWorkerAsset
	puts   map[string][]byte
	putCT  map[string]string
}

type fakeWorkerAsset struct {
	key  string
	mime string
}

func newFakeWorkerTransfer(origin string) *fakeWorkerTransfer {
	f := &fakeWorkerTransfer{
		origin: origin,
		secret: "test-secret",
		assets: map[string]fakeWorkerAsset{
			"asset_input1": {key: "uploads/ws/test/asset_input1/input.png", mime: "image/png"},
			"asset_mask1":  {key: "uploads/ws/test/asset_mask1/mask.png", mime: "image/png"},
		},
		puts:  map[string][]byte{},
		putCT: map[string]string{},
	}
	f.WorkerTransfer = workertransfer.NewWorkerTransfer(f, f, origin, f.secret)
	return f
}
func (f *fakeWorkerTransfer) GetAsset(_ context.Context, _, publicAssetID string) (*workspacemedia.Asset, error) {
	asset, ok := f.assets[strings.TrimSpace(publicAssetID)]
	if !ok {
		return nil, errors.New("fake: unknown input asset")
	}
	return &workspacemedia.Asset{StorageKey: asset.key, MIMEType: asset.mime}, nil
}

func (f *fakeWorkerTransfer) ReadObjectAtProfile(_ context.Context, _ string, key string, _ int64) (*storageports.StoredUploadObject, error) {
	raw, ok := f.puts[key]
	if !ok {
		return nil, errors.New("fake: landed object missing")
	}
	return &storageports.StoredUploadObject{ObjectKey: key, Data: raw, ContentType: f.putCT[key]}, nil
}

// fakeWorkerHTTPObjects backs the real media HTTP handler in tests.
type fakeWorkerHTTPObjects struct {
	data map[string][]byte
	ct   map[string]string
}

func (f *fakeWorkerHTTPObjects) ReadObjectAtProfile(_ context.Context, _ string, key string, _ int64) (*storageports.StoredUploadObject, error) {
	raw, ok := f.data[key]
	if !ok {
		return nil, errors.New("fake: object not found")
	}
	return &storageports.StoredUploadObject{ObjectKey: key, ContentType: f.ct[key], Data: raw}, nil
}

func (f *fakeWorkerHTTPObjects) PutObjectAtProfile(_ context.Context, _ string, key string, data []byte, contentType string) error {
	if f.data == nil {
		f.data = map[string][]byte{}
	}
	if f.ct == nil {
		f.ct = map[string]string{}
	}
	f.data[key] = append([]byte(nil), data...)
	f.ct[key] = contentType
	return nil
}

// fakeWorkerServer serves the real Worker contract plus the real media
// handlers on one loopback origin: POST /v1/generate validates the
// CanonicalImageRequest, fetches presigned inputs, PUTs outputs, and
// returns GenerateSuccess. It is the httptest stand-in for the
// ai-proxy during independent verification.
type fakeWorkerServer struct {
	t              *testing.T
	server         *httptest.Server
	transfer       *fakeWorkerTransfer
	capturedBody   map[string]interface{}
	capturedBodies []map[string]interface{}
	capturedAuth   string
	fetchedInputs  int
	respond        func(taskID, uploadURL string) (int, string)
	rawPNG         []byte
	outputType     string
}

func newFakeWorkerServer(t *testing.T, inputPNG []byte) *fakeWorkerServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fw := &fakeWorkerServer{t: t, rawPNG: inputPNG, outputType: "image/png", capturedBody: map[string]interface{}{}}
	objects := &fakeWorkerHTTPObjects{
		data: map[string][]byte{
			"uploads/ws/test/asset_input1/input.png": inputPNG,
			"uploads/ws/test/asset_mask1/mask.png":   inputPNG,
		},
		ct: map[string]string{
			"uploads/ws/test/asset_input1/input.png": "image/png",
			"uploads/ws/test/asset_mask1/mask.png":   "image/png",
		},
	}
	engine := gin.New()
	engine.POST("/v1/generate", fw.serveGenerate)
	h := runtimehttp.NewWorkerMediaHandler("test-secret", objects)
	engine.GET("/api/v1/worker/media/*key", h.ServeGET)
	engine.PUT("/api/v1/worker/media/*key", h.ServePUT)
	fw.server = httptest.NewServer(engine)
	fw.transfer = newFakeWorkerTransfer(fw.server.URL)
	// Seed the transfer's landed map with the handler's store so reads
	// observe real PUTs.
	fw.transfer.puts = objects.data
	fw.transfer.putCT = objects.ct
	return fw
}

func (fw *fakeWorkerServer) serveGenerate(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		c.String(http.StatusBadRequest, "invalid JSON")
		return
	}
	fw.capturedBody = decoded
	fw.capturedBodies = append(fw.capturedBodies, decoded)
	fw.capturedAuth = c.GetHeader("Authorization")
	taskID, _ := decoded["task_id"].(string)
	uploadURL, _ := decoded["upload_url"].(string)
	if fw.respond != nil {
		status, body := fw.respond(taskID, uploadURL)
		if status != 0 {
			c.Header("Content-Type", "application/json")
			c.String(status, body)
			return
		}
	}
	// Default success path: fetch every presigned input, PUT the output,
	// echo the landed URL.
	if fw.capturedAuth != "Bearer test-secret" {
		c.String(http.StatusUnauthorized, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"UNAUTHORIZED","message":"unauthorized"}}`)
		return
	}
	images, _ := decoded["images"].([]interface{})
	for _, item := range images {
		rawURL, _ := item.(string)
		resp, err := http.Get(rawURL)
		if err != nil {
			c.String(http.StatusBadRequest, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"INPUT_FETCH_FAILED","message":"fetch failed"}}`)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fw.fetchedInputs++
		if resp.StatusCode != http.StatusOK || len(body) == 0 {
			c.String(http.StatusBadRequest, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"INPUT_FETCH_FAILED","message":"fetch failed"}}`)
			return
		}
	}
	if strings.TrimSpace(uploadURL) == "" {
		c.String(http.StatusBadRequest, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"BAD_REQUEST","message":"upload_url is required"}}`)
		return
	}
	putReq, err := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(fw.rawPNG))
	if err != nil {
		c.String(http.StatusBadGateway, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"UPLOAD_FAILED","message":"put build failed"}}`)
		return
	}
	putReq.Header.Set("Content-Type", fw.outputType)
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		c.String(http.StatusBadGateway, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"UPLOAD_FAILED","message":"put failed"}}`)
		return
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		c.String(http.StatusBadGateway, `{"task_id":"`+taskID+`","status":"failed","error":{"code":"UPLOAD_FAILED","message":"put rejected"}}`)
		return
	}
	imageURL := uploadURL
	if idx := strings.Index(imageURL, "?"); idx >= 0 {
		imageURL = imageURL[:idx]
	}
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, `{"task_id":"`+taskID+`","status":"completed","provider":"openai","model":"gpt-image-1","image_url":"`+imageURL+`","content_type":"`+fw.outputType+`","receipt":{"upstream_status":200,"provider":"openai","model":"gpt-image-1"}}`)
}

type stubCustomParamsReader struct {
	params string
	found  bool
	err    error
	called bool
	// last lookup key: the structured pool model (provider plus bare
	// model), never a combined-string inference.
	gotProvider string
	gotModel    string
}

func (s *stubCustomParamsReader) LookupCustomParams(_ context.Context, provider, model string) (string, bool, error) {
	s.called = true
	s.gotProvider, s.gotModel = provider, model
	return s.params, s.found, s.err
}

func workerTestFactory(transfer *fakeWorkerTransfer) runtimeports.WorkerClientFactory {
	return func(context.Context) (runtimeports.WorkerClient, error) {
		return providerinfra.NewWorkerClient(providerinfra.WorkerClientConfig{BaseURL: transfer.origin, Secret: "test-secret"})
	}
}

func generationTaskForTest(t *testing.T, taskID string) *tasks.TaskPayload {
	t.Helper()
	return &tasks.TaskPayload{
		TaskID: taskID,
		UserID: 42,
		Prompt: "repeatable textile pattern",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}
}

func workerTestExecutor(fw *fakeWorkerServer, customParams runtimeports.ModelCustomParamsReader) *ProviderExecutor {
	return &ProviderExecutor{
		resultMaterializer: &captureResultMaterializer{
			remoteResult: taskmedia.MaterializedResults{
				Items: []taskmedia.MaterializedResult{{
					ImageURL: "/uploads/tasks/42/task-x/result.png",
					Output: tasks.TaskOutput{
						TaskID: "task-x",
						Role:   "image",
						URL:    "/uploads/tasks/42/task-x/result.png",
					},
				}},
			},
		},
		workerFactory: workerTestFactory(fw.transfer),
		customParams:  customParams,
		transfer:      fw.transfer,
	}
}

func TestCallWorkerAPISuccess(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	executor := workerTestExecutor(fw, nil)
	task := generationTaskForTest(t, "task-prompt")

	output, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
	})
	if err != nil {
		t.Fatalf("CallWorkerAPI returned error: %v", err)
	}
	// Single Worker exit: split provider/model facts, no path branching.
	if fw.capturedBody["provider"] != "openai" || fw.capturedBody["model"] != "dall-e-3" {
		t.Fatalf("provider/model = %v/%v, want split openai + dall-e-3", fw.capturedBody["provider"], fw.capturedBody["model"])
	}
	if fw.capturedBody["prompt"] != "repeatable textile pattern" {
		t.Fatalf("prompt = %v", fw.capturedBody["prompt"])
	}
	for _, banned := range []string{"outputs", "custom_params", "operation", "dry_run", "execution_token", "service", "feature"} {
		if _, ok := fw.capturedBody[banned]; ok {
			t.Fatalf("wire body carries legacy %q: %v", banned, fw.capturedBody)
		}
	}
	// Exact-key presigned PUT: key aligns with future Media scheme,
	// content-type bound, short TTL.
	uploadURL, _ := fw.capturedBody["upload_url"].(string)
	wantKey := platformworkermedia.MintOutputKey("", "task-prompt", 0)
	for _, want := range []string{"/api/v1/worker/media/" + wantKey, "ct=worker-image-v1", "exp=", "sig="} {
		if !strings.Contains(uploadURL, want) {
			t.Fatalf("upload_url = %q, want %q", uploadURL, want)
		}
	}
	// The fake Worker really PUT through the presigned URL: landed bytes
	// carry the bound content-type.
	if got := fw.transfer.putCT[wantKey]; got != "image/png" {
		t.Fatalf("landed content-type = %q, want image/png", got)
	}
	if output.ResolvedModel != "gpt-image-1" {
		t.Fatalf("resolved model = %q, want gpt-image-1", output.ResolvedModel)
	}
	if len(output.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(output.Outputs))
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
	if result["image_url"] != "/uploads/tasks/42/task-x/result.png" {
		t.Fatalf("image_url = %#v", result["image_url"])
	}
	if result["model"] != "openai/dall-e-3" {
		t.Fatalf("model = %#v", result["model"])
	}
	rawPreview, _ := result["raw_provider_output_preview"].(string)
	if rawPreview == "" {
		t.Fatal("raw_provider_output_preview should be present")
	}
	if strings.Contains(rawPreview, "base64,") {
		t.Fatalf("raw preview should redact image payload, got %s", rawPreview)
	}
}

func TestCallWorkerAPIWithInputsAndMask(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	executor := workerTestExecutor(fw, nil)
	task := &tasks.TaskPayload{
		TaskID: "task-inpaint",
		UserID: 42,
		Prompt: "fill the mask",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugInpaint,
			Media: &tasks.MediaInputs{
				Inputs: []tasks.MediaRef{{PublicAssetID: "asset_input1", Role: "input"}},
				Mask:   &tasks.MediaRef{PublicAssetID: "asset_mask1", Role: "mask"},
			},
		}),
	}

	if _, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/gpt-image-1", AttemptNo: 1,
	}); err != nil {
		t.Fatalf("CallWorkerAPI returned error: %v", err)
	}
	// Inputs first, mask last; the fake Worker fetched every presigned GET.
	images, _ := fw.capturedBody["images"].([]interface{})
	if len(images) != 2 {
		t.Fatalf("images = %v, want 2 presigned GETs", fw.capturedBody["images"])
	}
	if fw.capturedBody["mask_index"] != float64(1) {
		t.Fatalf("missing explicit mask role: %v", fw.capturedBody)
	}
	if fw.fetchedInputs != 2 {
		t.Fatalf("fetched inputs = %d, want 2", fw.fetchedInputs)
	}
	first, _ := images[0].(string)
	last, _ := images[1].(string)
	if !strings.Contains(first, "asset_input1") || !strings.Contains(last, "asset_mask1") {
		t.Fatalf("images order = %v, want input then mask", images)
	}
}

func TestCallWorkerAPIAppliesCatalogCustomParams(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	reader := &stubCustomParamsReader{params: `{"quality":"high"}`, found: true}
	executor := workerTestExecutor(fw, reader)
	task := generationTaskForTest(t, "task-catalog")

	if _, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
	}); err != nil {
		t.Fatalf("CallWorkerAPI returned error: %v", err)
	}
	if !reader.called {
		t.Fatal("catalog custom params reader was not consulted")
	}
	if reader.gotProvider != "openai" || reader.gotModel != "dall-e-3" {
		t.Fatalf("custom params lookup key = %q/%q, want structured openai/dall-e-3", reader.gotProvider, reader.gotModel)
	}
	options, _ := fw.capturedBody["provider_options"].(map[string]interface{})
	if options["quality"] != "high" {
		t.Fatalf("provider_options = %v, want catalog quality", fw.capturedBody["provider_options"])
	}
}

func TestCallWorkerAPIFailsClosedOnCustomParamsError(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	reader := &stubCustomParamsReader{err: errors.New("catalog down")}
	transfer := newFakeWorkerTransfer(fw.server.URL)
	executor := &ProviderExecutor{
		workerFactory: workerTestFactory(transfer),
		customParams:  reader,
		transfer:      transfer,
	}
	task := generationTaskForTest(t, "task-route-err")
	_, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "读取模型路由配置失败") {
		t.Fatalf("expected route-config failure, got %v", err)
	}
}

func TestCallWorkerAPIRejectsUnknownModel(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	executor := workerTestExecutor(fw, nil)
	task := generationTaskForTest(t, "task-bad-model")
	_, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "no-slash-model", AttemptNo: 1,
	})
	if err == nil {
		t.Fatal("unqualified model accepted, want error")
	}
}

func TestCallWorkerAPIRejectsPathOverride(t *testing.T) {
	// Legacy @path overrides have no Worker target: fail closed.
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	executor := workerTestExecutor(fw, nil)
	task := generationTaskForTest(t, "task-override")
	_, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/dall-e-3@v1/custom", AttemptNo: 1,
	})
	if err == nil {
		t.Fatal("path override accepted, want fail-closed")
	}
}

func TestCallWorkerAPIRejectsKeyMismatch(t *testing.T) {
	// The Worker must echo the exact presigned slot key.
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	fw.respond = func(taskID, uploadURL string) (int, string) {
		return http.StatusOK, `{"task_id":"` + taskID + `","status":"completed","provider":"openai","model":"gpt-image-1","image_url":"https://media.example/w/uploads/ws/unknown/` + taskID + `/evil.png","content_type":"image/png","receipt":{"upstream_status":200,"provider":"openai","model":"gpt-image-1"}}`
	}
	executor := workerTestExecutor(fw, nil)
	task := generationTaskForTest(t, "task-mismatch")
	_, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
		Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "presigned") {
		t.Fatalf("expected presigned key-mismatch failure, got %v", err)
	}
}

func TestCallWorkerAPISurfacesUpstreamErrors(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		category string
	}{
		{"bad request", 400, `{"task_id":"t","status":"failed","error":{"code":"BAD_REQUEST","message":"upload_url is required"}}`, tasks.FailureCategoryInputInvalid},
		{"unauthorized", 401, `{"task_id":"t","status":"failed","error":{"code":"UNAUTHORIZED","message":"unauthorized"}}`, tasks.FailureCategoryAuth},
		{"unsupported", 422, `{"task_id":"t","status":"failed","error":{"code":"UNSUPPORTED_MODEL","message":"queue/async"}}`, tasks.FailureCategoryInputInvalid},
		{"rate limited", 429, `{"task_id":"t","status":"failed","error":{"code":"UPSTREAM_ERROR","message":"busy","upstream_status":429}}`, tasks.FailureCategoryTimeout},
		{"server error", 500, `{"task_id":"t","status":"failed","error":{"code":"UPSTREAM_ERROR","message":"boom","upstream_status":500}}`, tasks.FailureCategoryProviderError},
		{"input fetch", 400, `{"task_id":"t","status":"failed","error":{"code":"INPUT_FETCH_FAILED","message":"fetch failed"}}`, tasks.FailureCategoryProviderError},
		{"upload", 502, `{"task_id":"t","status":"failed","error":{"code":"UPLOAD_FAILED","message":"put failed"}}`, tasks.FailureCategoryProviderError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			fw.respond = func(taskID, uploadURL string) (int, string) {
				return tc.status, tc.body
			}
			executor := workerTestExecutor(fw, nil)
			task := generationTaskForTest(t, "task-err")
			_, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
				Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
			})
			if err == nil {
				t.Fatalf("status %d accepted, want error", tc.status)
			}
			stage, category, ok := runtimeports.ExecutionFailureDetails(err)
			if !ok {
				t.Fatalf("expected execution failure details, got %T (%v)", err, err)
			}
			if stage != tasks.ExecutionStageProvider || category != tc.category {
				t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageProvider, tc.category)
			}
		})
	}
}

func TestCallWorkerUsesActualOutputMIME(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	for mime, data := range map[string][]byte{"image/jpeg": jpg.Bytes(), "image/webp": webp} {
		t.Run(mime, func(t *testing.T) {
			fw := newFakeWorkerServer(t, data)
			defer fw.server.Close()
			fw.outputType = mime
			executor := workerTestExecutor(fw, nil)
			result, err := executor.callWorker(context.Background(), "task-mime", "generation", "ws_test", "fal", "flux", "p", providerTaskParams{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
			if result.ImageURL != want {
				t.Fatalf("output data URL MIME or bytes mismatch: %s", result.ImageURL)
			}
			key := platformworkermedia.MintOutputKey("ws_test", "task-mime", 0)
			if fw.transfer.putCT[key] != mime {
				t.Fatalf("stored MIME %q", fw.transfer.putCT[key])
			}
		})
	}
}

func TestCallWorkerRejectsTruncatedLandedImages(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	for mime, data := range map[string][]byte{"image/png": buildTinyPNGBytes(t)[:33], "image/jpeg": jpg.Bytes()[:jpg.Len()-10], "image/webp": webp[:len(webp)-3]} {
		t.Run(mime, func(t *testing.T) {
			// The attack retains a complete dimension header but omits image body.
			if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
				t.Fatalf("fixture has no complete config: %v", err)
			}
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			key := platformworkermedia.MintOutputKey("ws_test", "truncated", 0)
			fw.transfer.puts[key] = data
			fw.transfer.putCT[key] = mime
			fw.respond = func(taskID, uploadURL string) (int, string) {
				return 200, `{"task_id":"` + taskID + `","status":"completed","image_url":"` + strings.Split(uploadURL, "?")[0] + `","content_type":"` + mime + `"}`
			}
			executor := workerTestExecutor(fw, nil)
			_, err := executor.callWorker(context.Background(), "truncated", "generation", "ws_test", "fal", "flux", "p", providerTaskParams{}, nil)
			if err == nil {
				t.Fatal("truncated landed image accepted")
			}
		})
	}
}

func TestCallWorkerAPIExecutesRequestedOutputs(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			executor := workerTestExecutor(fw, nil)
			items := make([]taskmedia.MaterializedResult, count)
			for i := range items {
				items[i] = taskmedia.MaterializedResult{ImageURL: fmt.Sprintf("/output/%d.png", i), Output: tasks.TaskOutput{AssetID: fmt.Sprintf("asset_%d", i), Role: "image", ResultIndex: i}}
			}
			executor.resultMaterializer = &captureResultMaterializer{remoteResult: taskmedia.MaterializedResults{Items: items}}
			task := generationTaskForTest(t, "task-multiple")
			task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: workflow.FeatureSlugGeneration, Controls: &tasks.GenerationControls{OutputCount: count}, Generation: &tasks.GenerationRequestSnapshot{Seed: ^uint64(0)}})
			output, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{Task: task, Model: "openai/gpt-image-1"})
			if err != nil {
				t.Fatal(err)
			}
			if len(output.Outputs) != count || len(fw.capturedBodies) != count {
				t.Fatalf("outputs=%d calls=%d, want %d", len(output.Outputs), len(fw.capturedBodies), count)
			}
			slots := map[string]bool{}
			for i, request := range fw.capturedBodies {
				wantSeeds := []float64{4294967295, 0, 1, 2}
				wantSeed := wantSeeds[i]
				if count == 1 {
					wantSeed = float64(^uint64(0))
				}
				if request["seed"] != wantSeed {
					t.Fatalf("wire seed[%d]=%v, want %v", i, request["seed"], wantSeed)
				}
				key, err := platformworkermedia.ParseKey(request["upload_url"].(string))
				if err != nil {
					t.Fatal(err)
				}
				if slots[key] {
					t.Fatalf("output slot reused: %s", key)
				}
				slots[key] = true
				if !strings.HasSuffix(key, fmt.Sprintf("/result-%d.png", i)) {
					t.Fatalf("output %d key=%q", i, key)
				}
				if output.Outputs[i].ResultIndex != i {
					t.Fatalf("result index=%d, want %d", output.Outputs[i].ResultIndex, i)
				}
			}
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
				t.Fatal(err)
			}
			if result["parsed_output"].(map[string]interface{})["image_count"] != float64(count) {
				t.Fatalf("result=%s", output.Result)
			}
		})
	}
}

func TestCallWorkerAPIKeepsOutputsBeforeLaterFailure(t *testing.T) {
	for _, successful := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(successful), func(t *testing.T) {
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			fw.respond = func(taskID, uploadURL string) (int, string) {
				if len(fw.capturedBodies) == successful+1 {
					return 503, fmt.Sprintf(`{"task_id":%q,"status":"failed","error":{"code":"UPSTREAM_ERROR","message":"unavailable"}}`, taskID)
				}
				return 0, ""
			}
			executor := workerTestExecutor(fw, nil)
			items := make([]taskmedia.MaterializedResult, successful)
			for i := range items {
				items[i] = taskmedia.MaterializedResult{ImageURL: fmt.Sprintf("/output/%d.png", i), Output: tasks.TaskOutput{AssetID: fmt.Sprintf("asset_%d", i), Role: "image", ResultIndex: i}}
			}
			materializer := &captureResultMaterializer{remoteResult: taskmedia.MaterializedResults{Items: items}}
			executor.resultMaterializer = materializer
			task := generationTaskForTest(t, "task-partial")
			task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: workflow.FeatureSlugGeneration, Controls: &tasks.GenerationControls{OutputCount: 4}})
			output, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{Task: task, Model: "openai/gpt-image-1"})
			if successful == 0 {
				if err == nil {
					t.Fatal("first output failure was swallowed")
				}
			} else {
				if err != nil {
					t.Fatalf("successful outputs lost: %v", err)
				}
				if len(output.Outputs) != successful || len(materializer.remoteSourceURLs) != successful {
					t.Fatalf("outputs=%d ingested=%d, want %d", len(output.Outputs), len(materializer.remoteSourceURLs), successful)
				}
			}
			if len(fw.capturedBodies) != successful+1 {
				t.Fatalf("calls=%d, want %d without retry", len(fw.capturedBodies), successful+1)
			}
		})
	}
}

func TestCallWorkerAPIPreservesCancellationAfterOneOutput(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fw.respond = func(taskID, uploadURL string) (int, string) {
		if len(fw.capturedBodies) == 2 {
			cancel()
			return 503, `{}`
		}
		return 0, ""
	}
	task := generationTaskForTest(t, "task-cancelled-batch")
	task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{SchemaVersion: tasks.CurrentParamsSchemaVersion, Service: workflow.FeatureSlugGeneration, Controls: &tasks.GenerationControls{OutputCount: 4}})
	output, err := workerTestExecutor(fw, nil).CallWorkerAPI(ctx, runtimeports.WorkerCallInput{Task: task, Model: "openai/gpt-image-1"})
	if !errors.Is(err, context.Canceled) || len(output.Outputs) != 0 {
		t.Fatalf("outputs=%v err=%v, want cancellation", output.Outputs, err)
	}
	if len(fw.capturedBodies) != 2 {
		t.Fatalf("calls=%d after cancellation", len(fw.capturedBodies))
	}
}

func TestCallWorkerAPIKeepsOutputWhenNextInputSigningFails(t *testing.T) {
	fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
	defer fw.server.Close()
	fw.respond = func(taskID, uploadURL string) (int, string) {
		// The first send already resolved its input. A disappearing source
		// makes the next presign fail through the real transfer adapter.
		delete(fw.transfer.assets, "asset_input1")
		return 0, ""
	}
	task := generationTaskForTest(t, "task-later-input-unavailable")
	task.Params = marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Service:       workflow.FeatureSlugRender,
		Controls:      &tasks.GenerationControls{OutputCount: 4},
		Media:         &tasks.MediaInputs{Inputs: []tasks.MediaRef{{PublicAssetID: "asset_input1"}}},
	})
	executor := workerTestExecutor(fw, nil)
	output, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{Task: task, Model: "fal/flux"})
	if err != nil || len(output.Outputs) != 1 {
		t.Fatalf("outputs=%d err=%v; want one completed output", len(output.Outputs), err)
	}
	if len(fw.capturedBodies) != 1 || fw.fetchedInputs != 1 {
		t.Fatalf("calls=%d fetched inputs=%d; want one without batch retry", len(fw.capturedBodies), fw.fetchedInputs)
	}
}

func TestCallWorkerAPISendsCanonicalOutputResolution(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		width  int
		height int
	}{
		{"default 1k", nil, 1024, 1024},
		{"1k", map[string]interface{}{"output_resolution": "1k"}, 1024, 1024},
		{"2k", map[string]interface{}{"output_resolution": "2k"}, 2048, 2048},
		{"4k", map[string]interface{}{"output_resolution": "4k"}, 4096, 4096},
		{"normalized resolution", map[string]interface{}{"output_resolution": " 2K "}, 2048, 2048},
		{"landscape 1k", map[string]interface{}{"output_resolution": "1k", "ratio": "16:9"}, 1024, 576},
		{"landscape 2k", map[string]interface{}{"output_resolution": "2k", "ratio": "16:9"}, 2048, 1152},
		{"landscape 4k", map[string]interface{}{"output_resolution": "4k", "ratio": "16:9"}, 4096, 2304},
		{"portrait snapped to 64px", map[string]interface{}{"output_resolution": "2k", "ratio": "2:3"}, 1344, 2048},
		{"custom 1k", map[string]interface{}{"output_resolution": "1k", "custom_width": 512, "custom_height": 256}, 512, 256},
		{"custom 2k", map[string]interface{}{"output_resolution": "2k", "custom_width": 512, "custom_height": 256}, 1024, 512},
		{"custom 4k overrides ratio", map[string]interface{}{"output_resolution": "4k", "custom_width": 512, "custom_height": 256, "ratio": "1:1"}, 2048, 1024},
		{"quality cannot set resolution", map[string]interface{}{"quality": "4k"}, 1024, 1024},
		{"custom quality cannot set resolution", map[string]interface{}{"output_resolution": "1k", "custom_width": 512, "custom_height": 256, "quality": "4k"}, 512, 256},
		{"quality independent of resolution", map[string]interface{}{"output_resolution": "2k", "quality": "high"}, 2048, 2048},
		{"original aspect ratio", map[string]interface{}{"output_resolution": "2k", "ratio": "3:2", "original_image_width": 900, "original_image_height": 600}, 2048, 1344},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fw := newFakeWorkerServer(t, buildTinyPNGBytes(t))
			defer fw.server.Close()
			executor := workerTestExecutor(fw, nil)
			params, err := taskapp.CompileWorkspaceParams(workflow.FeatureSlugGeneration, "resolution probe", tc.params, prompts.FeaturePromptRuntimeConfig{})
			if err != nil {
				t.Fatalf("compile workspace params: %v", err)
			}
			task := &tasks.TaskPayload{TaskID: "task-resolution", UserID: 42, Params: params}
			if _, err := executor.CallWorkerAPI(context.Background(), runtimeports.WorkerCallInput{
				Task: task, Model: "openai/dall-e-3", AttemptNo: 1,
			}); err != nil {
				t.Fatalf("CallWorkerAPI returned error: %v", err)
			}
			if fw.capturedBody["width"] != float64(tc.width) || fw.capturedBody["height"] != float64(tc.height) {
				t.Fatalf("size = %v/%v, want %d/%d", fw.capturedBody["width"], fw.capturedBody["height"], tc.width, tc.height)
			}
		})
	}
}
