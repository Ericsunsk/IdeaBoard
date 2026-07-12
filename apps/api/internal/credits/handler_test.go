package credits

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type adminHandlerServiceStub struct {
	startInput StartReconciliationInput
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

func (s *adminHandlerServiceStub) StartReconciliation(_ context.Context, input StartReconciliationInput) (ReconciliationRun, error) {
	s.startInput = input
	s.startCalls++
	return ReconciliationRun{ID: 1, Status: ReconciliationRunQueued, RequestedBy: input.AdminID, Reason: input.Reason}, nil
}

func (s *adminHandlerServiceStub) GetReconciliationRun(context.Context, int64) (ReconciliationRun, error) {
	return ReconciliationRun{}, nil
}

func (s *adminHandlerServiceStub) ListReconciliationRuns(context.Context, ListReconciliationInput) (ReconciliationRunPage, error) {
	return ReconciliationRunPage{}, nil
}

func (s *adminHandlerServiceStub) ListReconciliationFindings(context.Context, int64, ReconciliationFindingStatus, ListReconciliationInput) (ReconciliationFindingPage, error) {
	return ReconciliationFindingPage{}, nil
}

func TestStartReconciliationHandlerRequiresAndTrimsReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &adminHandlerServiceStub{}
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(7))
		c.Next()
	})
	router.POST("/runs", handler.StartReconciliation)

	for _, body := range []string{
		`{}`,
		`{"reason":"  "}`,
		`{"reason":"` + strings.Repeat("理", MaxReconciliationReasonLength+1) + `"}`,
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
	if service.startCalls != 1 || service.startInput.AdminID != 7 || service.startInput.Reason != "manual audit" || service.startInput.StaleHoldAfter != 24*time.Hour {
		t.Fatalf("StartReconciliation input=%+v calls=%d", service.startInput, service.startCalls)
	}
}
