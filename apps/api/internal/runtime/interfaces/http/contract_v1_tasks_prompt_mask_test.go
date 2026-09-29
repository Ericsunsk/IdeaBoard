package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	taskservice "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

// Prompt/mask submit plane (v1 task creation gap batch). Unlike
// newV1TaskFixture, this fixture wires the REAL canonical compiler
// (TaskCanonicalCompiler: taskservice.CompileWorkspaceParams, production
// parity) so the tests prove submit -> canonical params -> worker input
// end to end at the handler level.
type v1PromptMaskFixture struct {
	*v1TaskFixture
}

func newV1PromptMaskFixture(t *testing.T) *v1PromptMaskFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := workspacetasks.NewInMemoryStoreForTests()
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	assets := &fakeTaskAssets{assets: map[string]*workspacetasks.AssetInfo{}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), "user")
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir,
		Tasks: tasks, TaskQuoter: quoter, TaskHolder: holder, TaskQueue: queue, TaskAssets: assets,
		AllowNonTransactionalTaskSettlement: true,
		TaskCanonicalCompiler:               taskservice.CompileWorkspaceParams,
		TaskRouter:                          allowTestPoolResolver(),
	})
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "prompt-mask", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	for _, id := range []int64{1, 3} {
		dir.ensure(id)
	}
	fx := &v1TaskFixture{engine: router, store: store, dir: dir, ws: ws, tasks: tasks, quoter: quoter, holder: holder, queue: queue, assets: assets}
	fx.assets.assets[ws.PublicID+"\x00asset_input1"] = &workspacetasks.AssetInfo{ID: "asset_input1", WorkspaceID: ws.PublicID, Status: "ready"}
	fx.assets.assets[ws.PublicID+"\x00asset_mask1"] = &workspacetasks.AssetInfo{ID: "asset_mask1", WorkspaceID: ws.PublicID, Status: "ready"}
	return &v1PromptMaskFixture{fx}
}

func (fx *v1PromptMaskFixture) postTask(t *testing.T, key, body string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: 3,
		body: body, headers: map[string]string{contract.IdempotencyKeyHeader: key},
	})
	return status, payload
}

func (fx *v1PromptMaskFixture) canonicalParams(t *testing.T, taskID string) *tasksdomain.WorkflowTaskParams {
	t.Helper()
	stored, err := fx.tasks.GetTask(context.Background(), fx.ws.PublicID, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if strings.TrimSpace(stored.CanonicalParams) == "" {
		t.Fatal("canonical params empty, want compiled params")
	}
	typed, err := tasksdomain.UnmarshalParams(stored.CanonicalParams)
	if err != nil {
		t.Fatalf("decode canonical params: %v", err)
	}
	return typed
}

// Submit-with-prompt: data.prompt threads into the canonical Generation
// snapshot the worker bridges to the provider; replays converge, prompt
// changes conflict.
func TestV1TaskCreateWithPromptEndToEnd(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	body := `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"a red bicycle","params":{"output_count":1}}}`
	status, created := fx.postTask(t, "prompt-1", body)
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := v1Data(t, created).(map[string]any)["id"].(string)
	typed := fx.canonicalParams(t, taskID)
	if typed.Generation == nil {
		t.Fatal("missing generation snapshot")
	}
	if typed.Generation.UserPrompt != "a red bicycle" || typed.Generation.CompiledPrompt != "a red bicycle" {
		t.Fatalf("generation=%+v, want the submitted prompt", typed.Generation)
	}
	// Same key + same body replays the same task.
	status, replay := fx.postTask(t, "prompt-1", body)
	if status != http.StatusAccepted || replay["data"].(map[string]any)["id"] != taskID {
		t.Fatalf("replay = %d %v, want same task %s", status, replay, taskID)
	}
	// Same key + different prompt conflicts (prompt is in the hash).
	other := `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"a blue bicycle","params":{"output_count":1}}}`
	status, conflict := fx.postTask(t, "prompt-1", other)
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeTaskIdempotencyConflict) {
		t.Fatalf("prompt change = %d %v, want 409 task_idempotency_conflict", status, conflict)
	}
}

// Prompt boundaries: over-long prompts, missing prompts on required
// features, and params.prompt (still unknown_field) all fail 422.
func TestV1TaskCreatePromptBoundaries(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	for _, tc := range []struct{ name, body string }{
		{"over-long prompt", fmt.Sprintf(`{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":%q,"params":{}}}`,
			strings.Repeat("x", workspacetasks.MaxWorkspaceTaskPromptBytes+1))},
		{"missing prompt on generation", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"params":{}}}`},
		{"params prompt still rejected", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"ok","params":{"prompt":"stowaway"}}}`},
	} {
		status, payload := fx.postTask(t, "bound-"+tc.name, tc.body)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tc.name, status, payload)
		}
	}
}

// Inpaint-with-mask: the upload-session mask asset links by id through
// params.mask_asset_id into canonical media facts (typed mask + generation
// intent + plan mask input).
func TestV1TaskCreateInpaintWithMaskEndToEnd(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	body := `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill the hole","params":{"mask_asset_id":"asset_mask1"}}}`
	status, created := fx.postTask(t, "inpaint-mask-1", body)
	if status != http.StatusAccepted {
		t.Fatalf("create = %d %v, want 202", status, created)
	}
	taskID := v1Data(t, created).(map[string]any)["id"].(string)
	typed := fx.canonicalParams(t, taskID)
	if typed.Media == nil || typed.Media.Mask == nil || typed.Media.Mask.PublicAssetID != "asset_mask1" {
		t.Fatalf("media=%+v, want mask public asset id", typed.Media)
	}
	if typed.Generation == nil || len(typed.Generation.Media) != 2 {
		t.Fatalf("generation=%+v, want input + mask intents", typed.Generation)
	}
	if typed.Generation.CompiledPrompt != "fill the hole" {
		t.Fatalf("generation=%+v, want the submitted prompt", typed.Generation)
	}
}

// Mask policy: inpaint without a mask, a mask on a non-inpaint feature,
// and data-URL masks all fail 422 (strict union preserved).
func TestV1TaskCreateMaskPolicy(t *testing.T) {
	fx := newV1PromptMaskFixture(t)
	for _, tc := range []struct{ name, body string }{
		{"inpaint without mask", `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill it","params":{}}}`},
		{"mask on render", `{"data":{"service":"render","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"edit it","params":{"mask_asset_id":"asset_mask1"}}}`},
		{"data-url mask", `{"data":{"service":"inpaint","inputs":{"media_asset_ids":["asset_input1"]},"prompt":"fill it","params":{"mask_asset_id":"data:image/png;base64,AAAA"}}}`},
		{"unknown params still rejected", `{"data":{"service":"generation","inputs":{"media_asset_ids":[]},"prompt":"ok","params":{"mystery":1}}}`},
	} {
		status, payload := fx.postTask(t, "maskpol-"+tc.name, tc.body)
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("%s = %d %v, want 422 validation_failed", tc.name, status, payload)
		}
	}
}
