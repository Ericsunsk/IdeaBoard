package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRequestIDTestContext(requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	if requestID != "" {
		c.Set(RequestIDHeader, requestID)
	}
	return c, recorder
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("envelope is not JSON: %v; body=%s", err, body)
	}
	return payload
}

func TestSuccessEchoesRequestID(t *testing.T) {
	c, recorder := newRequestIDTestContext("req-123")
	Success(c, gin.H{"ok": true})
	payload := decodeEnvelope(t, recorder.Body.Bytes())
	if payload["request_id"] != "req-123" {
		t.Fatalf("request_id = %v, want req-123; body=%s", payload["request_id"], recorder.Body.String())
	}
	if payload["code"] != float64(0) {
		t.Fatalf("code = %v, want 0", payload["code"])
	}
}

func TestErrorOmitsEmptyRequestID(t *testing.T) {
	c, recorder := newRequestIDTestContext("")
	Error(c, http.StatusBadRequest, "参数错误")
	payload := decodeEnvelope(t, recorder.Body.Bytes())
	if _, ok := payload["request_id"]; ok {
		t.Fatalf("request_id should be omitted when unset; body=%s", recorder.Body.String())
	}
	if payload["code"] != float64(http.StatusBadRequest) || payload["message_safe"] != true {
		t.Fatalf("envelope = %v, want status code with message_safe", payload)
	}
}

func TestMachineErrorEchoesRequestID(t *testing.T) {
	c, recorder := newRequestIDTestContext("req-456")
	MachineError(c, http.StatusConflict, "reward_code_redeemed", "已领取过该奖励")
	payload := decodeEnvelope(t, recorder.Body.Bytes())
	if payload["request_id"] != "req-456" || payload["code"] != "reward_code_redeemed" {
		t.Fatalf("envelope = %v, want machine code with request_id", payload)
	}
}
