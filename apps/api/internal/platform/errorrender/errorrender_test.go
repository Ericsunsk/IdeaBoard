package errorrender

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type stubRow struct {
	targets []error
	status  int
	code    string
	message string
}

func (r stubRow) Matches(err error) bool { return TargetsMatch(r.targets, err) }

func serveRender(t *testing.T, fn func(c *gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", fn)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	return recorder
}

func TestTargetsMatchFirstWin(t *testing.T) {
	sentinelA := errors.New("sentinel a")
	sentinelB := errors.New("sentinel b")
	table := []stubRow{
		{targets: []error{sentinelA}, status: http.StatusBadRequest, code: "a", message: "a"},
		{targets: []error{sentinelB}, status: http.StatusConflict, code: "b", message: "b"},
	}
	joined := errors.Join(sentinelB, errors.New("provider detail"))
	row, ok := FirstMatch(table, joined)
	if !ok || row.code != "b" {
		t.Fatalf("FirstMatch = %+v, %v, want code b", row, ok)
	}
	row, ok = FirstMatch(table, errors.Join(sentinelB, sentinelA))
	if !ok || row.code != "a" {
		t.Fatalf("FirstMatch = %+v, %v, want first row a even when joined error order differs", row, ok)
	}
	if _, ok := FirstMatch(table, errors.New("boom")); ok {
		t.Fatal("FirstMatch matched unknown error")
	}
	if !TargetsMatch([]error{sentinelA}, errors.Join(errors.New("wrap"), sentinelA)) {
		t.Fatal("TargetsMatch missed wrapped sentinel")
	}
	if TargetsMatch([]error{sentinelA}, errors.New("boom")) {
		t.Fatal("TargetsMatch matched unknown error")
	}
	if !SingleMatch(sentinelA, errors.Join(errors.New("wrap"), sentinelA)) {
		t.Fatal("SingleMatch missed wrapped sentinel")
	}
}

func TestRenderKeepsMachineEnvelope(t *testing.T) {
	recorder := serveRender(t, func(c *gin.Context) {
		Render(c, http.StatusConflict, "test_conflict", "冲突，请重试")
	})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Safe bool   `json:"message_safe"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "test_conflict" || body.Msg != "冲突，请重试" || !body.Safe {
		t.Fatalf("body = %+v, want machine envelope", body)
	}
}

func TestRenderDefaultKeepsMachineEnvelope(t *testing.T) {
	recorder := serveRender(t, func(c *gin.Context) {
		RenderDefault(c, "test operation failed", http.StatusInternalServerError, "test_failed", "操作失败", errors.New("boom"))
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "test_failed" || body.Msg != "操作失败" {
		t.Fatalf("body = %+v, want default envelope", body)
	}
}

func TestRenderNumericKeepsNumericEnvelope(t *testing.T) {
	recorder := serveRender(t, func(c *gin.Context) {
		RenderNumeric(c, http.StatusInternalServerError, "保存失败")
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	var body struct {
		Code float64 `json:"code"`
		Msg  string  `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != float64(http.StatusInternalServerError) || body.Msg != "保存失败" {
		t.Fatalf("body = %+v, want numeric envelope", body)
	}
}

func TestRenderDefaultCorrelatesErrorLogWithResponse(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	recorder := serveRender(t, func(c *gin.Context) {
		c.Set(response.RequestIDHeader, "req-error-1")
		RenderDefault(c, "operation failed", 500, "test_failed", "操作失败", errors.New("internal detail"), "error_type", "test")
	})
	var entry, body map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != "req-error-1" || entry["request_id"] != body["request_id"] || entry["method"] != "GET" || entry["path"] != "/err" {
		t.Fatalf("error log lost request correlation: %v, response: %v", entry, body)
	}
	if entry["error_type"] != "test" || entry["err"] != "internal detail" || body["msg"] != "操作失败" || body["message_safe"] != true {
		t.Fatalf("error details must stay in the log: %v, response: %v", entry, body)
	}
}
