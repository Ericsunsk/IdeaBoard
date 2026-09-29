package worker

import (
	"context"
	"strings"
	"testing"

	taskservice "ideaboard/api/internal/tasking/application/task"
	"ideaboard/api/internal/tasking/workspacetasks"
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
	legacy, err := workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{
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
	legacy, err := workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{
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
	legacy, err = workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{
		ID: "task_prec2", Service: "generation", CanonicalParams: userOnly,
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	if legacy.Prompt != "user text" {
		t.Fatalf("prompt = %q, want user snapshot text", legacy.Prompt)
	}

	if _, err := workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{
		ID: "task_prec3", Service: "render", Params: map[string]any{"prompt": "legacy"},
	}); err == nil {
		t.Fatal("missing canonical params = nil, want fail-closed error")
	}
}

// Full mask loop at worker level: a task carrying params.mask_asset_id
// resolves its mask through the asset gate and reaches the provider;
// a vanished mask fails deterministically before any provider call.
func TestWorkspaceTaskWorkerMaskAssetLoop(t *testing.T) {
	ctx := context.Background()
	newMaskEnv := func() (*workerTestEnv, *workerTestAssets) {
		env := newWorkerTestEnv()
		assets := &workerTestAssets{ready: map[string]bool{"asset_mask1": true}, archived: map[string]bool{}}
		env.service = workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
			TaskRead: env.store, TaskLifecycle: env.store, Quoter: workerTestQuoter{}, Holder: env.holder, Queue: env.queue,
			Assets: assets, Resolver: allowTestPoolResolver(),
		})
		return env, assets
	}
	env, assets := newMaskEnv()
	created, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7, Service: "inpaint",
		Params: map[string]any{"mask_asset_id": "asset_mask1"},
	}, "mask-loop")
	if err != nil {
		t.Fatalf("submit with mask: %v", err)
	}
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "inpaint", []string{"openrouter/only-model"}))
	seenMask := ""
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		id, _ := workspacetasks.MaskAssetIDFromParams(task.Params)
		seenMask = id
		return workerTestRemoteOutput(), nil
	}), assets)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if seenMask != "asset_mask1" {
		t.Fatalf("provider saw mask %q, want asset_mask1", seenMask)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}

	// A mask that vanished after submit fails the task without claiming
	// (token must not rotate) and without calling the provider.
	token, err := workspacetasks.NewExecutionToken()
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := env.store.CreateTask(ctx, &workspacetasks.WorkspaceTask{
		ID: "task_gone_mask", WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "inpaint", Status: workspacetasks.StatusPending, ExecutionToken: token,
		Params: map[string]any{"mask_asset_id": "asset_vanished"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	called := false
	worker = env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), assets)
	if err := worker.Work(ctx, workerJobFor(seeded)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called with a vanished mask, want fail before generate")
	}
	got, err = env.store.GetTask(ctx, "ws_test", seeded.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ExecutionToken != token {
		t.Fatal("vanished mask must fail without claiming (token must not rotate)")
	}
	if !strings.Contains(got.ErrorMsg, "遮罩") {
		t.Fatalf("error_msg = %q, want the mask failure reason", got.ErrorMsg)
	}
}
