package errorrender

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMappingMatchesAndScopeFilter(t *testing.T) {
	sentinelAdmin := errors.New("admin failure")
	sentinelShared := errors.New("shared failure")
	table := []Mapping{
		{Targets: []error{sentinelAdmin}, Code: "conflict", Message: "conflict", Scope: "admin"},
		{Targets: []error{sentinelShared}, Code: "bad_request", Message: "bad"},
	}
	if !table[0].Matches(sentinelAdmin) || table[0].Matches(sentinelShared) {
		t.Fatal("Matches must use errors.Is membership")
	}
	if !table[0].InScope("admin") || !table[0].InScope("") {
		t.Fatal("scoped row must match its scope and the empty render scope")
	}
	if table[0].InScope("redeem") {
		t.Fatal("admin row must not participate in redeem scope")
	}
	if !table[1].InScope("redeem") {
		t.Fatal("unscoped row must participate in every scope")
	}
}

func renderMapped(t *testing.T, err error, table []Mapping, opts MappedOptions) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var status int
	var body map[string]any
	router.GET("/err", func(c *gin.Context) {
		RenderMapped(c, err, table, opts)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	status = recorder.Code
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	return status, body
}

func TestRenderMappedFirstMatchWins(t *testing.T) {
	sentinel := errors.New("boom")
	table := []Mapping{
		{Targets: []error{sentinel}, Code: "conflict", Message: "first"},
		{Targets: []error{sentinel}, Code: "bad_request", Message: "second"},
	}
	status, body := renderMapped(t, sentinel, table, MappedOptions{
		DefaultStatus: http.StatusInternalServerError, DefaultCode: "dflt", DefaultMessage: "dflt",
	})
	if status != http.StatusConflict || body["code"] != "conflict" || body["msg"] != "first" {
		t.Fatalf("status=%d body=%v, want 409 conflict/first", status, body)
	}
}

func TestRenderMappedScopeSelectsRows(t *testing.T) {
	sentinel := errors.New("invalid code")
	table := []Mapping{
		{Targets: []error{sentinel}, Code: "bad_request", Message: "invalid", Scope: "admin"},
		{Targets: []error{sentinel}, Code: "validation_failed", Message: "unavailable", Scope: "redeem"},
	}
	opts := MappedOptions{DefaultStatus: http.StatusInternalServerError, DefaultCode: "d", DefaultMessage: "d"}
	if status, body := renderMapped(t, sentinel, table, withScope(opts, "redeem")); status != http.StatusUnprocessableEntity || body["code"] != "validation_failed" {
		t.Fatalf("redeem scope: status=%d body=%v", status, body)
	}
	if status, body := renderMapped(t, sentinel, table, withScope(opts, "admin")); status != http.StatusBadRequest || body["code"] != "bad_request" {
		t.Fatalf("admin scope: status=%d body=%v", status, body)
	}
}

func TestRenderMappedStatusDerivesFromCentralRegistry(t *testing.T) {
	sentinel := errors.New("boom")
	// The edge table carries no status: an unregistered Code fails closed
	// to 500 through the central registry, never a caller mistake.
	table := []Mapping{{Targets: []error{sentinel}, Code: "unregistered_code_xyz", Message: "m"}}
	status, body := renderMapped(t, sentinel, table, MappedOptions{
		DefaultStatus: http.StatusBadRequest, DefaultCode: "d", DefaultMessage: "dflt",
	})
	if status != http.StatusInternalServerError || body["code"] != "unregistered_code_xyz" {
		t.Fatalf("unregistered code: status=%d body=%v, want 500 fail-closed", status, body)
	}
}

func withScope(opts MappedOptions, scope string) MappedOptions {
	opts.Scope = scope
	return opts
}

func TestRenderMappedDefaultAndNumeric(t *testing.T) {
	table := []Mapping{{Targets: []error{errors.New("other")}, Code: "unregistered_for_default_path", Message: "x"}}
	status, body := renderMapped(t, errors.New("unknown"), table, MappedOptions{
		DefaultStatus: http.StatusInternalServerError, DefaultCode: "op_failed", DefaultMessage: "failed",
	})
	if status != http.StatusInternalServerError || body["code"] != "op_failed" {
		t.Fatalf("machine default: status=%d body=%v", status, body)
	}
	status, _ = renderMapped(t, errors.New("unknown"), table, MappedOptions{
		DefaultStatus: http.StatusInternalServerError, DefaultMessage: "legacy",
		NumericDefault: true,
	})
	if status != http.StatusInternalServerError {
		t.Fatalf("numeric default: status=%d", status)
	}
}

func TestRenderMappedEmptyMessageFallsBackWithoutLeak(t *testing.T) {
	const secret = "SECRET_MUST_NOT_ESCAPE"
	sentinel := errors.New("sentinel")
	table := []Mapping{{Targets: []error{sentinel}, Code: "bad_request", Message: ""}}
	status, body := renderMapped(t, errors.Join(sentinel, errors.New(secret)), table, MappedOptions{
		DefaultStatus: http.StatusInternalServerError, DefaultCode: "d", DefaultMessage: "dflt",
		FallbackMessage: "fallback",
	})
	if status != http.StatusBadRequest || body["code"] != "bad_request" || body["msg"] != "fallback" {
		t.Fatalf("fallback: status=%d body=%v", status, body)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("response leaked wrapped detail: %s", raw)
	}
}
