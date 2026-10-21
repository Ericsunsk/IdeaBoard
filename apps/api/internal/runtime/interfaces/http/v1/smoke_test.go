package v1

// Phase 4-04 (#821) production-wiring smoke: all three restored surfaces
// (library folders/assets, tasks, uploads/media) live behind ONE engine
// with production-shaped deps (TaskAssets is the real
// workspacemedia.TaskAssetAdapter over the in-memory media store).
//
// Covers: every surface answers non-501; the cross-module composition
// (upload -> asset -> task input -> task output); the 404-vs-403 matrix
// spot-check (outsiders converge to 404, member-without-capability to
// 403, unwired binary-read seam to 503, deleted materialization route to
// 404); and a light
// store-level concurrency sanity with reported timings (no p95 gate).

import (
	"context"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type phase4Fixture struct {
	engine *gin.Engine
	store  *tenancy.InMemoryStore
	dir    *staticUserDirectory
	ws     *tenancy.Workspace
	tasks  *workspacetasks.InMemoryStore
	media  *workspacemedia.InMemoryWorkspaceMediaStore
	lib    *librarymemory.InMemoryWorkspaceLibraryStore
	queue  *fakeTaskQueue
}

func newPhase4Fixture(t *testing.T) *phase4Fixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := workspacetasks.NewInMemoryStoreForTests()
	txTasks := newV1TxTaskStore(tasks)
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	saved := make(map[string]routing.RoutingSnapshot)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "phase4", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 2, tenancy.RoleMember)
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir,
		Library: ContractV1LibraryDeps{Store: lib},
		Media:   media,
		// MediaObjects stays nil in this fixture: binary reads fail closed
		// with 503 while metadata keeps working. The production task worker
		// has its own renderer; this HTTP fixture does not exercise it.
		Tasks:         txTasks,
		TaskLifecycle: txTasks,
		TaskQuoter:    quoter,
		TaskHolder:    holder,
		TaskQueue:     queue,
		// Production-shaped asset gate: the real adapter over the
		// media store, so task input/output verification runs the
		// same ready/archived/invisible mapping as production.
		TaskAssets:          &workspacemedia.TaskAssetAdapter{Store: media},
		TaskRouter:          allowTestPoolResolver(),
		TaskSnapshotSaver:   v1TxSnapshotSaver(saved),
		TaskBillingAccounts: v1FakeBillingAccounts{},
		// #917: the preflight resolves the same fixture account submit
		// holds and gates the same fixture funds.
		TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
		TaskQuoteWallets:         newV1FakeQuoteWallets(),
		TaskAcceptance:           &v1TxSubmissionAcceptance{assets: &workspacemedia.TaskAssetAdapter{Store: media}},
	})
	return &phase4Fixture{engine: router, store: store, dir: dir, ws: ws, tasks: tasks, media: media, lib: lib, queue: queue}
}

func phase4Call(t *testing.T, fx *phase4Fixture, method, path, body string, userID int64) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{"Idempotency-Key": fmt.Sprintf("phase4-%d", v1TestRequestSeq.Add(1))}
	if method == http.MethodGet && !strings.Contains(path, "outputs") {
		delete(headers, "Idempotency-Key")
	}
	status, _, decoded := doV1(t, fx.engine, v1Call{method: method, path: path, body: body, userID: userID, headers: headers})
	return status, decoded
}

