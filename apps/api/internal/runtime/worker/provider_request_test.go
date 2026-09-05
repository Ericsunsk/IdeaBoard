package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/providerendpoints"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

func TestNewProviderRequestCollectsProviderMediaRefs(t *testing.T) {
	params := map[string]interface{}{
		"image_url":            "https://img.example/single.png",
		"image_urls":           []interface{}{"https://img.example/source.png", "https://img.example/shared.png"},
		"reference_image_urls": []interface{}{"https://img.example/ref.png", "https://img.example/source.png"},
		"mask_image_urls":      []interface{}{"https://img.example/mask-list.png", "https://img.example/shared.png"},
		"original_image_url":   "https://img.example/source.png",
		"mask_image_url":       "https://img.example/mask.png",
	}

	req := newProviderRequest(
		"prompt", 640, 480, params, "gemini-image",
		resolvedProviderRequestFacts{primitive: workflow.GenerationPrimitiveTextToImage, outputCount: 1, outputResolution: workflow.OutputResolution1K},
		workflow.RuntimeOperationGeneration, workflow.RuntimeInvocationGenerate, workflow.RuntimeControlsNone,
	)

	wantOpenAI := []string{
		"https://img.example/source.png",
		"https://img.example/shared.png",
		"https://img.example/ref.png",
	}
	if !reflect.DeepEqual(req.ImageRefs, wantOpenAI) {
		t.Fatalf("unexpected OpenAI refs: got=%#v want=%#v", req.ImageRefs, wantOpenAI)
	}

	wantGoogle := []string{
		"https://img.example/source.png",
		"https://img.example/shared.png",
		"https://img.example/ref.png",
		"https://img.example/mask-list.png",
		"https://img.example/mask.png",
	}
	if !reflect.DeepEqual(providerGoogleImageRefs(req), wantGoogle) {
		t.Fatalf("unexpected Google refs: got=%#v want=%#v", providerGoogleImageRefs(req), wantGoogle)
	}
	if req.Size != "640x480" {
		t.Fatalf("unexpected size: %s", req.Size)
	}
}

func TestNewProviderRequestConsumesResolvedOutputFacts(t *testing.T) {
	req := newProviderRequest("prompt", 640, 480, map[string]interface{}{
		"output_count":      5,
		"output_resolution": "4k",
	}, "gpt-image-1", resolvedProviderRequestFacts{
		primitive: workflow.GenerationPrimitiveTextToImage, outputCount: 2, outputResolution: workflow.OutputResolution2K,
	}, workflow.RuntimeOperationGeneration, workflow.RuntimeInvocationGenerate, workflow.RuntimeControlsNone)
	if req.OutputCount != 2 || req.OutputResolution != workflow.OutputResolution2K {
		t.Fatalf("resolved output facts = count:%d resolution:%q", req.OutputCount, req.OutputResolution)
	}
}

func TestProviderAdapterBoundaryContainsNoWorkflowMapOrFeature(t *testing.T) {
	for _, contract := range []reflect.Type{
		reflect.TypeOf(providerRequest{}),
		reflect.TypeOf(openAIProviderControls{}),
		reflect.TypeOf(googleProviderControls{}),
	} {
		for index := 0; index < contract.NumField(); index++ {
			field := contract.Field(index)
			if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Interface {
				t.Fatalf("%s.%s exposes dynamic workflow data", contract.Name(), field.Name)
			}
			if strings.Contains(field.Name, "Feature") || strings.Contains(field.Name, "Billing") || strings.Contains(field.Name, "Node") {
				t.Fatalf("%s.%s exposes orchestration concept to provider adapter", contract.Name(), field.Name)
			}
		}
	}
}

