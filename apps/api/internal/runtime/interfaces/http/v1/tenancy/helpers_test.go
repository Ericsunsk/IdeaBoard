package tenancy

import (
	"net/http"
	"testing"

	domainboards "ideaboard/api/internal/boards"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	"ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

var v1Data = testsupport.Data
var v1ErrorCode = testsupport.ErrorCode
var v1WorkspaceFixture = testsupport.WorkspaceFixture
var testUUIDForUser = testsupport.TestUUIDForUser
var configureV1WorkspaceDefaultQuota = testsupport.ConfigureWorkspaceDefaultQuota
var v1OrganizationWorkspaceFixture = testsupport.OrganizationWorkspaceFixture
var testInvitations = testsupport.TestInvitations
var testInvitationReader = testsupport.TestInvitationReader
var v1GapsErrorFields = testsupport.ErrorFields

type billingAccountStore = shared.AccountStore

type ContractV1Deps = Deps
type ContractV1MemberDeps = MemberDeps
type contractV1Handler = Handler

func RegisterContractV1Routes(group *gin.RouterGroup, _ runtimehttp.AuthDeps, deps ContractV1Deps) {
	h := NewHandler(deps)
	h.Mount(group)
	h.MountPublic(group)
}

func RegisterContractV1RoutesOnEngine(router *gin.Engine, _ runtimehttp.AuthDeps, deps ContractV1Deps) {
	group := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(group, runtimehttp.AuthDeps{}, deps)
}

func newV1MediaTestEngine(store domaintenancy.Store, dir *staticUserDirectory, deps ContractV1Deps, withGlobalMiddleware ...bool) *gin.Engine {
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
	RegisterContractV1RoutesOnEngine(router, runtimehttp.AuthDeps{}, deps)
	return router
}

func newV1TestEngine(store domaintenancy.Store, dir *staticUserDirectory) *gin.Engine {
	var userDir *testsupport.StaticUserDirectory
	if dir != nil {
		userDir = dir.StaticUserDirectory
	}
	router := testsupport.NewTestEngine(userDir)
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	var boardStore domainboards.WorkspaceBoardStore
	if membershipScope, ok := store.(domainboards.WorkspaceMembershipScope); ok {
		boardStore = domainboards.NewInMemoryWorkspaceBoardStoreForTests(membershipScope, dir)
	}
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store:       store,
		Users:       userDir,
		Boards:      boardStore,
		Invitations: testInvitations(store, nil, nil, NewInvitationIdentityReader(nil, nil, userDir)),
	})
	return router
}
