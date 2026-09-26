package httpapi

// Control Plane tests (issue #808): authorization matrix, audit record
// fields, header-only idempotency replay/conflict, reason-required 400,
// contract envelope + Request-Id, separate rate-limit bucket, scoped
// task access without a creation path, and the events-layer exclusion
// guard. Legacy /api/admin routes are not touched here.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billing/application"
	creditshealth "ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/domain"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// controlFixtureUsers are deterministic operator fixtures: 11 is the
// admin operator (and tenancy owner), 21/22 regular user targets, 23 an
// admin user target (impersonation must refuse it).
const (
	controlOperatorID int64 = 11
	controlUserAID    int64 = 21
	controlUserBID    int64 = 22
	controlAdminTID   int64 = 23
)

type controlCall struct {
	method  string
	path    string
	body    string
	userID  int64
	role    string
	caps    []string
	headers map[string]string
}

var controlRequestSeq atomic.Int64

func newControlTestEngine(deps ContractControlDeps) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	dir := newStaticUserDirectory()
	for _, id := range []int64{controlOperatorID, controlUserAID, controlUserBID, controlAdminTID} {
		dir.ensure(id)
	}
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
		var p principal.Principal
		if rawCaps := strings.TrimSpace(c.GetHeader("X-Test-Caps")); rawCaps != "" {
			var caps []string
			for _, cap := range strings.Split(rawCaps, ",") {
				if trimmed := strings.TrimSpace(cap); trimmed != "" {
					caps = append(caps, trimmed)
				}
			}
			p, err = principal.NewExtended(userID, id,
				fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role,
				principal.SubjectHumanSession, "session", "", caps)
		} else {
			p, err = principal.New(userID, id,
				fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		}
		if err != nil || principal.Set(c, p) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	control := router.Group(ContractControlGroupPath)
	control.Use(ContractRequestIDMiddleware())
	RegisterContractControlRoutes(control, AuthDeps{}, deps)
	return router
}

func doControl(t *testing.T, engine *gin.Engine, call controlCall) (int, http.Header, map[string]any) {
	t.Helper()
	reader := strings.NewReader(call.body)
	req := httptest.NewRequest(call.method, call.path, reader)
	if call.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	requestID := fmt.Sprintf("test-control-req-%d", controlRequestSeq.Add(1))
	req.Header.Set(contract.RequestIDHeader, requestID)
	if call.userID > 0 {
		req.Header.Set("X-Test-User", strconv.FormatInt(call.userID, 10))
	}
	if call.role != "" {
		req.Header.Set("X-Test-Role", call.role)
	}
	if len(call.caps) > 0 {
		req.Header.Set("X-Test-Caps", strings.Join(call.caps, ","))
	}
	for key, value := range call.headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if got := recorder.Header().Get(contract.RequestIDHeader); got != requestID {
		t.Fatalf("%s %s: Request-Id = %q, want %q", call.method, call.path, got, requestID)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		// SSE/streaming or empty bodies are not used on control; every
		// control response must be a JSON envelope.
		t.Fatalf("%s %s: response is not JSON: %v; body=%s", call.method, call.path, err, recorder.Body.String())
	}
	return recorder.Code, recorder.Header(), payload
}

func controlErrorCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %v", payload)
	}
	code, _ := errBody["code"].(string)
	if code == "" {
		t.Fatalf("error envelope requires code: %v", payload)
	}
	requestID, _ := errBody["request_id"].(string)
	if requestID == "" {
		t.Fatalf("error envelope requires request_id: %v", payload)
	}
	if _, hasData := payload["data"]; hasData {
		t.Fatalf("error envelope must not carry data: %v", payload)
	}
	return code
}

func controlData(t *testing.T, payload map[string]any) any {
	t.Helper()
	data, ok := payload["data"]
	if !ok {
		t.Fatalf("success envelope requires data: %v", payload)
	}
	if _, hasError := payload["error"]; hasError {
		t.Fatalf("success envelope must not carry error: %v", payload)
	}
	return data
}

// fakeControlUsers serves deterministic user reads keyed by UUID string.
type fakeControlUsers struct {
	mu    sync.Mutex
	users map[string]*identitydomain.User
}

