package taskservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
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

func TestAdminGetTaskSerializesEmptyExecutionAttemptsAsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(ServiceDeps{
		TaskRepo: adminTaskDetailStoreStub{task: &tasks.Task{TaskID: "no-attempt-task", Status: tasks.StatusPending}},
	}, ServiceOptions{})
	router := gin.New()
	router.GET("/admin/tasks/:task_id", NewHandler(service).GetTask)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/tasks/no-attempt-task", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Data struct {
			Task struct {
				ExecutionAttempts []json.RawMessage `json:"execution_attempts"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Task.ExecutionAttempts == nil || len(body.Data.Task.ExecutionAttempts) != 0 {
		t.Fatalf("execution_attempts=%v, want []", body.Data.Task.ExecutionAttempts)
	}
}

func TestCancelAdminTaskUsesAuthenticatedAdminActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID:     "admin-handler-cancel",
		UserID:     42,
		UserPrompt: "test",
		Prompt:     "test",
		Status:     tasks.StatusRunning,
		Params:     "{}",
	}
	if err := createTaskServiceTestTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	service := NewService(ServiceDeps{TaskRepo: taskRepo, CancelStore: &fakeCancelStore{}}, ServiceOptions{})
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/admin/tasks/:task_id/cancel", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 99, principal.RoleAdmin)
		handler.CancelAdminTask(c)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/tasks/"+task.TaskID+"/cancel", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	stopped, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if stopped.CancelledBy == nil || *stopped.CancelledBy != 99 {
		t.Fatalf("cancelled_by=%v, want authenticated admin ID", stopped.CancelledBy)
	}
}

func TestSubmitTaskHandlerReturnsEnvelopeAndSubmitsCanonicalPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	submissionStore := &recordingSubmissionStore{}
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID: 101,
				Models: []accounts.AccountModel{{
					Name:    "gpt-image-1",
					Enabled: true,
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
		"client_action_id": "handler-fusion",
		"feature":          workflow.FeatureSlugFusion,
		"image_urls":       []string{"https://example.com/a.png", "https://example.com/b.png"},
		"params": map[string]interface{}{
			"model":        "gpt-image-1",
			"output_count": 4,
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
	if call.canonical.UserID() != 42 {
		t.Fatalf("canonical user id=%d, want 42", call.canonical.UserID())
	}
	typedParams, err := tasks.UnmarshalParams(call.canonical.Params())
	if err != nil {
		t.Fatalf("unmarshal queue params: %v", err)
	}
	if typedParams.Feature != workflow.FeatureSlugFusion {
		t.Fatalf("feature=%q, want fusion", typedParams.Feature)
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

	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID: 101,
				Models: []accounts.AccountModel{{
					Name:    "gpt-image-1",
					Enabled: true,
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
		"feature":          workflow.FeatureSlugGeneration,
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

	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name: "gpt-image-1", Enabled: true,
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
		"feature":          workflow.FeatureSlugGeneration,
		"params":           map[string]interface{}{"model": "gpt-image-1"},
	}))

	assertMachineErrorResponse(t, rec, http.StatusServiceUnavailable, "pricing_unavailable", "暂时无法获取报价，请重试")
}

func TestSubmitTaskHandlerHidesSubmissionStoreCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const internalCause = "river database connection refused"
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name: "gpt-image-1", Enabled: true,
			}},
		}}},
		SubmissionStore: &recordingSubmissionStore{err: errors.New(internalCause)},
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":           "生成图案",
		"client_action_id": "handler-submission-failed",
		"feature":          workflow.FeatureSlugGeneration,
	}))

	assertMachineErrorResponse(t, rec, http.StatusServiceUnavailable, "submission_failed", "任务提交失败，请稍后重试")
	if strings.Contains(rec.Body.String(), internalCause) {
		t.Fatalf("response leaked internal cause: %s", rec.Body.String())
	}
}

func TestSubmitTaskHandlerMissingSubmissionStoreReturnsSafeServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name: "gpt-image-1", Enabled: true,
			}},
		}}},
		MediaValidator: testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":           "生成图案",
		"client_action_id": "handler-missing-submission-store",
		"feature":          workflow.FeatureSlugGeneration,
	}))

	assertMachineErrorResponse(t, rec, http.StatusServiceUnavailable, "submission_failed", "任务提交失败，请稍后重试")
	if strings.Contains(rec.Body.String(), "未配置") {
		t.Fatalf("response leaked internal configuration state: %s", rec.Body.String())
	}
}

func TestCheckSubmitTaskReadinessHandlerMissingBillingReturnsStructuredSafeError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name: "gpt-image-1", Enabled: true,
			}},
		}}},
		MediaValidator: testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", map[string]interface{}{
		"prompt":           "生成图案",
		"client_action_id": "readiness-missing-billing",
		"feature":          workflow.FeatureSlugGeneration,
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Code int                         `json:"code"`
		Msg  string                      `json:"msg"`
		Data submitTaskReadinessResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Msg != "success" || body.Data.Ready ||
		body.Data.ErrorCode != "submission_failed" ||
		body.Data.Message != "任务提交检查暂不可用，请稍后重试" {
		t.Fatalf("unexpected readiness response: %+v", body)
	}
	if strings.Contains(rec.Body.String(), "未配置") {
		t.Fatalf("response leaked internal configuration state: %s", rec.Body.String())
	}
}

func TestCheckSubmitTaskReadinessHandlerReturnsFeatureEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
			{
				ID:           101,
				Capabilities: []string{"fill"},
				Models: []accounts.AccountModel{{
					Name:    "gpt-image-1",
					Enabled: true,
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
		"client_action_id": "handler-outpaint",
		"feature":          workflow.FeatureSlugOutpaint,
		"image_urls":       []string{"https://example.com/source.png"},
		"params": map[string]interface{}{
			"model":                 "gpt-image-1",
			"output_count":          1,
			"original_image_width":  1024,
			"original_image_height": 768,
			"outpaint": map[string]interface{}{
				"target_width": 1280, "target_height": 768,
				"image_offset_x": 0, "image_offset_y": 0,
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
	if !body.Data.Ready || body.Data.Feature != workflow.FeatureSlugOutpaint || body.Data.Message != "" || body.Data.Billing == nil {
		t.Fatalf("unexpected readiness data: %+v", body.Data)
	}
	if body.Data.Billing.UnitCredits != 3 || body.Data.Billing.Quantity != 1 || body.Data.Billing.TotalCredits != 3 || !body.Data.Billing.Sufficient {
		t.Fatalf("unexpected readiness quote: %+v", body.Data.Billing)
	}
}

func TestCheckSubmitTaskReadinessMatrixReturnsAllFeatureEnvelopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
			ID:     101,
			Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		}}},
		SubmissionStore: &recordingSubmissionStore{},
		MediaValidator:  testSubmitMediaValidator{},
		TaskBilling:     newHandlerTaskBillingService(),
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness/matrix", map[string]interface{}{
		"requests": []map[string]interface{}{
			{"feature": workflow.FeatureSlugOutpaint, "prompt": "matrix one"},
			{"feature": workflow.FeatureSlugCutout, "prompt": "matrix two"},
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []submitTaskReadinessResponse `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || len(body.Data.Items) != 2 {
		t.Fatalf("unexpected matrix response: %+v", body)
	}
	if body.Data.Items[0].Feature != workflow.FeatureSlugOutpaint || body.Data.Items[1].Feature != workflow.FeatureSlugCutout {
		t.Fatalf("unexpected item features: %+v", body.Data.Items)
	}
}

