package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/errorrender"
	rewardsdomain "ideaboard/api/internal/rewards/domain"

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

func TestRewardDomainTableHasNoEmptyMessages(t *testing.T) {
	// Programmer-error guard: every row carries its frozen safe message,
	// so the RenderMapped empty-message fallback stays unreachable and
	// raw wrapped errors (code plaintext, digests) can never reach the
	// wire. Scopes are limited to the two known surfaces.
	for i, row := range rewardDomainErrorTable {
		if len(row.Targets) == 0 {
			t.Errorf("row %d (%s) has no targets", i, row.Code)
		}
		if row.Code == "" || row.Message == "" {
			t.Errorf("row %d targets=%v has empty code/message", i, row.Targets)
		}
		// Status is never stored per row: it derives from the central
		// registry by Code, so every row must resolve to a 4xx/5xx
		// outcome there (unknown codes fail closed to 500).
		if status := errorrender.StatusForCode(errorrender.Code(row.Code)); status < 400 || status >= 600 {
			t.Errorf("row %d (%s) registry status = %d, want 4xx/5xx", i, row.Code, status)
		}
		switch row.Scope {
		case "", rewardScopeAdmin:
		default:
			t.Errorf("row %d (%s) scope = %q, want admin/empty", i, row.Code, row.Scope)
		}
	}
}

func TestRewardAdminScopeSharesSingleInvalidOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		scope      string
		err        error
		wantStatus int
		wantCode   string
	}{
		{scope: rewardScopeAdmin, err: rewardsdomain.ErrInvalidCode, wantStatus: http.StatusBadRequest, wantCode: codeRewardCodeInvalid},
		{scope: rewardScopeAdmin, err: rewardsdomain.ErrRewardCodeConflict, wantStatus: http.StatusConflict, wantCode: codeRewardCodeIdempotencyConflict},
	}
	for _, tc := range cases {
		router := gin.New()
		router.GET("/err", func(c *gin.Context) { writeRewardError(c, tc.err) })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
		if recorder.Code != tc.wantStatus {
			t.Fatalf("scope %s err %v: status = %d, want %d body=%s", tc.scope, tc.err, recorder.Code, tc.wantStatus, recorder.Body.String())
		}
		var body struct {
			Code string `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != tc.wantCode {
			t.Fatalf("scope %s err %v: code = %q, want %q", tc.scope, tc.err, body.Code, tc.wantCode)
		}
		if strings.Contains(recorder.Body.String(), "SECRET") {
			t.Fatalf("scope %s response leaked detail: %s", tc.scope, recorder.Body.String())
		}
	}
}