func newFakeControlUsers() *fakeControlUsers {
	mk := func(id int64, role string, status int16, authUserID string) *identitydomain.User {
		publicID := testUUIDForUser(id)
		now := time.Now().UTC()
		return &identitydomain.User{
			ID: id, PublicID: publicID, DisplayName: fmt.Sprintf("user-%d", id),
			AuthUserID: &authUserID, Role: role, Status: status,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	f := &fakeControlUsers{users: map[string]*identitydomain.User{}}
	for _, u := range []*identitydomain.User{
		mk(controlOperatorID, "admin", identitydomain.StatusActive, "auth-11"),
		mk(controlUserAID, "user", identitydomain.StatusActive, "auth-21"),
		mk(controlUserBID, "user", identitydomain.StatusActive, "auth-22"),
		mk(controlAdminTID, "admin", identitydomain.StatusActive, "auth-23"),
	} {
		f.users[u.PublicID.String()] = u
	}
	return f
}

func (f *fakeControlUsers) ListUsers(_ context.Context, req *identityports.UserListRequest) (*identityports.UserListResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := &identityports.UserListResponse{}
	for _, u := range f.users {
		if req != nil && req.Role != "" && u.Role != req.Role {
			continue
		}
		resp.List = append(resp.List, &identityports.AdminUserListItem{
			PublicID: u.PublicID, DisplayName: u.DisplayName, Role: u.Role,
			Status: u.Status, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		})
	}
	resp.Total = int64(len(resp.List))
	return resp, nil
}

func (f *fakeControlUsers) GetByPublicID(_ context.Context, publicID string) (*identitydomain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[strings.TrimSpace(publicID)]; ok {
		return u, nil
	}
	return nil, database.ErrNotFound
}

// fakeIdentityAdminCommands records mutation calls for audit/cancel tests.
type fakeIdentityAdminCommands struct {
	mu     sync.Mutex
	calls  []string
	actors []int64
	fail   error
}

func (f *fakeIdentityAdminCommands) record(name string, actor principal.Principal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	f.actors = append(f.actors, actor.UserID())
}

func (f *fakeIdentityAdminCommands) SetRole(_ context.Context, actor principal.Principal, _ string, _ string) error {
	f.record("set_role", actor)
	return f.fail
}

func (f *fakeIdentityAdminCommands) Ban(_ context.Context, actor principal.Principal, _ string, _ identityports.IdentityAdminBanInput) error {
	f.record("ban", actor)
	return f.fail
}

func (f *fakeIdentityAdminCommands) Unban(_ context.Context, actor principal.Principal, _ string) error {
	f.record("unban", actor)
	return f.fail
}

func (f *fakeIdentityAdminCommands) RevokeSessions(_ context.Context, actor principal.Principal, _ string) error {
	f.record("revoke_sessions", actor)
	return f.fail
}

func (f *fakeIdentityAdminCommands) Delete(_ context.Context, actor principal.Principal, _ string) error {
	f.record("delete", actor)
	return f.fail
}

func (f *fakeIdentityAdminCommands) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

type fakeImpersonator struct {
	cookies []string
	err     error
}

func (f *fakeImpersonator) ImpersonateUser(_ context.Context, _ string, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.cookies) > 0 {
		return f.cookies, nil
	}
	return []string{"better-auth.session_token=impersonated; Path=/; HttpOnly"}, nil
}

// fakeControlCredits counts commands, captures metadata, and can force
// domain failures for error-mapping tests.
type fakeControlCredits struct {
	mu         sync.Mutex
	calls      int
	lastMeta   domain.CommandMetadata
	lastKind   string
	lastUserID int64
	fail       error
}

func (f *fakeControlCredits) result(userID int64, kind string, amount domain.CreditAmount, meta domain.CommandMetadata) (creditsports.CommandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastKind = kind
	f.lastUserID = userID
	f.lastMeta = meta
	if f.fail != nil {
		return creditsports.CommandResult{}, f.fail
	}
	return creditsports.CommandResult{
		TransactionID: "ctxn_test_" + kind,
		CommandType:   domain.CommandGrant,
		Amount:        amount.Int64(),
		Wallet:        domain.WalletBalance{UserID: userID, Available: 100, Reserved: 0},
		CreatedAt:     time.Now().UTC(),
	}, nil
}

func (f *fakeControlCredits) Grant(_ context.Context, c creditsports.GrantCommand) (creditsports.CommandResult, error) {
	return f.result(c.UserID, "grant", c.Amount, c.Metadata)
}

func (f *fakeControlCredits) Deduct(_ context.Context, c creditsports.DeductionCommand) (creditsports.CommandResult, error) {
	return f.result(c.UserID, "deduct", c.Amount, c.Metadata)
}

func (f *fakeControlCredits) Correct(_ context.Context, c creditsports.CorrectionCommand) (creditsports.CommandResult, error) {
	return f.result(c.UserID, "correct", c.Amount, c.Metadata)
}

func (f *fakeControlCredits) Refund(_ context.Context, c creditsports.RefundCommand) (creditsports.CommandResult, error) {
	return f.result(c.UserID, "refund", c.Amount, c.Metadata)
}

type fakeControlCreditDirectory struct{}

func (fakeControlCreditDirectory) ResolvePublicID(_ context.Context, raw string) (credithttp.UserIdentity, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return credithttp.UserIdentity{}, database.ErrNotFound
	}
	for _, id := range []int64{controlOperatorID, controlUserAID, controlUserBID, controlAdminTID} {
		if testUUIDForUser(id) == parsed {
			return credithttp.UserIdentity{ID: id, PublicID: parsed, DisplayName: fmt.Sprintf("user-%d", id)}, nil
		}
	}
	return credithttp.UserIdentity{}, database.ErrNotFound
}

