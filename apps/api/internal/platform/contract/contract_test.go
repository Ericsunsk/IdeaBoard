package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func newContractTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces", nil)
	return c, recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, recorder.Body.String())
	}
	return payload
}

func TestRenderSuccessEnvelopeShape(t *testing.T) {
	c, recorder := newContractTestContext()
	RenderSuccess(c, map[string]string{"name": "workspace"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	payload := decodeBody(t, recorder)
	data, ok := payload["data"].(map[string]any)
	if !ok || data["name"] != "workspace" {
		t.Fatalf("data = %v, want name=workspace", payload["data"])
	}
	if _, ok := payload["error"]; ok {
		t.Fatalf("success envelope must not carry error: %s", recorder.Body.String())
	}
	if _, ok := payload["meta"]; ok {
		t.Fatalf("single-resource envelope must omit meta: %s", recorder.Body.String())
	}
}

func TestRenderPaginatedEnvelopeShape(t *testing.T) {
	c, recorder := newContractTestContext()
	RenderPaginated(c, []string{"a"}, NewPageMeta("opaque_cursor", true))
	payload := decodeBody(t, recorder)
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("paginated envelope requires meta: %s", recorder.Body.String())
	}
	if meta["next_cursor"] != "opaque_cursor" || meta["has_more"] != true {
		t.Fatalf("meta = %v, want next_cursor/has_more", meta)
	}
}

func TestRenderErrorShapeIncludesRequestID(t *testing.T) {
	c, recorder := newContractTestContext()
	c.Set(response.RequestIDHeader, "req-legacy-1")
	RenderCode(c, CodePaginationCursorInvalid, "cursor invalid")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	payload := decodeBody(t, recorder)
	errBody, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope requires error object: %s", recorder.Body.String())
	}
	if errBody["code"] != string(CodePaginationCursorInvalid) {
		t.Fatalf("code = %v, want pagination_cursor_invalid", errBody["code"])
	}
	if errBody["request_id"] != "req-legacy-1" {
		t.Fatalf("request_id = %v, want req-legacy-1", errBody["request_id"])
	}

	// Contract Request-Id takes precedence over the legacy transport header.
	c2, recorder2 := newContractTestContext()
	c2.Set(response.RequestIDHeader, "req-legacy-2")
	c2.Set(RequestIDHeader, "req-v1-2")
	RenderError(c2, http.StatusConflict, CodeTaskIdempotencyConflict, "conflict")
	if recorder2.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder2.Code)
	}
	errBody2 := decodeBody(t, recorder2)["error"].(map[string]any)
	if errBody2["request_id"] != "req-v1-2" {
		t.Fatalf("request_id = %v, want req-v1-2", errBody2["request_id"])
	}
}

func TestRenderErrorWithFieldDetails(t *testing.T) {
	c, recorder := newContractTestContext()
	RenderErrorWithDetails(c, StatusOf(CodeValidationFailed), CodeValidationFailed,
		"validation failed", FieldErrors{Fields: map[string]string{"name": "required"}})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	details := decodeBody(t, recorder)["error"].(map[string]any)["details"].(map[string]any)
	fields, ok := details["fields"].(map[string]any)
	if !ok || fields["name"] != "required" {
		t.Fatalf("details.fields = %v, want name=required", details)
	}
}

func TestStatusMatrix(t *testing.T) {
	cases := map[Code]int{
		CodePaginationCursorInvalid: http.StatusBadRequest,
		CodeIdempotencyKeyRequired:  http.StatusBadRequest,
		CodeBadRequest:              http.StatusBadRequest,
		CodeUnauthorized:            http.StatusUnauthorized,
		CodeForbidden:               http.StatusForbidden,
		CodeNotFound:                http.StatusNotFound,
		CodeTaskIdempotencyConflict: http.StatusConflict,
		CodeIdempotencyConflict:     http.StatusConflict,
		CodeConflict:                http.StatusConflict,
		CodeValidationFailed:        http.StatusUnprocessableEntity,
		CodeRateLimited:             http.StatusTooManyRequests,
		CodeServiceUnavailable:      http.StatusServiceUnavailable,
		CodeContractNotImplemented:  http.StatusNotImplemented,
		Code("not_a_kernel_code"):   http.StatusInternalServerError,
	}
	for code, want := range cases {
		if got := StatusOf(code); got != want {
			t.Errorf("StatusOf(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestParsePageSizeBounds(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"", DefaultPageSize, false},
		{"  ", DefaultPageSize, false},
		{"1", 1, false},
		{"20", 20, false},
		{"100", 100, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"101", 0, true},
		{"200", 0, true},
		{"abc", 0, true},
		{"20.5", 0, true},
	}
	for _, tc := range cases {
		got, err := ParsePageSize(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParsePageSize(%q) = %d, want error", tc.raw, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParsePageSize(%q) = %d, %v; want %d, nil", tc.raw, got, err, tc.want)
		}
	}
	if DefaultPageSize != 20 || MaxPageSize != 100 {
		t.Fatalf("kernel bounds drifted: default=%d max=%d, want 20/100", DefaultPageSize, MaxPageSize)
	}
}

type contractTestCursor struct {
	Seq int    `json:"seq"`
	UID string `json:"uid"`
}

func TestCursorRoundTripAndInvalid(t *testing.T) {
	encoded, err := EncodeCursor(contractTestCursor{Seq: 7, UID: "usr_123"})
	if err != nil || encoded == "" {
		t.Fatalf("EncodeCursor err=%v", err)
	}
	decoded, err := DecodeCursor[contractTestCursor](encoded)
	if err != nil || decoded == nil || decoded.Seq != 7 || decoded.UID != "usr_123" {
		t.Fatalf("DecodeCursor = %+v, %v", decoded, err)
	}
	// First page: empty cursor decodes to nil without error.
	first, err := DecodeCursor[contractTestCursor]("")
	if err != nil || first != nil {
		t.Fatalf("empty cursor = %+v, %v; want nil, nil", first, err)
	}
	for _, bad := range []string{"!!!", "not-base64!!", strings.Repeat("a", 513)} {
		_, err := DecodeCursor[contractTestCursor](bad)
		if !IsCursorInvalid(err) {
			t.Errorf("cursor %q err=%v, want cursor-invalid classification", bad, err)
		}
	}
}

func TestIdempotencyReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	store := NewIdempotencyStore(DefaultIdempotencyStoreOptions())
	scope := IdempotencyScope("usr_123", "task.create")
	hash := HashIdempotencyRequest("task.create", "fingerprint")

	first, replayed, err := store.Do(ctx, scope, 42, "key-1", hash, func() (any, error) {
		return "first-result", nil
	})
	if err != nil || replayed || first != "first-result" {
		t.Fatalf("leader = %v, replayed=%v, err=%v", first, replayed, err)
	}

	// Same key + same hash replays the first result.
	second, replayed, err := store.Do(ctx, scope, 42, "key-1", hash, func() (any, error) {
		return "second-result", nil
	})
	if err != nil || !replayed || second != "first-result" {
		t.Fatalf("replay = %v, replayed=%v, err=%v; want first-result, true, nil", second, replayed, err)
	}

	// Same key + different hash is a 409 conflict.
	_, _, err = store.Do(ctx, scope, 42, "key-1", HashIdempotencyRequest("task.create", "other"), func() (any, error) {
		return "other-result", nil
	})
	if !IsIdempotencyConflict(err) {
		t.Fatalf("different-hash reuse err=%v, want idempotency conflict", err)
	}
}

func TestIdempotencyKeyHeaderOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws_1/tasks", nil)
	c.Request.Header.Set(IdempotencyKeyHeader, "  key-abc  ")
	key, err := IdempotencyKeyFromHeader(c)
	if err != nil || key != "key-abc" {
		t.Fatalf("header key = %q, %v; want key-abc, nil", key, err)
	}

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws_1/tasks", nil)
	if _, err := IdempotencyKeyFromHeader(c2); err == nil {
		t.Fatal("missing header must return the classified required-key error")
	}
	if _, err := ParseIdempotencyKey(""); err == nil {
		t.Fatal("empty key must fail validation")
	}
}

func TestStrictJSONRejectsUnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	var ok payload
	if err := DecodeStrict([]byte(`{"name":"w"}`), &ok); err != nil || ok.Name != "w" {
		t.Fatalf("valid body err=%v", err)
	}
	var unknown payload
	if err := DecodeStrict([]byte(`{"name":"w","extra":1}`), &unknown); err == nil {
		t.Fatal("unknown field must be rejected by default")
	}
	var trailing payload
	if err := DecodeStrict([]byte(`{"name":"w"} {"name":"x"}`), &trailing); err == nil {
		t.Fatal("trailing payload must be rejected")
	}
}

func TestPublicIDValidators(t *testing.T) {
	valid := map[string]func(string) bool{
		"usr_abc123":    IsUserID,
		"org_abc123":    IsOrganizationID,
		"ws_abc123":     IsWorkspaceID,
		"board_abc123":  IsBoardID,
		"task_abc123":   IsTaskID,
		"asset_abc123":  IsAssetID,
		"upload_abc123": IsUploadID,
		"evt_abc123":    IsEventID,
		"usr_a-1_B2":    IsUserID,
		"task_550e8400": IsTaskID,
	}
	for id, check := range valid {
		if !check(id) {
			t.Errorf("valid id %q rejected", id)
		}
		if prefix, ok := PrefixOf(id); !ok || prefix == "" {
			t.Errorf("PrefixOf(%q) = %q, %v; want prefix", id, prefix, ok)
		}
	}
	invalid := []string{"", "usr_", "usr_ with space", "usr_a/b", "123", "user_abc", "ws_"}
	for _, id := range invalid {
		if IsValidPublicID(PrefixUser, id) && strings.HasPrefix(id, PrefixUser) {
			t.Errorf("invalid id %q accepted", id)
		}
	}
	if IsTaskID("usr_abc123") {
		t.Error("usr_ id must not validate as task_")
	}
	if _, ok := PrefixOf("internal-42"); ok {
		t.Error("numeric internal id must not carry a kernel prefix")
	}
}

func TestMintPublicID(t *testing.T) {
	for _, prefix := range []string{PrefixWorkspace, PrefixBoard, PrefixUpload, PrefixAsset} {
		first, err := MintPublicID(prefix, 12)
		if err != nil {
			t.Fatalf("MintPublicID(%q): %v", prefix, err)
		}
		second, err := MintPublicID(prefix, 12)
		if err != nil {
			t.Fatalf("MintPublicID(%q): %v", prefix, err)
		}
		if first == second {
			t.Errorf("MintPublicID(%q) minted duplicate ids", prefix)
		}
		if !IsValidPublicID(prefix, first) {
			t.Errorf("MintPublicID(%q) = %q, want valid shape", prefix, first)
		}
	}
}