func TestCheckSubmitTaskReadinessHandlerHidesMissingProviderConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := newSubmitServiceForTest(submitTestServiceDeps{
		AccountRepo:     &fakeSubmitAccountStore{},
		SubmissionStore: &recordingSubmissionStore{},
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(t, service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", map[string]interface{}{
		"prompt":           "生成海报",
		"client_action_id": "handler-provider-unavailable",
		"feature":          workflow.FeatureSlugGeneration,
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Code int                         `json:"code"`
		Msg  string                      `json:"msg"`
		Data submitTaskReadinessResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Msg != "success" || body.Data.Ready {
		t.Fatalf("unexpected readiness response: %+v", body)
	}
	if body.Data.Message != providerUnavailableMessage || body.Data.ErrorCode != "provider_unavailable" {
		t.Fatalf("readiness error=%+v, want structured provider unavailable", body.Data)
	}
}

func TestCheckSubmitTaskReadinessHandlerReturnsStructuredPricingUnavailableError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, quoteErr := range []error{billing.ErrSKUNotFound, billing.ErrPricingUnavailable} {
		t.Run(quoteErr.Error(), func(t *testing.T) {
			service := newSubmitServiceForTest(submitTestServiceDeps{
				AccountRepo: &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{{
					ID: 101,
					Models: []accounts.AccountModel{{
						Name: "gpt-image-1", Enabled: true,
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
				"feature":          workflow.FeatureSlugGeneration,
				"params":           map[string]interface{}{"model": "gpt-image-1"},
			}))

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var body struct {
				Code int                         `json:"code"`
				Msg  string                      `json:"msg"`
				Data submitTaskReadinessResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != 0 || body.Msg != "success" || body.Data.Ready ||
				body.Data.ErrorCode != "pricing_unavailable" ||
				body.Data.Message != "暂时无法获取报价，请重试" ||
				body.Data.Feature != workflow.FeatureSlugGeneration {
				t.Fatalf("unexpected readiness response: %+v", body)
			}
		})
	}
}

type handlerBillingQuoter struct{ err error }

func (q handlerBillingQuoter) QuoteAtResolution(_ context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if q.err != nil {
		return billing.Quote{}, q.err
	}
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	return billing.Quote{
		PriceSnapshot: billing.PriceSnapshot{SKU: sku, DisplayName: "Test AI image", PriceVersionID: 1, PriceVersion: 1, OutputResolution: resolution, UnitCredits: 3},
		Quantity:      quantity,
		TotalCredits:  3 * quantity,
	}, nil
}

func (q handlerBillingQuoter) QuoteAtResolutionInTx(ctx context.Context, _ pgx.Tx, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	return q.QuoteAtResolution(ctx, rawSKU, resolution, quantity)
}

type handlerBillingCredits struct{}

func (handlerBillingCredits) GetWallet(context.Context, int64) (credits.WalletDetails, error) {
	return credits.WalletDetails{WalletBalance: credits.WalletBalance{Available: 100}, Status: credits.WalletStatusActive}, nil
}

func (handlerBillingCredits) ListLedger(context.Context, credits.ListLedgerInput) (credits.LedgerPage, error) {
	return credits.LedgerPage{}, nil
}

func (handlerBillingCredits) InTx(pgx.Tx) credits.Commands { return nil }

func newHandlerTaskBillingService() *taskbilling.Service {
	return taskbilling.NewService(handlerBillingQuoter{}, handlerBillingCredits{})
}

type submitTestServiceDeps struct {
	AccountRepo     AccountStore
	SubmissionStore TaskSubmissionStore
	MediaValidator  taskmedia.Validator
	MediaPreparer   taskmedia.GenerationMediaPreparer
	PromptStore     FeaturePromptStore
	TaskBilling     *taskbilling.Service
}

func newSubmitServiceForTest(deps submitTestServiceDeps, opts ServiceOptions) *Service {
	promptStore := deps.PromptStore
	if promptStore == nil {
		configs := make(map[string]prompts.FeaturePromptRuntimeConfig)
		for _, slug := range workflow.SupportedFeatureSlugs() {
			configs[slug] = prompts.FeaturePromptRuntimeConfig{
				Prompt:                "{{user_prompt}}",
				UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug),
			}
		}
		promptStore = fakeFeaturePromptStore{configs: configs}
	}
	mediaPreparer := deps.MediaPreparer
	if mediaPreparer == nil {
		mediaPreparer = &recordingGenerationMediaPreparer{}
	}
	return NewService(ServiceDeps{
		TaskIntake: NewTaskIntake(TaskIntakeDeps{
			Accounts:      deps.AccountRepo,
			Prompts:       promptStore,
			Media:         deps.MediaValidator,
			MediaPreparer: mediaPreparer,
		}),
		SubmissionStore: deps.SubmissionStore,
		TaskBilling:     deps.TaskBilling,
	}, opts)
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

func TestAdminTaskMonitorReadModelsExposeImmutableExecutionSnapshot(t *testing.T) {
	task := &tasks.Task{
		TaskID: "terminal-task",
	}
	snapshot := tasks.AdminTaskExecutionSnapshot{AccountID: 11, ProviderEndpointID: 12, ProviderEndpointName: "Immutable Endpoint", ResolvedModel: "immutable-model", APIType: "google-native"}

	listDTO := adminTaskMonitorListItemFromTask(task, snapshot, true)
	if listDTO.LatestExecution == nil || listDTO.LatestExecution.AccountID != 11 || listDTO.LatestExecution.ProviderEndpointID != 12 {
		t.Fatalf("list latest execution=%+v, want immutable owner snapshot", listDTO.LatestExecution)
	}

	detailDTO := adminTaskMonitorDetailFromTask(task, tasks.AdminBillingSnapshot{}, false, []tasks.AdminTaskAttempt{{
		AccountID: snapshot.AccountID, ProviderEndpointID: snapshot.ProviderEndpointID, ProviderEndpointName: snapshot.ProviderEndpointName,
		ResolvedModel: snapshot.ResolvedModel, APIType: snapshot.APIType,
	}})
	if detailDTO.LatestExecution == nil || detailDTO.LatestExecution.ProviderEndpointName != "Immutable Endpoint" {
		t.Fatalf("detail latest execution=%+v, want explicit header snapshot", detailDTO.LatestExecution)
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
	router.POST("/submit/readiness/matrix", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, userID, principal.RoleUser)
		handler.CheckSubmitTaskReadinessMatrix(c)
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
