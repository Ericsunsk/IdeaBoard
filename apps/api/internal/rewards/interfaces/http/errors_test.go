package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRedactedRewardErrorSummaryKeepsAttributionWithoutSensitiveMaterial pins
// the #783 contract: an unmatched 5xx error stays attributable in logs
// (type + redacted summary) while Code plaintext, digests, and request
// payloads never reach the log line. The wire envelope is asserted by the
// handler security tests; this test pins the log-side half.
func TestRedactedRewardErrorSummaryKeepsAttributionWithoutSensitiveMaterial(t *testing.T) {
	const rawCode = "SECRET294"
	const fullDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	requestBody := `{"code":"` + rawCode + `"}`
	err := errors.New("lookup failed code=" + rawCode + " digest=" + fullDigest + " body=" + requestBody)

	summary := redactedRewardErrorSummary(err)
	if summary == "" {
		t.Fatal("redacted summary is empty; 5xx would be unattributable")
	}
	if !strings.Contains(summary, "lookup failed") {
		t.Fatalf("summary lost the attributable prefix: %q", summary)
	}
	for _, forbidden := range []string{rawCode, fullDigest, requestBody} {
		if strings.Contains(summary, forbidden) {
			t.Fatalf("summary exposed sensitive value %q: %q", forbidden, summary)
		}
	}
	if strings.Contains(summary, "\n") {
		t.Fatalf("summary is not single-line: %q", summary)
	}
}

func TestRedactedRewardErrorSummaryBoundsLengthAndNil(t *testing.T) {
	if got := redactedRewardErrorSummary(nil); got != "" {
		t.Fatalf("nil error summary = %q, want empty", got)
	}
	err := errors.New(strings.Repeat("x", maxRewardErrorSummaryRunes+100))
	if got := redactedRewardErrorSummary(err); len([]rune(got)) != maxRewardErrorSummaryRunes {
		t.Fatalf("long summary length = %d runes, want %d", len([]rune(got)), maxRewardErrorSummaryRunes)
	}
}

func TestRewardErrorRenderFallsBackToFixedSafeMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "SECRET294"
	mapping := rewardErrorMapping{
		targets: []error{errors.New("sentinel")},
		status:  http.StatusBadRequest,
		code:    "test_empty_message",
	}
	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		mapping.render(c, errors.Join(errors.New("sentinel"), errors.New("raw code="+secret)))
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "test_empty_message" || body.Msg != "操作失败，请稍后重试" {
		t.Fatalf("response = %+v, want fixed fallback", body)
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("response leaked wrapped detail: %s", recorder.Body.String())
	}
}
