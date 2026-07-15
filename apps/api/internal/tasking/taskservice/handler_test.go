package taskservice

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testsupport"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func TestAdminListTasksRejectsInvalidIDFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		query   string
		message string
	}{
		{name: "user id zero", query: "user_id=0", message: "user_id 必须大于 0"},
		{name: "account id negative", query: "account_id=-1", message: "account_id 必须大于 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/admin/tasks", NewHandler(nil).ListTasks)

			req := httptest.NewRequest(http.MethodGet, "/admin/tasks?"+tt.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.message) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tt.message)
			}
		})
	}
}

func TestSubmitTaskHandlerReturnsEnvelopeAndSubmitsCanonicalPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	submissionStore := &recordingSubmissionStore{}
	service := NewService(ServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID: 101,
				Models: []accounts.AccountModel{{
					Name:         "gpt-image-1",
					Enabled:      true,
					FeatureSlugs: []string{workflow.FeatureSlugStyleFusion},
				}},
			},
		}},
		SubmissionStore: submissionStore,
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":           "融合但保留主体",
		"client_action_id": "handler-style-fusion",
		"feature":          workflow.FeatureSlugStyleFusion,
		"image_urls":       []string{"https://example.com/a.png", "https://example.com/b.png"},
		"params": map[string]interface{}{
			"model": "gpt-image-1",
			"fusion": map[string]interface{}{
				"input_weights": []float64{3, 1},
			},
		},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Code int                `json:"code"`
		Msg  string             `json:"msg"`
		Data submitTaskResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Msg != "success" || body.Data.TaskID == "" {
		t.Fatalf("unexpected response body: %+v", body)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.task.TaskID != body.Data.TaskID || call.queueTask.TaskID != body.Data.TaskID {
		t.Fatalf("task ids do not match response: task=%q queue=%q response=%q", call.task.TaskID, call.queueTask.TaskID, body.Data.TaskID)
	}
	if call.task.EffectiveCorrelationID() != body.Data.TaskID || call.queueTask.EffectiveCorrelationID() != body.Data.TaskID {
		t.Fatalf("correlation ids task=%q queue=%q, want response task_id %q", call.task.EffectiveCorrelationID(), call.queueTask.EffectiveCorrelationID(), body.Data.TaskID)
	}
	if call.task.UserID != 42 || call.queueTask.UserID != 42 {
		t.Fatalf("user ids task=%d queue=%d, want 42", call.task.UserID, call.queueTask.UserID)
	}
	if call.task.Params != call.queueTask.Params {
		t.Fatalf("task params and queue params differ: task=%s queue=%s", call.task.Params, call.queueTask.Params)
	}
	typedParams, err := tasks.UnmarshalParams(call.queueTask.Params)
	if err != nil {
		t.Fatalf("unmarshal queue params: %v", err)
	}
	if typedParams.Feature != workflow.FeatureSlugStyleFusion {
		t.Fatalf("feature=%q, want style-fusion", typedParams.Feature)
	}
	if typedParams.Media == nil || len(typedParams.Media.Inputs) != 2 {
		t.Fatalf("media inputs=%#v, want two inputs", typedParams.Media)
	}
	if math.Abs(typedParams.Media.Inputs[0].Weight-0.75) > 0.000001 ||
		math.Abs(typedParams.Media.Inputs[1].Weight-0.25) > 0.000001 {
		t.Fatalf("media input weights=%#v, want 0.75/0.25", typedParams.Media.Inputs)
	}
	if typedParams.Fusion == nil || !almostEqualSlice(typedParams.Fusion.InputWeights, []float64{0.75, 0.25}) {
		t.Fatalf("fusion=%#v, want normalized weights", typedParams.Fusion)
	}
}

func TestSubmitTaskHandlerReturnsStructuredInsufficientCreditsError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := NewService(ServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID: 101,
				Models: []accounts.AccountModel{{
					Name:         "gpt-image-1",
					Enabled:      true,
					FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
				}},
			},
		}},
		SubmissionStore: &recordingSubmissionStore{err: taskbilling.ErrInsufficientCredits},
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":           "生成海报",
		"client_action_id": "handler-insufficient",
		"feature":          workflow.FeatureSlugPatternGen,
		"params": map[string]interface{}{
			"model": "gpt-image-1",
		},
	}))

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusPaymentRequired, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "insufficient_credits" || body.Msg != "Idea不足" {
		t.Fatalf("unexpected insufficient response: %+v", body)
	}
}

func TestSubmitTaskHandlerReturnsStructuredPricingUnavailableError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := NewService(ServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name: "gpt-image-1", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		}}},
		SubmissionStore: &recordingSubmissionStore{err: taskbilling.ErrPricingUnavailable},
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":           "生成海报",
		"client_action_id": "handler-pricing-unavailable",
		"feature":          workflow.FeatureSlugPatternGen,
		"params":           map[string]interface{}{"model": "gpt-image-1"},
	}))

	assertMachineErrorResponse(t, rec, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
}

