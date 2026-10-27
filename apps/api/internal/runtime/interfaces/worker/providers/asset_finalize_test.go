package providers

// Asset-kind finalize test: the workspace provider-result materializer
// ingests bytes before finalize, so CallGatewayAPI must accept outputs
// that carry a workspace asset id with no URL round-trip. URL outputs keep
// the existing validation untouched (see gateway_execute_test.go).

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"
)

// assetKindResultMaterializer returns workspace asset-kind outputs for any
// provider image URL (mirrors the production workspace materializer shape
// without any media store).
type assetKindResultMaterializer struct {
	prefix string
}

func (m *assetKindResultMaterializer) MaterializeRemoteResults(_ context.Context, _ int64, taskID string, sourceURLs []string, _ taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	items := make([]taskmedia.MaterializedResult, 0, len(sourceURLs))
	for index := range sourceURLs {
		assetID := m.prefix + string(rune('a'+index))
		items = append(items, taskmedia.MaterializedResult{
			ImageURL: assetID,
			Output: tasks.TaskOutput{
				TaskID: taskID, AssetID: assetID,
				Role: "result", ResultIndex: index,
			},
		})
	}
	return taskmedia.MaterializedResults{Items: items}, nil
}

func TestCallGatewayAPIAcceptsAssetKindOutputs(t *testing.T) {
	rawImagePayload := base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + rawImagePayload + `"}}]}}]}`))
	}))
	defer server.Close()

	executor := &ProviderExecutor{
		resultMaterializer: &assetKindResultMaterializer{prefix: "asset_faketest000"},
		gatewayFactory:     gatewayTestFactory(server.URL),
	}
	task := &tasks.TaskPayload{
		TaskID: "task-asset-finalize",
		UserID: 42,
		Prompt: "fake asset finalize",
		Params: marshalWorkflowTaskParamsForTest(t, &tasks.WorkflowTaskParams{
			SchemaVersion: tasks.CurrentParamsSchemaVersion,
			Service:       workflow.FeatureSlugGeneration,
		}),
	}

	output, err := executor.CallGatewayAPI(context.Background(), runtimeports.GatewayCallInput{
		Task: task, Model: "openrouter/fake-test-image", ExecutionToken: "tok-asset", AttemptNo: 1,
	})
	if err != nil {
		t.Fatalf("CallGatewayAPI with asset-kind outputs: %v", err)
	}
	if len(output.Outputs) != 1 {
		t.Fatalf("outputs len = %d, want 1", len(output.Outputs))
	}
	got := output.Outputs[0]
	if got.AssetID == "" || got.URL != "" || got.ChecksumSHA256 != "" {
		t.Fatalf("output = %+v, want asset-kind (id only, no URL/checksum)", got)
	}
	if got.ResultIndex != 0 {
		t.Fatalf("result_index = %d, want 0", got.ResultIndex)
	}
}
