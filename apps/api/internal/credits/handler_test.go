package credits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type adminCommandServiceStub struct{}

func (*adminCommandServiceStub) Grant(context.Context, GrantCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (*adminCommandServiceStub) Deduct(context.Context, DeductionCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (*adminCommandServiceStub) Correct(context.Context, CorrectionCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

func (*adminCommandServiceStub) Refund(context.Context, RefundCommand) (CommandResult, error) {
	return CommandResult{}, nil
}

type userWalletServiceStub struct{}

func (*userWalletServiceStub) GetWallet(context.Context, int64) (WalletDetails, error) {
	return WalletDetails{}, nil
}

type adminReadServiceStub struct{}

func (*adminReadServiceStub) GetWallet(context.Context, int64) (WalletDetails, error) {
	return WalletDetails{}, nil
}

func (*adminReadServiceStub) ListLedger(context.Context, ListLedgerInput) (LedgerPage, error) {
	return LedgerPage{}, nil
}

type reconciliationServiceStub struct {
	startInput StartManualReconciliationInput
	startCalls int
}

func (s *reconciliationServiceStub) GetCreditHealth(context.Context) (CreditHealth, error) {
	return CreditHealth{Status: CreditHealthHealthy}, nil
}

func (s *reconciliationServiceStub) StartReconciliation(_ context.Context, input StartManualReconciliationInput) (ReconciliationRun, error) {
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

func (s *reconciliationServiceStub) GetReconciliationRun(context.Context, int64) (ReconciliationRun, error) {
	return ReconciliationRun{}, nil
}

func (s *reconciliationServiceStub) ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error) {
	return ReconciliationRunPage{}, nil
}

func (s *reconciliationServiceStub) ListGlobalReconciliationFindings(context.Context, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error) {
	return ReconciliationFindingPage{}, nil
}

type handlerTestUserResolver struct{}

func (handlerTestUserResolver) ResolvePublicID(context.Context, string) (UserIdentity, error) {
	return UserIdentity{ID: 94, PublicID: uuid.MustParse("00000000-0000-4000-8000-000000000094"), DisplayName: "User 94"}, nil
}

func (handlerTestUserResolver) ResolveID(context.Context, int64) (UserIdentity, error) {
	return UserIdentity{ID: 94, PublicID: uuid.MustParse("00000000-0000-4000-8000-000000000094"), DisplayName: "User 94"}, nil
}

func TestCreditRoleAdaptersServeTheirPublicHTTPSeams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := handlerTestUserResolver{}
	userHandler := NewUserHandler(&userWalletServiceStub{}, resolver)
	adminCommandHandler := NewAdminCommandHandler(&adminCommandServiceStub{}, resolver)
	adminReadHandler := NewAdminReadHandler(&adminReadServiceStub{}, resolver)
	reconciliationHandler := NewReconciliationHandler(&reconciliationServiceStub{}, resolver)

	userRouter := gin.New()
	userRouter.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 94, principal.RoleUser)
		c.Next()
	})
	userRouter.GET("/credits/wallet", userHandler.GetCurrentWallet)
	userResponse := httptest.NewRecorder()
	userRouter.ServeHTTP(userResponse, httptest.NewRequest(http.MethodGet, "/credits/wallet", nil))
	if userResponse.Code != http.StatusOK {
		t.Fatalf("user wallet status = %d, body=%s", userResponse.Code, userResponse.Body.String())
	}

	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 7, principal.RoleAdmin)
		c.Next()
	})
	adminRouter.POST("/credits/grants", adminCommandHandler.Grant)
	adminRouter.GET("/credits/wallets/:public_id", adminReadHandler.GetWallet)
	adminRouter.GET("/credits/health", reconciliationHandler.GetCreditHealth)

	grantResponse := httptest.NewRecorder()
	grantRequest := httptest.NewRequest(http.MethodPost, "/credits/grants", strings.NewReader(`{"user_public_id":"00000000-0000-4000-8000-000000000094","amount_credits":1,"idempotency_key":"handler-role-test","reason":"role adapter test","business_reference_id":"handler-role-test"}`))
	grantRequest.Header.Set("Content-Type", "application/json")
	adminRouter.ServeHTTP(grantResponse, grantRequest)
	if grantResponse.Code != http.StatusOK {
		t.Fatalf("admin command status = %d, body=%s", grantResponse.Code, grantResponse.Body.String())
	}

	readResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(readResponse, httptest.NewRequest(http.MethodGet, "/credits/wallets/00000000-0000-4000-8000-000000000094", nil))
	if readResponse.Code != http.StatusOK {
		t.Fatalf("admin read status = %d, body=%s", readResponse.Code, readResponse.Body.String())
	}

	reconciliationResponse := httptest.NewRecorder()
	adminRouter.ServeHTTP(reconciliationResponse, httptest.NewRequest(http.MethodGet, "/credits/health", nil))
	if reconciliationResponse.Code != http.StatusOK {
		t.Fatalf("reconciliation status = %d, body=%s", reconciliationResponse.Code, reconciliationResponse.Body.String())
	}
}

