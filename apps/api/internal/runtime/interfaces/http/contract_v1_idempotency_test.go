package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

func TestRequireV1IdempotencyKey(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantKey    string
		wantOK     bool
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusBadRequest},
		{name: "blank", header: "  \t", wantStatus: http.StatusBadRequest},
		{name: "overlong", header: strings.Repeat("k", contract.IdempotencyMaxKeyLength+1), wantStatus: http.StatusBadRequest},
		{name: "trimmed valid key", header: "  board-create-1  ", wantKey: "board-create-1", wantOK: true, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			if tt.header != "" {
				c.Request.Header.Set(contract.IdempotencyKeyHeader, tt.header)
			}

			gotKey, gotOK := requireV1IdempotencyKey(c)
			if gotKey != tt.wantKey || gotOK != tt.wantOK {
				t.Fatalf("requireV1IdempotencyKey() = (%q, %t), want (%q, %t)", gotKey, gotOK, tt.wantKey, tt.wantOK)
			}
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if !tt.wantOK {
				var payload contract.ErrorEnvelope
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode error envelope: %v; body=%s", err, recorder.Body.String())
				}
				if payload.Error.Code != string(contract.CodeIdempotencyKeyRequired) {
					t.Fatalf("error code = %q, want %q", payload.Error.Code, contract.CodeIdempotencyKeyRequired)
				}
				if !c.IsAborted() {
					t.Fatal("invalid key must abort the request")
				}
			}
		})
	}
}
