package worker

// Execution bridge tests: the gateway payload is synthesized from the
// durable canonical plan (execution.go). The worker-level mask loop lives
// in claim_test.go (mask resolution gates the claim phase).

import (
	"testing"

	taskservice "ideaboard/api/internal/tasking/application/task"
	tasks "ideaboard/api/internal/tasking/domain"
)

// The worker must actually USE the v1 prompt: the bridge reads the
// canonical Generation snapshot (CompiledPrompt, fallback UserPrompt)
// stamped by CompileWorkspaceParams from data.prompt.
func TestWorkspaceBridgeTaskPayloadUsesGenerationPrompt(t *testing.T) {
	raw, err := taskservice.CompileWorkspaceParams("generation", "a red bicycle", map[string]interface{}{
		"output_count": 1,
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	legacy, err := workspaceBridgeTaskPayload(&tasks.WorkspaceTask{
		ID: "task_prompt1", WorkspaceID: "ws_test", OwnerUserID: 7,
		Service:         "generation",
		CanonicalParams: raw,
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	if legacy.Prompt != "a red bicycle" {
		t.Fatalf("bridged prompt = %q, want the v1 prompt", legacy.Prompt)
	}
}

// Snapshot precedence: CompiledPrompt wins; UserPrompt is the fallback.
func TestWorkspaceBridgeTaskPayloadPromptPrecedence(t *testing.T) {
	compiled := `{"schema_version":6,"service":"generation","generation":{"user_prompt":"user text","compiled_prompt":"compiled text"}}`
	legacy, err := workspaceBridgeTaskPayload(&tasks.WorkspaceTask{
		ID: "task_prec1", Service: "generation",
		CanonicalParams: compiled, Params: map[string]any{"prompt": "stale"},
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	if legacy.Prompt != "compiled text" {
		t.Fatalf("prompt = %q, want compiled snapshot text", legacy.Prompt)
	}

	userOnly := `{"schema_version":6,"service":"generation","generation":{"user_prompt":"user text"}}`
	legacy, err = workspaceBridgeTaskPayload(&tasks.WorkspaceTask{
		ID: "task_prec2", Service: "generation", CanonicalParams: userOnly,
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	if legacy.Prompt != "user text" {
		t.Fatalf("prompt = %q, want user snapshot text", legacy.Prompt)
	}

	if _, err := workspaceBridgeTaskPayload(&tasks.WorkspaceTask{
		ID: "task_prec3", Service: "render", Params: map[string]any{"prompt": "legacy"},
	}); err == nil {
		t.Fatal("missing canonical params = nil, want fail-closed error")
	}
}