const phase4Checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestPhase4SurfacesLive proves every restored surface answers non-501
// and composes: upload -> asset public_id -> task input -> task output.
func TestPhase4SurfacesLive(t *testing.T) {
	fx := newPhase4Fixture(t)
	ws := fx.ws.PublicID

	status, folderBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/library/folders",
		`{"data":{"name":"inspo"}}`, 1)
	if status != http.StatusCreated {
		t.Fatalf("create folder = %d (%v), want 201", status, folderBody)
	}
	folderID, _ := folderBody["data"].(map[string]any)["id"].(string)

	status, saveBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/library/assets",
		`{"data":{"source":"external","provider":"unsplash","source_id":"ph4-1","source_url":"https://images.unsplash.com/ph4-1","preview_url":"https://images.unsplash.com/ph4-1","original_url":"https://images.unsplash.com/ph4-1","folder_id":"`+folderID+`"}}`, 1)
	if status != http.StatusCreated {
		t.Fatalf("save library asset = %d (%v), want 201", status, saveBody)
	}

	status, uploadBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/uploads",
		`{"data":{"filename":"shot.png","mime_type":"image/png","size_bytes":1024,"checksum_sha256":"`+phase4Checksum+`","media_kind":"image"}}`, 1)
	if status != http.StatusCreated {
		t.Fatalf("create upload = %d (%v), want 201", status, uploadBody)
	}
	uploadID, _ := uploadBody["data"].(map[string]any)["id"].(string)

	status, completeBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/uploads/"+uploadID+"/complete",
		`{"data":{"checksum_sha256":"`+phase4Checksum+`","size_bytes":1024,"mime_type":"image/png","media_kind":"image","width":100,"height":80}}`, 1)
	if status != http.StatusOK {
		t.Fatalf("complete upload = %d (%v), want 200", status, completeBody)
	}
	assetID, _ := completeBody["data"].(map[string]any)["asset"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(assetID, "asset_") {
		t.Fatalf("completed asset id = %q, want asset_ shape", assetID)
	}

	for _, get := range []struct{ name, path string }{
		{"get upload", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/uploads/" + uploadID},
		{"list media", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/media-assets?page_size=10"},
		{"get asset", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/media-assets/" + assetID},
		{"list folders", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/library/folders"},
		{"list library assets", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/library/assets?folder_id=" + folderID},
	} {
		if status, body := phase4Call(t, fx, http.MethodGet, get.path, "", 1); status != http.StatusOK {
			t.Fatalf("%s = %d (%v), want 200", get.name, status, body)
		}
	}

	// Task creation gates on the produced asset through the production
	// TaskAssetAdapter (invisible asset ids 404 as media_not_found).
	// The resolution is omitted so the scalable policy default (1k)
	// applies: an explicit "default" is rejected for scalable services
	// (ADR-0012 rule 3).
	status, taskBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/tasks",
		`{"data":{"service":"render","inputs":{"media_asset_ids":["`+assetID+`"]},"params":{},"quantity":1}}`, 1)
	if status != http.StatusAccepted {
		t.Fatalf("create task = %d (%v), want 202", status, taskBody)
	}
	taskID, _ := taskBody["data"].(map[string]any)["id"].(string)
	if location := taskBody["location"]; location != nil {
		t.Logf("task location: %v", location)
	}

	status, badTask := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/tasks",
		`{"data":{"service":"render","inputs":{"media_asset_ids":["asset_missing00000001"]},"params":{}}}`, 1)
	if status != http.StatusNotFound {
		t.Fatalf("create task with invisible asset = %d (%v), want 404 media_not_found", status, badTask)
	}

	for _, get := range []struct{ name, path string }{
		{"quote", ""},
		{"get task", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/tasks/" + taskID},
		{"list tasks", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/tasks?page_size=10"},
		{"task outputs", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/tasks/" + taskID + "/outputs"},
	} {
		if get.name == "quote" {
			status, body := phase4Call(t, fx, http.MethodPost,
				runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/task-quotes",
				`{"data":{"service":"render","quantity":1}}`, 1)
			if status != http.StatusOK {
				t.Fatalf("quote task = %d (%v), want 200", status, body)
			}
			continue
		}
		if status, body := phase4Call(t, fx, http.MethodGet, get.path, "", 1); status != http.StatusOK {
			t.Fatalf("%s = %d (%v), want 200", get.name, status, body)
		}
	}

	status, cancelBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/tasks/"+taskID+"/cancel",
		`{"data":{"reason":"smoke done"}}`, 1)
	if status != http.StatusOK {
		t.Fatalf("cancel task = %d (%v), want 200", status, cancelBody)
	}

	// Owner archives the asset (member 2 lacks the capability: 403,
	// covered in the matrix test below).
	status, archiveBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/media-assets/"+assetID+"/archive", "", 1)
	if status != http.StatusOK {
		t.Fatalf("archive asset = %d (%v), want 200", status, archiveBody)
	}
	if len(fx.queue.enqueued) != 1 || fx.queue.enqueued[0].ID != taskID {
		t.Fatalf("enqueued = %+v, want the created task", fx.queue.enqueued)
	}
}

// TestPhase4AccessMatrix spot-checks the 404-vs-403 convergence across
// all three modules plus the fail-closed 503 seams.
func TestPhase4AccessMatrix(t *testing.T) {
	fx := newPhase4Fixture(t)
	ws := fx.ws.PublicID
	const outsider = int64(9)

	// Outsider (no membership) converges to 404 on every surface.
	for _, get := range []struct{ name, path string }{
		{"library folders", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/library/folders"},
		{"library assets", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/library/assets"},
		{"tasks", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/tasks"},
		{"media assets", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/media-assets"},
		{"uploads", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/uploads/upload_missing00000001"},
	} {
		if status, body := phase4Call(t, fx, http.MethodGet, get.path, "", outsider); status != http.StatusNotFound {
			t.Fatalf("outsider %s = %d (%v), want 404", get.name, status, body)
		}
	}

	// Malformed resource ids converge to 404 (never 400).
	for _, get := range []struct{ name, path string }{
		{"task", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/tasks/nope"},
		{"media asset", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/media-assets/42"},
		{"upload", runtimehttp.ContractV1GroupPath + "/workspaces/" + ws + "/uploads/42"},
	} {
		if status, body := phase4Call(t, fx, http.MethodGet, get.path, "", 1); status != http.StatusNotFound {
			t.Fatalf("malformed %s = %d (%v), want 404", get.name, status, body)
		}
	}
	// Library assets have no GET-by-id route; the malformed shape
	// converges to 404 through the move (PATCH) seam instead.
	if status, body := phase4Call(t, fx, http.MethodPatch,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/library/assets/42",
		`{"data":{"folder_id":""}}`, 1); status != http.StatusNotFound {
		t.Fatalf("malformed library asset = %d (%v), want 404", status, body)
	}

	// Member without owner/admin cannot archive (403, not 404: the
	// workspace is visible, the capability is missing).
	status, uploadBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/uploads",
		`{"data":{"filename":"m.png","mime_type":"image/png","size_bytes":512,"checksum_sha256":"`+phase4Checksum+`","media_kind":"image"}}`, 1)
	if status != http.StatusCreated {
		t.Fatalf("create upload = %d (%v)", status, uploadBody)
	}
	uploadID, _ := uploadBody["data"].(map[string]any)["id"].(string)
	status, completeBody := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/uploads/"+uploadID+"/complete",
		`{"data":{"checksum_sha256":"`+phase4Checksum+`","size_bytes":512,"mime_type":"image/png","media_kind":"image"}}`, 1)
	if status != http.StatusOK {
		t.Fatalf("complete upload = %d (%v)", status, completeBody)
	}
	assetID, _ := completeBody["data"].(map[string]any)["asset"].(map[string]any)["id"].(string)
	if status, body := phase4Call(t, fx, http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/media-assets/"+assetID+"/archive", "", 2); status != http.StatusForbidden {
		t.Fatalf("member archive = %d (%v), want 403", status, body)
	}

	// Deleted materialization route stays gone (404, never 503): the
	// former POST /library/materializations surface was cut
	// owner-approved (unregistered paths serve Gin's plain-text 404,
	// not a JSON envelope, so assert via the raw recorder). Unwired
	// binary reads still fail closed with 503 (never 501/500,
	// MediaObjects nil in this fixture).
	matReq := httptest.NewRequest(http.MethodPost,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/library/materializations",
		strings.NewReader(`{"data":{"library_asset_id":"lasset_missing00000001"}}`))
	testsupport.SetTestUser(matReq, 1)
	matReq.Header.Set("Idempotency-Key", "phase4-deleted-mat-1")
	matRecorder := httptest.NewRecorder()
	fx.engine.ServeHTTP(matRecorder, matReq)
	if matRecorder.Code != http.StatusNotFound {
		t.Fatalf("deleted materialization route = %d, want 404", matRecorder.Code)
	}
	req := httptest.NewRequest(http.MethodGet,
		runtimehttp.ContractV1GroupPath+"/workspaces/"+ws+"/media-assets/"+assetID+"/content", nil)
	testsupport.SetTestUser(req, 1)
	recorder := httptest.NewRecorder()
	fx.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("binary content without objects = %d, want 503", recorder.Code)
	}
}

// TestPhase4ConcurrencySanity drives the three postgres-shaped stores
// (mutex-guarded in-memory siblings) in parallel and reports honest
// timings. No p95 gate is enforced; the numbers below are the report.
func TestPhase4ConcurrencySanity(t *testing.T) {
	fx := newPhase4Fixture(t)
	ctx := context.Background()
	ws := fx.ws.PublicID

	const workers = 24
	timings := make([]time.Duration, 0, workers*3)
	var mu sync.Mutex
	record := func(d time.Duration) {
		mu.Lock()
		timings = append(timings, d)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers*3)
	for i := 0; i < workers; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			_, err := fx.tasks.CreateTask(ctx, &workspacetasks.WorkspaceTask{
				ID: fmt.Sprintf("task_load%04d", i), WorkspaceID: ws, OwnerUserID: 1,
				Service: "render", Status: workspacetasks.StatusPending,
			})
			record(time.Since(start))
			if err != nil {
				errs <- fmt.Errorf("task %d: %w", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			_, err := fx.media.CreateSession(ctx, workspacemedia.CreateSessionParams{
				WorkspaceID: ws, OwnerUserID: 1, Filename: fmt.Sprintf("load-%d.png", i),
				MIMEType: "image/png", SizeBytes: 1024, ChecksumSHA256: phase4Checksum,
				MediaKind: workspacemedia.MediaKindImage,
			})
			record(time.Since(start))
			if err != nil {
				errs <- fmt.Errorf("upload %d: %w", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			_, err := fx.lib.ListFolders(ctx, ws)
			record(time.Since(start))
			if err != nil {
				errs <- fmt.Errorf("library query %d: %w", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
	p50 := timings[len(timings)/2]
	p95 := timings[int(float64(len(timings))*0.95)]
	var total time.Duration
	for _, d := range timings {
		total += d
	}
	t.Logf("phase4 concurrency: %d ops across task-create/upload-create/library-query, avg=%s p50=%s p95=%s max=%s",
		len(timings), total/time.Duration(len(timings)), p50, p95, timings[len(timings)-1])
	count, err := fx.tasks.CountTasks(ctx, ws)
	if err != nil || count != workers {
		t.Fatalf("task count = %d (%v), want %d", count, err, workers)
	}
}