func TestStartReconciliationHandlerRequiresAndTrimsReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &reconciliationServiceStub{}
	handler := NewReconciliationHandler(service, nil)
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
		{name: "grant", method: http.MethodPost, path: "/grants", route: "/grants", run: NewAdminCommandHandler(&adminCommandServiceStub{}, nil).Grant},
		{name: "wallet", method: http.MethodGet, path: "/wallets/00000000-0000-4000-8000-000000000001", route: "/wallets/:public_id", run: NewAdminReadHandler(&adminReadServiceStub{}, nil).GetWallet},
		{name: "ledger", method: http.MethodGet, path: "/wallets/00000000-0000-4000-8000-000000000001/ledger", route: "/wallets/:public_id/ledger", run: NewAdminReadHandler(&adminReadServiceStub{}, nil).ListLedger},
		{name: "start reconciliation", method: http.MethodPost, path: "/runs", route: "/runs", run: NewReconciliationHandler(&reconciliationServiceStub{}, nil).StartReconciliation},
		{name: "list reconciliation runs", method: http.MethodGet, path: "/runs", route: "/runs", run: NewReconciliationHandler(&reconciliationServiceStub{}, nil).ListReconciliationRuns},
		{name: "get reconciliation run", method: http.MethodGet, path: "/runs/1", route: "/runs/:run_id", run: NewReconciliationHandler(&reconciliationServiceStub{}, nil).GetReconciliationRun},
		{name: "list global reconciliation findings", method: http.MethodGet, path: "/findings?status=open", route: "/findings", run: NewReconciliationHandler(&reconciliationServiceStub{}, nil).ListGlobalReconciliationFindings},
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
	adminReadHandler := NewAdminReadHandler(&adminReadServiceStub{}, nil)
	reconciliationHandler := NewReconciliationHandler(&reconciliationServiceStub{}, nil)

	for _, tt := range []struct {
		name  string
		path  string
		route string
		run   gin.HandlerFunc
	}{
		{name: "wallet", path: "/wallets/00000000-0000-4000-8000-000000000001", route: "/wallets/:public_id", run: adminReadHandler.GetWallet},
		{name: "ledger", path: "/wallets/00000000-0000-4000-8000-000000000001/ledger", route: "/wallets/:public_id/ledger", run: adminReadHandler.ListLedger},
		{name: "list reconciliation runs", path: "/runs", route: "/runs", run: reconciliationHandler.ListReconciliationRuns},
		{name: "get reconciliation run", path: "/runs/1", route: "/runs/:run_id", run: reconciliationHandler.GetReconciliationRun},
		{name: "list global reconciliation findings", path: "/findings?status=open", route: "/findings", run: reconciliationHandler.ListGlobalReconciliationFindings},
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
				NewAdminCommandHandler(nil, nil).Grant(c)
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

func TestCreditActorJSONOmitsInternalIdentity(t *testing.T) {
	payload, err := json.Marshal(Actor{Type: ActorAdmin, ID: "7"})
	if err != nil {
		t.Fatalf("marshal actor: %v", err)
	}
	if got := string(payload); got != `{"type":"admin"}` {
		t.Fatalf("public actor = %s, want type only", got)
	}
}
