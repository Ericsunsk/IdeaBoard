package credits

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type adminHandlerServiceStub struct {
	startInput StartManualReconciliationInput
	startCalls int
}

func (s *adminHandlerServiceStub) Grant(context.Context, GrantCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (s *adminHandlerServiceStub) Deduct(context.Context, DeductionCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (s *adminHandlerServiceStub) Correct(context.Context, CorrectionCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (s *adminHandlerServiceStub) Refund(context.Context, RefundCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (s *adminHandlerServiceStub) GetWallet(context.Context, int64) (WalletDetails, error) {
	return WalletDetails{}, nil
}

func (s *adminHandlerServiceStub) ListLedger(context.Context, ListLedgerInput) (LedgerPage, error) {
	return LedgerPage{}, nil
}

func (s *adminHandlerServiceStub) GetCreditHealth(context.Context) (CreditHealth, error) {
	return CreditHealth{Status: CreditHealthHealthy}, nil
}

func (s *adminHandlerServiceStub) StartReconciliation(_ context.Context, input StartManualReconciliationInput) (ReconciliationRun, error) {
	s.startInput = input
	s.startCalls++
	requestedBy := input.Action.ActorID()
	return ReconciliationRun{
		ID:          1,
		Status:      ReconciliationRunQueued,
		Trigger:     ReconciliationTriggerManual,
		RequestedBy: &requestedBy,
		Reason:      input.Action.Reason(),
	}, nil
}

func (s *adminHandlerServiceStub) GetReconciliationRun(context.Context, int64) (ReconciliationRun, error) {
	return ReconciliationRun{}, nil
}

func (s *adminHandlerServiceStub) ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error) {
	return ReconciliationRunPage{}, nil
}

func (s *adminHandlerServiceStub) ListGlobalReconciliationFindings(context.Context, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error) {
	return ReconciliationFindingPage{}, nil
}

func TestStartReconciliationHandlerRequiresAndTrimsReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &adminHandlerServiceStub{}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	router.POST("/runs", handler.StartReconciliation)

	for _, body := range []string{
		`{}`,
		`{"reason":"  "}`,
		`{"reason":"` + strings.Repeat("理", adminaction.MaxReasonRunes+1) + `"}`,
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body length=%d status=%d response=%s", len(body), response.Code, response.Body.String())
		}
	}
	if service.startCalls != 0 {
		t.Fatalf("StartReconciliation calls=%d before valid request", service.startCalls)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"reason":"  manual audit  "}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d response=%s", response.Code, response.Body.String())
	}
	if service.startCalls != 1 || service.startInput.Action.ActorID() != 7 || service.startInput.Action.Reason() != "manual audit" {
		t.Fatalf("StartReconciliation input=%+v calls=%d", service.startInput, service.startCalls)
	}
}

func TestAdminCreditHandlersRejectUserPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		route  string
		run    gin.HandlerFunc
	}{
		{name: "grant", method: http.MethodPost, path: "/grants", route: "/grants", run: NewHandler(&adminHandlerServiceStub{}).Grant},
		{name: "wallet", method: http.MethodGet, path: "/wallets/1", route: "/wallets/:user_id", run: NewHandler(&adminHandlerServiceStub{}).GetWallet},
		{name: "ledger", method: http.MethodGet, path: "/wallets/1/ledger", route: "/wallets/:user_id/ledger", run: NewHandler(&adminHandlerServiceStub{}).ListLedger},
		{name: "start reconciliation", method: http.MethodPost, path: "/runs", route: "/runs", run: NewHandler(&adminHandlerServiceStub{}).StartReconciliation},
		{name: "list reconciliation runs", method: http.MethodGet, path: "/runs", route: "/runs", run: NewHandler(&adminHandlerServiceStub{}).ListReconciliationRuns},
		{name: "get reconciliation run", method: http.MethodGet, path: "/runs/1", route: "/runs/:run_id", run: NewHandler(&adminHandlerServiceStub{}).GetReconciliationRun},
		{name: "list global reconciliation findings", method: http.MethodGet, path: "/findings?status=open", route: "/findings", run: NewHandler(&adminHandlerServiceStub{}).ListGlobalReconciliationFindings},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				testsupport.SetPrincipal(t, c, 7, principal.RoleUser)
				c.Next()
			})
			router.Handle(tt.method, tt.route, tt.run)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`)))
			if recorder.Code != http.StatusForbidden || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminCreditReadHandlersRejectMissingPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(&adminHandlerServiceStub{})

	for _, tt := range []struct {
		name  string
		path  string
		route string
		run   gin.HandlerFunc
	}{
		{name: "wallet", path: "/wallets/1", route: "/wallets/:user_id", run: handler.GetWallet},
		{name: "ledger", path: "/wallets/1/ledger", route: "/wallets/:user_id/ledger", run: handler.ListLedger},
		{name: "list reconciliation runs", path: "/runs", route: "/runs", run: handler.ListReconciliationRuns},
		{name: "get reconciliation run", path: "/runs/1", route: "/runs/:run_id", run: handler.GetReconciliationRun},
		{name: "list global reconciliation findings", path: "/findings?status=open", route: "/findings", run: handler.ListGlobalReconciliationFindings},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET(tt.route, tt.run)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != http.StatusUnauthorized || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAdminCreditHandlersAuthenticateBeforeServiceAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name       string
		setup      func(*gin.Context)
		wantStatus int
	}{
		{name: "missing Principal", wantStatus: http.StatusUnauthorized},
		{name: "user Principal", setup: func(c *gin.Context) { testsupport.SetPrincipal(t, c, 7, principal.RoleUser) }, wantStatus: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/grants", func(c *gin.Context) {
				if tt.setup != nil {
					tt.setup(c)
				}
				NewHandler(nil).Grant(c)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/grants", strings.NewReader(`{}`)))
			if recorder.Code != tt.wantStatus || strings.Count(recorder.Body.String(), `"code"`) != 1 {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestWriteCreditErrorUsesIdeaDisplayName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	writeCreditError(context, ErrInsufficientCredits)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"msg":"Idea不足"`) {
		t.Fatalf("response=%s, want Idea display name", body)
	}
}
