package taskservice

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"

	"github.com/gin-gonic/gin"
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
	router := newSubmitHandlerTestRouter(service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit", map[string]interface{}{
		"prompt":     "融合但保留主体",
		"feature":    workflow.FeatureSlugStyleFusion,
		"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
		"params": map[string]interface{}{
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
	}, ServiceOptions{})
	router := newSubmitHandlerTestRouter(service, 42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newJSONRequest(t, http.MethodPost, "/submit/readiness", map[string]interface{}{
		"prompt":     "向右扩展为工作室背景",
		"feature":    workflow.FeatureSlugImageExtend,
		"image_urls": []string{"https://example.com/source.png"},
		"params": map[string]interface{}{
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
	if !body.Data.Ready || body.Data.Feature != workflow.FeatureSlugImageExtend || body.Data.Message != "" {
		t.Fatalf("unexpected readiness data: %+v", body.Data)
	}
}

func newSubmitHandlerTestRouter(service *Service, userID int64) *gin.Engine {
	handler := NewHandler(service)
	router := gin.New()
	router.POST("/submit", func(c *gin.Context) {
		c.Set("user_id", userID)
		handler.SubmitTask(c)
	})
	router.POST("/submit/readiness", func(c *gin.Context) {
		c.Set("user_id", userID)
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
