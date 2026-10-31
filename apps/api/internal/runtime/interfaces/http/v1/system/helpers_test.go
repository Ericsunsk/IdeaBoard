package system

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1boards "ideaboard/api/internal/runtime/interfaces/http/v1/boards"
	v1library "ideaboard/api/internal/runtime/interfaces/http/v1/library"
	v1media "ideaboard/api/internal/runtime/interfaces/http/v1/media"
	v1tasks "ideaboard/api/internal/runtime/interfaces/http/v1/tasks"
	v1tenancy "ideaboard/api/internal/runtime/interfaces/http/v1/tenancy"
	"ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type v1Call struct {
	method  string
	path    string
	body    string
	userID  int64
	headers map[string]string
}

type staticUserDirectory struct {
	*testsupport.StaticUserDirectory
}

func (d *staticUserDirectory) ensure(userID int64) uuid.UUID {
	return d.Ensure(userID)
}

func newStaticUserDirectory() *staticUserDirectory {
	return &staticUserDirectory{StaticUserDirectory: testsupport.NewStaticUserDirectory()}
}

func doV1(t *testing.T, engine *gin.Engine, call v1Call) (int, http.Header, map[string]any) {
	t.Helper()
	return testsupport.Do(t, engine, testsupport.Call{
		Method:  call.method,
		Path:    call.path,
		Body:    call.body,
		UserID:  call.userID,
		Headers: call.headers,
	})
}

func doV1RoundTrip(t *testing.T, engine *gin.Engine, call v1Call, reqIDPrefix string) (int, http.Header, string) {
	t.Helper()
	return testsupport.DoRoundTrip(t, engine, testsupport.Call{
		Method:  call.method,
		Path:    call.path,
		Body:    call.body,
		UserID:  call.userID,
		Headers: call.headers,
	}, reqIDPrefix)
}

var v1Data = testsupport.Data
var v1ErrorCode = testsupport.ErrorCode
var v1WorkspaceFixture = testsupport.WorkspaceFixture
var testUUIDForUser = testsupport.TestUUIDForUser
var v1ErrorFields = testsupport.ErrorFields
var configureV1WorkspaceDefaultQuota = testsupport.ConfigureWorkspaceDefaultQuota
var v1GapsDeclare = testsupport.GapsDeclare
var v1GapsStage = testsupport.GapsStage
var v1GapsComplete = testsupport.GapsComplete
var v1GapsChecksum = testsupport.GapsChecksum
var v1MediaAssetID = testsupport.MediaAssetID
var testInvitations = testsupport.TestInvitations
var NewInvitationIdentityReader = testsupport.NewInvitationIdentityReader

type fakeTaskQuoter = testsupport.FakeTaskQuoter
var allowTestPoolResolver = testsupport.AllowTestPoolResolver
type v1FakeBillingAccounts = testsupport.FakeBillingAccounts
type v1FakeQuoteWallets = testsupport.FakeQuoteWallets
var newV1FakeQuoteWallets = testsupport.NewFakeQuoteWallets
type fakeTaskHolder = testsupport.FakeTaskHolder
var newFakeTaskHolder = testsupport.NewFakeTaskHolder
type fakeTaskQueue = testsupport.FakeTaskQueue
type v1TxTaskStore = testsupport.TxTaskStore
var newV1TxTaskStore = testsupport.NewTxTaskStore
var v1TxSnapshotSaver = testsupport.TxSnapshotSaver

type fakeTaskAssets struct {
	assets map[string]*taskdomain.AssetInfo
}

func (f *fakeTaskAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*taskdomain.AssetInfo, error) {
	if f == nil || f.assets == nil {
		return nil, taskdomain.ErrMediaNotFound
	}
	info, ok := f.assets[workspaceID+"\x00"+assetID]
	if !ok {
		return nil, taskdomain.ErrMediaNotFound
	}
	return info, nil
}

type v1TxSubmissionAcceptance struct {
	boards taskports.BoardAuthorizer
	assets taskports.AssetReader
}

func (f *v1TxSubmissionAcceptance) AuthorizeSubmission(ctx context.Context, tx pgx.Tx, input taskdomain.CreateInput) (taskdomain.TaskBoardBinding, error) {
	t := &testsupport.TxSubmissionAcceptance{Boards: f.boards, Assets: f.assets}
	return t.AuthorizeSubmission(ctx, tx, input)
}

func v1CreateTaskBodyJSON(kind string) string {
	return fmt.Sprintf(`{"data":{"service":%q,"inputs":{"media_asset_ids":[]},"params":{}}}`, kind)
}

const v1TestAssetURL = "https://images.unsplash.com/photo-test"

type fakeLibraryBilling struct {
	mu         sync.Mutex
	holdErr    error
	captures   int
	releases   int
	lastHoldWS string
	lastHoldBy int64
}

