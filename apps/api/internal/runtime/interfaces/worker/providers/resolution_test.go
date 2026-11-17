package providers

import (
	"context"
	"testing"

	"ideaboard/api/internal/prompts"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

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