func TestProviderRequestShapesOnlyDeclaredRuntimeControlContract(t *testing.T) {
	params := map[string]interface{}{
		"variation":          map[string]interface{}{"strength": 40},
		"variation_strength": 40,
		"upscale":            map[string]interface{}{"scale": 2.0},
	}

	none := newProviderRequest(
		"", 0, 0, params, "", resolvedProviderRequestFacts{}, workflow.RuntimeOperationGeneration,
		workflow.RuntimeInvocationGenerate, workflow.RuntimeControlsNone,
	)
	if none.OpenAI.VariationStrength != nil || none.Scale != 0 {
		t.Fatalf("none contract leaked controls: %#v", none)
	}

	variation := newProviderRequest(
		"", 0, 0, params, "", resolvedProviderRequestFacts{}, workflow.RuntimeOperationVariations,
		workflow.RuntimeInvocationEdit, workflow.RuntimeControlsVariation,
	)
	if variation.OpenAI.VariationStrength == nil || *variation.OpenAI.VariationStrength != 40 || variation.Scale != 0 {
		t.Fatalf("variation controls = %#v", variation)
	}

	outpaint := newProviderRequest(
		"", 0, 0, params, "", resolvedProviderRequestFacts{}, workflow.RuntimeOperationOutpaint,
		workflow.RuntimeInvocationEdit, workflow.RuntimeControlsOutpaint,
	)
	if outpaint.OpenAI.VariationStrength != nil || outpaint.Scale != 0 {
		t.Fatalf("outpaint controls = %#v", outpaint)
	}

	upscale := newProviderRequest(
		"", 0, 0, params, "", resolvedProviderRequestFacts{}, workflow.RuntimeOperationUpscale,
		workflow.RuntimeInvocationTransform, workflow.RuntimeControlsUpscale,
	)
	if upscale.Scale != 2 || upscale.OpenAI.VariationStrength != nil {
		t.Fatalf("upscale controls = %#v", upscale)
	}
}