func (f *fakeLibraryBilling) Hold(ctx context.Context, workspacePublicID string, actorUserID int64, amountCredits int64, idempotencyKey, reason string) (string, error) {
	if f.holdErr != nil {
		return "", f.holdErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastHoldWS, f.lastHoldBy = workspacePublicID, actorUserID
	return "hold-1", nil
}

func (f *fakeLibraryBilling) Capture(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captures++
	return nil
}

func (f *fakeLibraryBilling) Release(ctx context.Context, workspacePublicID string, actorUserID int64, holdID string, amountCredits int64, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases++
	return nil
}

type fakeLibraryQuoter struct {
	amount int64
	err    error
}

func (f *fakeLibraryQuoter) Quote(ctx context.Context, workspacePublicID, sku string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.amount, nil
}

type staticMemberDisplayProvider = testsupport.StaticMemberDisplayProvider
var testInvitationReader = testsupport.TestInvitationReader

var v1GapsPNGBytes = testsupport.GapsPNGBytes
var v1GapsErrorFields = testsupport.ErrorFields

type workspaceEventViewer = WorkspaceEventViewer
type contractV1Handler = Handler

type ContractV1TaskDeps = v1tasks.ContractV1TaskDeps
type ContractV1MediaDeps = v1media.ContractV1MediaDeps
type ContractV1LibraryDeps = v1library.ContractV1LibraryDeps

type ContractV1Deps = Deps

func RegisterContractV1Routes(group *gin.RouterGroup, _ runtimehttp.AuthDeps, deps ContractV1Deps) {
	if deps.Idempotency == nil {
		deps.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	sysHandler := NewHandler(deps)
	sysHandler.Mount(group)
	sysHandler.MountPublic(group)

	if deps.Boards != nil {
		boardHandler := v1boards.NewHandler(v1boards.Deps{
			Store:                     deps.Store,
			Users:                     deps.Users,
			Boards:                    deps.Boards,
			Idempotency:               deps.Idempotency,
			EmitWorkspaceEvent:        sysHandler.EmitWorkspaceEvent,
			DeliverBoardAccessSignals: sysHandler.DeliverBoardAccessSignals,
		})
		boardHandler.Mount(group)
	}

	if deps.Media.Media != nil || deps.Media.MediaObjects != nil {
		mediaHandler := v1media.NewHandler(v1media.Deps{
			Store:              deps.Store,
			Media:              deps.Media,
			Boards:             deps.Boards,
			Idempotency:        deps.Idempotency,
			EmitWorkspaceEvent: sysHandler.EmitWorkspaceEvent,
		})
		mediaHandler.Mount(group)
		mediaHandler.MountPublic(group)
	}

	if deps.Tasks.Tasks != nil {
		taskHandler := v1tasks.NewHandler(v1tasks.Deps{
			Store:                    deps.Store,
			Users:                    deps.Users,
			Boards:                   deps.Boards,
			Idempotency:              deps.Idempotency,
			Tasks:                    deps.Tasks,
			EmitWorkspaceEventWithID: sysHandler.EmitWorkspaceEventWithID,
			EmitTaskBillingSettled:   sysHandler.EmitTaskBillingSettled,
		})
		taskHandler.Mount(group)
	}

	if deps.Library.Store != nil {
		libHandler := v1library.NewHandler(v1library.Deps{
			Store:   deps.Store,
			Library: deps.Library,
		})
		libHandler.Mount(group)
	}

	if deps.Store != nil {
		tenancyHandler := v1tenancy.NewHandler(v1tenancy.Deps{
			Store:                     deps.Store,
			Users:                     deps.Users,
			Idempotency:               deps.Idempotency,
			Invitations:               deps.Invitations,
			InvitationReader:          deps.InvitationReader,
			Boards:                    deps.Boards,
			DeliverBoardAccessSignals: sysHandler.DeliverBoardAccessSignals,
		})
		tenancyHandler.Mount(group)
		tenancyHandler.MountPublic(group)
	}
}

func newV1MediaTestEngine(store tenancy.Store, dir *staticUserDirectory, deps ContractV1Deps, withGlobalMiddleware ...bool) *gin.Engine {
	var userDir *testsupport.StaticUserDirectory
	if dir != nil {
		userDir = dir.StaticUserDirectory
	}
	router := testsupport.NewTestEngine(userDir)
	if len(withGlobalMiddleware) > 0 && withGlobalMiddleware[0] {
		runtimehttp.RegisterRoutes(router, runtimehttp.Dependencies{
			CORSMiddleware: func(c *gin.Context) { c.Next() },
		}, nil, runtimehttp.FrontendRouteOptions{})
	}
	deps.Store = store
	deps.Users = userDir
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, deps)
	return router
}