func (fakeControlCreditDirectory) ResolveID(_ context.Context, id int64) (credithttp.UserIdentity, error) {
	for _, known := range []int64{controlOperatorID, controlUserAID, controlUserBID, controlAdminTID} {
		if known == id {
			return credithttp.UserIdentity{ID: id, PublicID: testUUIDForUser(id), DisplayName: fmt.Sprintf("user-%d", id)}, nil
		}
	}
	return credithttp.UserIdentity{}, database.ErrNotFound
}

type fakeControlCreditReads struct{}

func (fakeControlCreditReads) GetWallet(_ context.Context, userID int64) (domain.WalletDetails, error) {
	return domain.WalletDetails{WalletBalance: domain.WalletBalance{
		UserID: userID, PublicID: testUUIDForUser(userID),
		DisplayName: fmt.Sprintf("user-%d", userID), Available: 100, Reserved: 5,
	}}, nil
}

func (fakeControlCreditReads) ListLedger(_ context.Context, input domain.ListLedgerInput) (domain.LedgerPage, error) {
	return domain.LedgerPage{Transactions: []domain.LedgerTransaction{}}, nil
}

type fakeControlReconciliation struct{}

func (fakeControlReconciliation) GetCreditHealth(_ context.Context) (creditshealth.CreditHealth, error) {
	return creditshealth.CreditHealth{}, nil
}

func (fakeControlReconciliation) StartReconciliation(_ context.Context, input creditsports.StartManualReconciliationInput) (creditsports.ReconciliationRun, error) {
	return creditsports.ReconciliationRun{Reason: input.Reason}, nil
}

func (fakeControlReconciliation) GetReconciliationRun(_ context.Context, runID int64) (creditsports.ReconciliationRun, error) {
	return creditsports.ReconciliationRun{}, nil
}

func (fakeControlReconciliation) ListReconciliationRuns(_ context.Context, _ creditsports.ListReconciliationInput) (creditsports.ReconciliationRunPage, error) {
	return creditsports.ReconciliationRunPage{Runs: []creditsports.ReconciliationRun{}}, nil
}

func (fakeControlReconciliation) ListGlobalReconciliationFindings(_ context.Context, _ creditsports.ReconciliationFindingStatus, _ creditsports.ListReconciliationInput) (creditsports.ReconciliationFindingPage, error) {
	return creditsports.ReconciliationFindingPage{Findings: []creditsports.ReconciliationFinding{}}, nil
}

type fakeControlTasks struct {
	mu           sync.Mutex
	cancelCalls  int
	cancelActor  int64
	cancelTaskID string
	cancelReason string
	failCancel   error
	failGet      error
	failList     error
}