func TestCheckSubmitTaskReadinessHandlerReturnsFeatureEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := NewService(ServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID: 101,
				Models: []accounts.AccountModel{{
					Name:         "gpt-image-1",
					Enabled:      true,
					FeatureSlugs: []string{workflow.FeatureSlugImageExtend},
				}},
			},
		}},
		SubmissionStore: &recordingSubmissionStore{},
		MediaValidator:  testSubmitMediaValidator{},
		TaskBilling:     newHandlerTaskBillingService(),
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", map[string]interface{}{
		"prompt":           "向右扩展为工作室背景",
		"client_action_id": "handler-image-extend",
		"billing_quantity": 4,
		"feature":          workflow.FeatureSlugImageExtend,
		"image_urls":       []string{"https://example.com/source.png"},
		"params": map[string]interface{}{
			"model": "gpt-image-1",
			"extend": map[string]interface{}{
				"top_percent":    0,
				"right_percent":  25,
				"bottom_percent": 0,
				"left_percent":   0,
			},
		},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Code int                         `json:"code"`
		Msg  string                      `json:"msg"`
		Data submitTaskReadinessResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Msg != "success" {
		t.Fatalf("unexpected response envelope: %+v", body)
	}
	if !body.Data.Ready || body.Data.Feature != workflow.FeatureSlugImageExtend || body.Data.Message != "" || body.Data.Billing == nil {
		t.Fatalf("unexpected readiness data: %+v", body.Data)
	}
	if body.Data.Billing.UnitCredits != 3 || body.Data.Billing.Quantity != 4 || body.Data.Billing.TotalCredits != 12 || !body.Data.Billing.Sufficient {
		t.Fatalf("unexpected readiness quote: %+v", body.Data.Billing)
	}
}

func TestCheckSubmitTaskReadinessHandlerRequiresValidBillingQuantity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := NewService(ServiceDeps{}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)
	for _, billingQuantity := range []interface{}{nil, 0, 17} {
		t.Run(fmt.Sprint(billingQuantity), func(t *testing.T) {
			body := map[string]interface{}{
				"prompt":           "生成海报",
				"client_action_id": "handler-invalid-quantity",
				"feature":          workflow.FeatureSlugPatternGen,
			}
			if billingQuantity != nil {
				body["billing_quantity"] = billingQuantity
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestCheckSubmitTaskReadinessHandlerReturnsStructuredPricingUnavailableError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, quoteErr := range []error{billing.ErrSKUNotFound, billing.ErrPricingUnavailable} {
		t.Run(quoteErr.Error(), func(t *testing.T) {
			service := NewService(ServiceDeps{
				AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
					ID: 101,
					Models: []accounts.AccountModel{{
						Name: "gpt-image-1", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
					}},
				}}},
				SubmissionStore: &recordingSubmissionStore{},
				MediaValidator:  testSubmitMediaValidator{},
				TaskBilling: taskbilling.NewService(
					handlerBillingQuoter{err: quoteErr},
					handlerBillingCredits{},
				),
			}, ServiceOptions{})
			router := newSubmitHandlerTestRouter(t, service, 42)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", map[string]interface{}{
				"prompt":           "生成海报",
				"client_action_id": "handler-pricing-unavailable",
				"billing_quantity": 1,
				"feature":          workflow.FeatureSlugPatternGen,
				"params":           map[string]interface{}{"model": "gpt-image-1"},
			}))

			assertMachineErrorResponse(t, rec, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
		})
	}
}

type handlerBillingQuoter struct{ err error }

func (q handlerBillingQuoter) Quote(_ context.Context, rawSKU string, quantity int64) (billing.Quote, error) {
	if q.err != nil {
		return billing.Quote{}, q.err
	}
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	return billing.Quote{
		PriceSnapshot: billing.PriceSnapshot{SKU: sku, DisplayName: "Test AI image", PriceVersionID: 1, PriceVersion: 1, UnitCredits: 3},
		Quantity:      quantity,
		TotalCredits:  3 * quantity,
	}, nil
}

func (q handlerBillingQuoter) QuoteInTx(ctx context.Context, _ pgx.Tx, rawSKU string, quantity int64) (billing.Quote, error) {
	return q.Quote(ctx, rawSKU, quantity)
}

type handlerBillingCredits struct{}

func (handlerBillingCredits) GetWallet(context.Context, int64) (credits.WalletDetails, error) {
	return credits.WalletDetails{WalletBalance: credits.WalletBalance{Available: 100}, Status: credits.WalletStatusActive}, nil
}

func (handlerBillingCredits) ListLedger(context.Context, credits.ListLedgerInput) (credits.LedgerPage, error) {
	return credits.LedgerPage{}, nil
}

func (handlerBillingCredits) InTx(pgx.Tx) credits.Commands     { return nil }
func (handlerBillingCredits) InSQLTx(*sql.Tx) credits.Commands { return nil }

func newHandlerTaskBillingService() *taskbilling.Service {
	return taskbilling.NewService(handlerBillingQuoter{}, handlerBillingCredits{})
}

func assertMachineErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, code, msg string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d, want %d, body=%s", recorder.Code, status, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != code || body.Msg != msg {
		t.Fatalf("machine error=%+v, want code=%q msg=%q", body, code, msg)
	}
}

func newSubmitHandlerTestRouter(t testing.TB, service *Service, userID int64) *gin.Engine {
	t.Helper()
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/submit", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		handler.SubmitTask(c)
	})
	router.POST("/submit/readiness", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		handler.CheckSubmitTaskReadiness(c)
	})
	return router
}

func newJSONRequest(t *testing.T, method string, target string, body interface{}) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func almostEqualSlice(got []float64, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if math.Abs(got[index]-want[index]) > 0.000001 {
			return false
		}
	}
	return true
}