func TestParseAndNormalizeTaskParamsFlattensCanonicalWorkflowParams(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "outpaint",
		Media: &tasks.MediaInputs{
			Original: &tasks.MediaRef{AssetID: 2, Role: "original"},
		},
		Controls: &tasks.GenerationControls{
			OriginalWidth:  640,
			OriginalHeight: 480,
		},
		Outpaint: &tasks.OutpaintParams{
			TargetWidth:  768,
			TargetHeight: 480,
			ImageOffsetX: 0,
			ImageOffsetY: 0,
		},
	})
	executor := &ProviderExecutor{mediaResolver: testRuntimeMediaResolver{urls: map[int64]string{
		2: "https://img.example/source.png",
	}}}
	got, err := executor.parseAndNormalizeTaskParams(context.Background(), &tasks.TaskPayload{
		TaskID: "task-canonical-params",
		Params: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = executor.resolveRuntimeProviderTaskParams(context.Background(), got)
	if err != nil {
		t.Fatal(err)
	}

	if got.ExecutionPlan == nil || got.ExecutionPlan.RuntimeOperation() != workflow.RuntimeOperationOutpaint {
		t.Fatalf("runtime operation = %#v, want outpaint", got.ExecutionPlan)
	}
	// Issue #664: the durable boundary no longer carries
	// `original_image_url`. The original is always resolved
	// through the processing-input derivative. The runtime
	// provider call receives the masked input only.
	if got.Provider["original_image_url"] != nil {
		t.Fatalf("original_image_url must be dropped from the durable boundary: %#v", got.Provider["original_image_url"])
	}
	if got.Provider["original_image_width"] != float64(640) || got.Provider["original_image_height"] != float64(480) {
		t.Fatalf("original size = %#v x %#v", got.Provider["original_image_width"], got.Provider["original_image_height"])
	}
	if got.Provider["outpaint"] == nil {
		t.Fatalf("outpaint = %#v, want canonical geometry", got.Provider["outpaint"])
	}
	if _, exists := got.Provider["media"]; exists {
		t.Fatalf("provider map should not expose canonical media object: %#v", got.Provider)
	}
}

func TestParseAndNormalizeTaskParamsRejectsInvalidDurableContracts(t *testing.T) {
	tests := map[string]string{
		"missing schema": `{"feature":"generation"}`,
		"legacy schema":  `{"schema_version":1,"feature":"generation"}`,
		"future schema":  `{"schema_version":99,"feature":"generation"}`,
		"missing plan":   `{"schema_version":5,"feature":"generation"}`,
		"malformed plan": `{"schema_version":5,"feature":"generation","execution_plan":{"schema_version":1}}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := (&ProviderExecutor{}).parseAndNormalizeTaskParams(context.Background(), &tasks.TaskPayload{TaskID: "invalid-contract", Params: raw}); err == nil {
				t.Fatal("expected invalid task contract to be rejected")
			}
		})
	}
}

func TestProviderRuntimeAdapterForUsesProtocolAndRuntimeOperation(t *testing.T) {
	tests := []struct {
		name      string
		apiType   string
		protocol  string
		operation workflow.RuntimeOperation
		want      providerRuntimeAdapter
		wantErr   bool
	}{
		{name: "model prompt", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolModelAPI, operation: workflow.RuntimeOperationGeneration, want: providerRuntimeAdapterModelImage},
		{name: "image fill on model protocol", apiType: workflow.ProviderGoogleAI, protocol: providerendpoints.ProtocolModelAPI, operation: workflow.RuntimeOperationInpaint, wantErr: true},
		{name: "cutout", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolImagePipeline, operation: workflow.RuntimeOperationCutout, want: providerRuntimeAdapterCutout},
		{name: "upscale", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolImagePipeline, operation: workflow.RuntimeOperationUpscale, want: providerRuntimeAdapterUpscale},
		{name: "self-hosted image fill", apiType: workflow.ProviderSelfHosted, protocol: providerendpoints.ProtocolImageFill, operation: workflow.RuntimeOperationOutpaint, want: providerRuntimeAdapterFluxFill},
		{name: "transform API type mismatch", apiType: workflow.ProviderGoogleAI, protocol: providerendpoints.ProtocolImagePipeline, operation: workflow.RuntimeOperationUpscale, wantErr: true},
		{name: "transform on model protocol", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolModelAPI, operation: workflow.RuntimeOperationCutout, wantErr: true},
		{name: "generation on transform protocol", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolImagePipeline, operation: workflow.RuntimeOperationGeneration, wantErr: true},
		{name: "unknown operation", apiType: workflow.ProviderOpenAICompatible, protocol: providerendpoints.ProtocolModelAPI, operation: workflow.RuntimeOperation("unknown"), wantErr: true},
		{name: "missing protocol", apiType: workflow.ProviderOpenAICompatible, operation: workflow.RuntimeOperationGeneration, wantErr: true},
		{name: "missing API type", protocol: providerendpoints.ProtocolModelAPI, operation: workflow.RuntimeOperationGeneration, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := providerRuntimeAdapterFor(tt.apiType, tt.protocol, tt.operation)
			if (err != nil) != tt.wantErr {
				t.Fatalf("providerRuntimeAdapterFor error = %v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("adapter = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCallDrawingAPIRejectsUnknownDurableOperationBeforeProviderIO(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugGeneration,
	})
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatal(err)
	}
	plan, ok := params["execution_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution_plan = %#v", params["execution_plan"])
	}
	runtime, ok := plan["runtime"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution_plan.runtime = %#v", plan["runtime"])
	}
	runtime["operation"] = "unknown-operation"
	corruptRaw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	providerCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		providerCalls++
	}))
	defer server.Close()

	account := &accounts.ApiAccount{
		ID: 1, Name: "strict-plan", ApiKey: "key", ApiType: workflow.ProviderOpenAICompatible,
		Protocol: providerendpoints.ProtocolModelAPI, ProxyURL: server.URL,
	}
	_, err = (&ProviderExecutor{}).callDrawingAPIWithAssignment(
		context.Background(),
		runtimeAccountAccessForTest(account),
		&tasks.TaskPayload{TaskID: "task-corrupt-operation", Prompt: "draw", Params: string(corruptRaw)},
		assignmentSnapshotForTest(account, "gpt-image-1"),
	)
	if err == nil {
		t.Fatal("expected corrupt execution plan to fail")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok || stage != tasks.ExecutionStageAssigned || category != tasks.FailureCategoryInputInvalid {
		t.Fatalf("failure = stage:%q category:%q ok:%v err:%v", stage, category, ok, err)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls)
	}
}

func TestCallDrawingAPIRejectsProtocolOperationMismatchBeforeProviderIO(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugGeneration,
	})
	providerCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		providerCalls++
	}))
	defer server.Close()

	account := &accounts.ApiAccount{
		ID: 1, Name: "wrong-protocol", ApiKey: "key", ApiType: workflow.ProviderOpenAICompatible,
		Protocol: providerendpoints.ProtocolImagePipeline, ProxyURL: server.URL,
	}
	_, err := (&ProviderExecutor{}).callDrawingAPIWithAssignment(
		context.Background(),
		runtimeAccountAccessForTest(account),
		&tasks.TaskPayload{TaskID: "task-protocol-mismatch", Prompt: "draw", Params: raw},
		assignmentSnapshotForTest(account, "gpt-image-1"),
	)
	if err == nil {
		t.Fatal("expected protocol/operation mismatch to fail")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok || stage != tasks.ExecutionStageAssigned || category != tasks.FailureCategoryInputInvalid {
		t.Fatalf("failure = stage:%q category:%q ok:%v err:%v", stage, category, ok, err)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls)
	}
}

func TestParseAndNormalizeTaskParamsRejectsFactsForgedAroundValidPlan(t *testing.T) {
	base := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugVariations,
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{{AssetID: 1, Role: "input"}},
		},
	})
	tests := map[string]func(*tasks.WorkflowTaskParams){
		"generation schema": func(params *tasks.WorkflowTaskParams) {
			params.Generation.SchemaVersion = 1
		},
		"generation primitive": func(params *tasks.WorkflowTaskParams) {
			params.Generation.Primitive = string(workflow.GenerationPrimitiveTextToImage)
		},
		"generation output count": func(params *tasks.WorkflowTaskParams) {
			params.Generation.RequestedOutputCount = 2
		},
		"generation media": func(params *tasks.WorkflowTaskParams) {
			params.Generation.Media = []tasks.GenerationMediaIntent{{InputIndex: 0, Role: "mask", AssetID: 1}}
		},
		"provider output controls": func(params *tasks.WorkflowTaskParams) {
			params.Controls.OutputCount = 2
		},
		"provider media": func(params *tasks.WorkflowTaskParams) {
			params.Media = nil
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params, err := tasks.UnmarshalParams(base)
			if err != nil {
				t.Fatal(err)
			}
			mutate(params)
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (&ProviderExecutor{}).parseAndNormalizeTaskParams(context.Background(), &tasks.TaskPayload{TaskID: "forged-contract", Params: string(raw)}); err == nil {
				t.Fatal("expected forged task facts to be rejected")
			}
		})
	}
}

func TestParseAndNormalizeTaskParamsRejectsCanonicalControlsOutsidePlanContract(t *testing.T) {
	base := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugGeneration,
	})
	tests := map[string]func(*tasks.WorkflowTaskParams){
		"variation": func(params *tasks.WorkflowTaskParams) {
			params.Variation = &tasks.VariationsParams{Strength: 40}
		},
		"outpaint": func(params *tasks.WorkflowTaskParams) {
			params.Outpaint = &tasks.OutpaintParams{TargetWidth: 768, TargetHeight: 480}
		},
		"upscale": func(params *tasks.WorkflowTaskParams) {
			params.Upscale = &tasks.UpscaleParams{Scale: 2}
		},
		"fusion": func(params *tasks.WorkflowTaskParams) {
			params.Fusion = &tasks.FusionParams{Mode: "weighted_reference", InputWeights: []float64{1}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params, err := tasks.UnmarshalParams(base)
			if err != nil {
				t.Fatal(err)
			}
			mutate(params)
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (&ProviderExecutor{}).parseAndNormalizeTaskParams(
				context.Background(),
				&tasks.TaskPayload{TaskID: "forged-controls", Params: string(raw)},
			); err == nil || !strings.Contains(err.Error(), "canonical controls") {
				t.Fatalf("error = %v, want canonical control mismatch", err)
			}
		})
	}
}

func TestPrepareProviderExecutionCompletesPreflightBeforeLocalMediaIO(t *testing.T) {
	useMask := true
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugInpaint,
		Media: &tasks.MediaInputs{
			Original: &tasks.MediaRef{AssetID: 1, Role: "original"},
			Mask:     &tasks.MediaRef{AssetID: 2, Role: "mask"},
			UseMask:  &useMask,
		},
	})
	mediaCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mediaCalls++
	}))
	defer server.Close()
	raw = strings.ReplaceAll(raw, "https://media.example", server.URL)

	unexpectedModelAccount := &accounts.ApiAccount{
		ID: 1, Name: "unexpected-model", ApiKey: "key", ApiType: workflow.ProviderOpenAICompatible,
		Protocol: providerendpoints.ProtocolImageFill, ProxyURL: "http://127.0.0.1:18080",
	}
	_, err := (&ProviderExecutor{}).resolveProviderExecution(
		context.Background(),
		runtimeAccountAccessForTest(unexpectedModelAccount),
		&tasks.TaskPayload{TaskID: "task-unexpected-model", Params: raw},
		assignmentSnapshotForTest(unexpectedModelAccount, "gpt-image-1"),
	)
	if err == nil || !strings.Contains(err.Error(), "modelless operation") {
		t.Fatalf("error = %v, want unexpected model", err)
	}
	if mediaCalls != 0 {
		t.Fatalf("media HTTP calls = %d, want 0", mediaCalls)
	}

	forged, unmarshalErr := tasks.UnmarshalParams(raw)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	forged.Variation = &tasks.VariationsParams{Strength: 40}
	forgedRaw, marshalErr := json.Marshal(forged)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	incompatibleAccount := &accounts.ApiAccount{
		ID: 1, Name: "incompatible-controls", ApiKey: "key", ApiType: workflow.ProviderOpenAICompatible,
		Protocol: providerendpoints.ProtocolImageFill, ProxyURL: "http://127.0.0.1:18080",
	}
	_, err = (&ProviderExecutor{}).resolveProviderExecution(
		context.Background(),
		runtimeAccountAccessForTest(incompatibleAccount),
		&tasks.TaskPayload{TaskID: "task-incompatible-controls", Params: string(forgedRaw)},
		assignmentSnapshotForTest(incompatibleAccount, ""),
	)
	if err == nil || !strings.Contains(err.Error(), "canonical controls") {
		t.Fatalf("error = %v, want incompatible controls", err)
	}
	if mediaCalls != 0 {
		t.Fatalf("media HTTP calls after incompatible task = %d, want 0", mediaCalls)
	}
}

func TestRiverPayloadCanonicalParamsBecomeProviderRequest(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "fusion",
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{
				{AssetID: 1, Role: "input", Weight: 0.75},
				{AssetID: 2, Role: "input", Weight: 0.25},
			},
			References: []tasks.MediaRef{
				{AssetID: 3, Role: "reference"},
			},
		},
		Controls: &tasks.GenerationControls{
			CustomWidth:  512,
			CustomHeight: 768,
		},
		Generation: &tasks.GenerationRequestSnapshot{
			SchemaVersion:        tasks.CurrentParamsSchemaVersion,
			Feature:              "fusion",
			Primitive:            string(workflow.GenerationPrimitiveImageToImage),
			UserPrompt:           "融合两张图",
			CompiledPrompt:       "融合两张图",
			TemplateFingerprint:  "test-fusion",
			RequestedOutputCount: 1,
		},
		Fusion: &tasks.FusionParams{
			Mode:         "weighted_reference",
			InputWeights: []float64{0.75, 0.25},
		},
	})
	submitted := &tasks.TaskPayload{
		TaskID: "task-river-contract",
		UserID: 42,
		Prompt: "融合两张图",
		Params: raw,
	}
	decoded := roundTripDrawingTaskForTest(t, submitted)

	executor := &ProviderExecutor{mediaResolver: testRuntimeMediaResolver{urls: map[int64]string{
		1: "https://img.example/a.png", 2: "https://img.example/b.png", 3: "https://img.example/ref.png",
	}}}
	params, err := executor.parseAndNormalizeTaskParams(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	params, err = executor.resolveRuntimeProviderTaskParams(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	req, err := params.providerRequest(decoded.Prompt, "gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}

	if req.Prompt != submitted.Prompt || req.Model != "gpt-image-1" || req.Size != "512x768" {
		t.Fatalf("provider request = prompt:%q model:%q size:%q", req.Prompt, req.Model, req.Size)
	}
	if req.Primitive != workflow.GenerationPrimitiveImageToImage {
		t.Fatalf("provider primitive=%q, want image-to-image", req.Primitive)
	}
	if !reflect.DeepEqual(req.ImageRefs, []string{
		"https://img.example/a.png",
		"https://img.example/b.png",
		"https://img.example/ref.png",
	}) {
		t.Fatalf("OpenAI refs=%#v", req.ImageRefs)
	}
	if req.OpenAI.VariationStrength != nil {
		t.Fatalf("unexpected OpenAI controls: %#v", req.OpenAI)
	}
	if !req.OpenAI.UseEditEndpoint {
		t.Fatalf("fusion should compile to OpenAI edit controls: %#v", req.OpenAI)
	}
	if req.Google.AspectRatio != "" {
		t.Fatalf("unexpected Google controls: %#v", req.Google)
	}
}

func TestPrepareProviderExecutionAllowsGenerationControls(t *testing.T) {
	raw := marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       workflow.FeatureSlugGeneration,
		Controls: &tasks.GenerationControls{
			Ratio:       "4:3",
			OutputCount: 2,
		},
	})
	account := &accounts.ApiAccount{
		ID:       1,
		Name:     "generation-provider",
		ApiKey:   "key",
		ApiType:  workflow.ProviderOpenAICompatible,
		Protocol: providerendpoints.ProtocolModelAPI,
		ProxyURL: "http://127.0.0.1:18080/v1",
	}
	exec, err := (&ProviderExecutor{}).resolveProviderExecution(
		context.Background(),
		runtimeAccountAccessForTest(account),
		&tasks.TaskPayload{
			TaskID: "task-generation-provider-hints",
			UserID: 42,
			Prompt: "repeatable textile pattern",
			Params: raw,
		},
		assignmentSnapshotForTest(account, "gpt-image-1"),
	)
	if err != nil {
		t.Fatalf("resolveProviderExecution returned error: %v", err)
	}
	if exec.request.OutputCount != 2 {
		t.Fatalf("output_count = %d, want 2", exec.request.OutputCount)
	}
	if exec.runtimeOperation != workflow.RuntimeOperationGeneration {
		t.Fatalf("runtime operation = %q, want generation", exec.runtimeOperation)
	}
}

func TestProviderTaskParamsPreferCanonicalFeature(t *testing.T) {
	params := newProviderTaskParams(&tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       "fusion",
		Extra: map[string]interface{}{
			"feature": "inpaint",
		},
	}, nil)

	if params.Provider["feature"] != "fusion" {
		t.Fatalf("provider params should keep canonical feature, got %#v", params.Provider)
	}
}
