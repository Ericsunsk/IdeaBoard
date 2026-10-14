package httpapi

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeTaskQuoter struct {
	unitCredits int64
	calls       int
	err         error
}

// allowTestPoolResolver serves every service/resolution from one enabled
// entry (#854: replaces the legacy allowTestRouting gate; the resolver is
// the only availability seam).

func allowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{4, 5, 6}
	return routing.NewRoutingResolver(allowAllV1PoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllV1PoolStore struct{}

func (allowAllV1PoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Model: "openrouter/test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (f *fakeTaskQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	f.calls++
	if f.err != nil {
		return workspacetasks.BillingSnapshot{}, f.err
	}
	if quantity <= 0 {
		quantity = 1
	}
	unit := f.unitCredits
	if unit == 0 {
		unit = 10
	}
	return workspacetasks.BillingSnapshot{
		SKU: "ai-image:" + kind, DisplayName: kind, PriceVersionID: 1, PriceVersion: 1,
		OutputResolution: resolution, UnitCredits: unit, Quantity: quantity,
		TotalCredits: unit * quantity, QuotedAt: time.Now().UTC(), SettlementState: "held",
	}, nil
}

type fakeTaskHolder struct {
	holds    map[string]int64
	captured map[string]int64
	released map[string]int64
	holdErr  error
}

func newFakeTaskHolder() *fakeTaskHolder {
	return &fakeTaskHolder{holds: map[string]int64{}, captured: map[string]int64{}, released: map[string]int64{}}
}

func (f *fakeTaskHolder) Hold(ctx context.Context, userID int64, amount int64, key, taskID string) (string, error) {
	if f.holdErr != nil {
		return "", f.holdErr
	}
	holdID := "hold_" + taskID
	f.holds[holdID] = amount
	return holdID, nil
}

func (f *fakeTaskHolder) Capture(ctx context.Context, holdID string, amount int64, taskID string) error {
	f.captured[holdID] = amount
	return nil
}

func (f *fakeTaskHolder) Release(ctx context.Context, holdID string, amount int64, taskID string) error {
	f.released[holdID] = amount
	return nil
}

type fakeTaskQueue struct {
	enqueued []*workspacetasks.WorkspaceTask
	failNext error
}

func (f *fakeTaskQueue) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.enqueued = append(f.enqueued, task)
	return nil
}

type fakeTaskAssets struct {
	assets map[string]*workspacetasks.AssetInfo
}

func (f *fakeTaskAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacetasks.AssetInfo, error) {
	info, ok := f.assets[workspaceID+"\x00"+assetID]
	if !ok {
		return nil, workspacetasks.ErrMediaNotFound
	}
	return info, nil
}

type v1TaskFixture struct {
	engine *gin.Engine
	store  *tenancy.InMemoryStore
	dir    *staticUserDirectory
	ws     *tenancy.Workspace
	tasks  *workspacetasks.InMemoryStore
	quoter *fakeTaskQuoter
	holder *fakeTaskHolder
	queue  *fakeTaskQueue
	assets *fakeTaskAssets
}

func newV1TaskFixture(t *testing.T, pools ...map[string]routing.Pool) *v1TaskFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	tasks := workspacetasks.NewInMemoryStoreForTests()
	quoter := &fakeTaskQuoter{}
	holder := newFakeTaskHolder()
	queue := &fakeTaskQueue{}
	assets := &fakeTaskAssets{assets: map[string]*workspacetasks.AssetInfo{}}
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
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
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
	v1 := router.Group(ContractV1GroupPath)
	// An explicit pool map serves quote/submit through the shared
	// RoutingResolver; the default fixture allows every
	// service/resolution via one enabled entry. An empty pool map
	// converges to 422 service_resolution_unavailable.
	resolver := allowTestPoolResolver()
	if len(pools) > 0 {
		resolver = routing.NewRoutingResolver(&v1StubPoolStore{pools: pools[0]})
	}
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{
		Store: store, Users: dir,
		Tasks: tasks, TaskQuoter: quoter, TaskHolder: holder, TaskQueue: queue, TaskAssets: assets,
		AllowNonTransactionalTaskSettlement: true,
		TaskRouter:                          resolver,
	})
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "tasks", OwnerUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, 2, tenancy.RoleAdmin)
	store.SeedWorkspaceMember(ws.PublicID, 3, tenancy.RoleMember)
	for _, id := range []int64{1, 2, 3, 9} {
		dir.ensure(id)
	}
	return &v1TaskFixture{engine: router, store: store, dir: dir, ws: ws, tasks: tasks, quoter: quoter, holder: holder, queue: queue, assets: assets}
}

func v1CreateTaskBodyJSON(kind string) string {
	return fmt.Sprintf(`{"data":{"service":%q,"inputs":{"media_asset_ids":[]},"params":{}}}`, kind)
}

type v1StubPoolStore struct {
	pools map[string]routing.Pool
}

func (s *v1StubPoolStore) ListPool(ctx context.Context, service string) (routing.Pool, error) {
	if p, ok := s.pools[service]; ok {
		return p, nil
	}
	return routing.Pool{Service: service}, nil
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