func (f *fakeControlTasks) GetTask(_ context.Context, taskID string) (*taskapp.AdminMonitorDetail, error) {
	if f.failGet != nil {
		return nil, f.failGet
	}
	return &taskapp.AdminMonitorDetail{}, nil
}

func (f *fakeControlTasks) ListTasks(_ context.Context, req *tasksdomain.TaskListRequest) ([]taskapp.AdminMonitorListItem, tasksdomain.TaskListCounts, error) {
	if f.failList != nil {
		return nil, tasksdomain.TaskListCounts{}, f.failList
	}
	return []taskapp.AdminMonitorListItem{}, tasksdomain.TaskListCounts{}, nil
}

func (f *fakeControlTasks) ResolveAdminTaskUserID(_ context.Context, raw string) (int64, error) {
	return controlUserAID, nil
}

func (f *fakeControlTasks) CancelAdminTask(_ context.Context, taskID string, adminID int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls++
	f.cancelActor = adminID
	f.cancelTaskID = taskID
	f.cancelReason = reason
	return f.failCancel
}

type fakeControlCatalog struct {
	mu            sync.Mutex
	publishActor  int64
	publishPrices map[billing.OutputResolution]int64
}

func (f *fakeControlCatalog) List(_ context.Context) ([]application.BillableItemCatalogCard, error) {
	return []application.BillableItemCatalogCard{}, nil
}

func (f *fakeControlCatalog) PublishPrices(_ context.Context, _ string, prices map[billing.OutputResolution]int64, actorID int64) (map[billing.OutputResolution]billing.PriceInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishActor = actorID
	f.publishPrices = prices
	return map[billing.OutputResolution]billing.PriceInfo{}, nil
}

func (f *fakeControlCatalog) SetAvailability(_ context.Context, _ string, _ bool, _ int64) (billing.AvailabilityInfo, error) {
	return billing.AvailabilityInfo{}, nil
}

func (f *fakeControlCatalog) DeleteArchived(_ context.Context, _ string, _ int64) (billing.ArchivedItemDeleteResult, error) {
	return billing.ArchivedItemDeleteResult{}, nil
}

type fakeControlPayments struct {
	mu             sync.Mutex
	refundCmd      paymentcontracts.CreatePaymentRefundCommand
	refundCalls    int
	configureCalls int
}

func (f *fakeControlPayments) GetStripeConfiguration(_ context.Context) (paymentcontracts.StripeConfigurationSummary, error) {
	return paymentcontracts.StripeConfigurationSummary{}, nil
}

func (f *fakeControlPayments) ConfigureStripe(_ context.Context, cmd paymentcontracts.ConfigureStripeCommand) (paymentcontracts.StripeConfigurationSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configureCalls++
	return paymentcontracts.StripeConfigurationSummary{}, nil
}

func (f *fakeControlPayments) ListAdminPaymentOrders(_ context.Context, _ paymentcontracts.AdminPaymentOrderListInput) (paymentcontracts.AdminPaymentOrderPage, error) {
	return paymentcontracts.AdminPaymentOrderPage{}, nil
}

func (f *fakeControlPayments) GetAdminPaymentOrder(_ context.Context, orderNumber string) (paymentcontracts.AdminPaymentOrderDetail, error) {
	return paymentcontracts.AdminPaymentOrderDetail{PublicOrderNumber: orderNumber}, nil
}

func (f *fakeControlPayments) CreatePaymentRefund(_ context.Context, cmd paymentcontracts.CreatePaymentRefundCommand) (paymentsdomain.PaymentRefund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refundCalls++
	f.refundCmd = cmd
	return paymentsdomain.PaymentRefund{PublicOrderNumber: cmd.OrderNumber}, nil
}

type fakeControlBillingAccounts struct{}

func (fakeControlBillingAccounts) EnsureBillingAccountForWorkspace(_ context.Context, workspaceInternalID int64, workspacePublicID string) (tenancypg.BillingAccount, error) {
	return tenancypg.BillingAccount{
		WorkspaceInternalID: workspaceInternalID, WorkspacePublicID: workspacePublicID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}, nil
}
