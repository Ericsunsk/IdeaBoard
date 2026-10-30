package contract

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
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

func TestStatusMatrixPromotedEdgeCodes(t *testing.T) {
	// Every code promoted from a domain edge table pins its exact
	// pre-promotion HTTP status here, so a registry edit that
	// re-maps a status fails loudly (wire freeze).
	cases := map[Code]int{
		CodeBillingInvalidInput:       http.StatusBadRequest,
		CodeBillingSKUNotFound:        http.StatusNotFound,
		CodeBillingPricingUnavailable: http.StatusUnprocessableEntity,
		CodeBillingItemNotArchived:    http.StatusConflict,
		CodeBillingArchivedItemInUse:  http.StatusConflict,
		CodeBillingQuoteTotalExceeded: http.StatusUnprocessableEntity,
		CodeBillingServiceUnavailable: http.StatusServiceUnavailable,
		CodeBillingOperationFailed:    http.StatusInternalServerError,

		CodeRewardInvalidInput:            http.StatusBadRequest,
		CodeRewardCodeIDInvalid:           http.StatusBadRequest,
		CodeRewardRedemptionIDInvalid:     http.StatusBadRequest,
		CodeRewardPaginationInvalid:       http.StatusBadRequest,
		CodeRewardCodeInvalid:             http.StatusBadRequest,
		CodeRewardCodeConflict:            http.StatusConflict,
		CodeRewardCodeIdempotencyConflict: http.StatusConflict,
		CodeRewardCodeNotFound:            http.StatusNotFound,
		CodeRewardCodeCapInvalid:          http.StatusConflict,
		CodeRewardCodeRedeemed:            http.StatusConflict,
		CodeRewardRedemptionNotFound:      http.StatusNotFound,
		CodeRewardCodeUnavailable:         http.StatusUnprocessableEntity,
		CodeRewardRedeemConflict:          http.StatusConflict,
		CodeRewardWalletLocked:            http.StatusConflict,
		CodeRewardRateLimited:             http.StatusTooManyRequests,
		CodeRewardServiceUnavailable:      http.StatusServiceUnavailable,
		CodeRewardOperationFailed:         http.StatusInternalServerError,
		CodeRewardRedeemFailed:            http.StatusInternalServerError,

		CodeBoardInvalidInput:      http.StatusBadRequest,
		CodeBoardNameConflict:      http.StatusConflict,
		CodeBoardIDConflict:        http.StatusConflict,
		CodeBoardPermissionDenied:  http.StatusForbidden,
		CodeBoardMemberRoleInvalid: http.StatusBadRequest,
		CodeBoardMemberSelfInvalid: http.StatusBadRequest,
		CodeBoardOperationFailed:   http.StatusInternalServerError,

		CodeStripeConfigurationInvalidInput:  http.StatusBadRequest,
		CodeStripePermissionDenied:           http.StatusForbidden,
		CodeStripeConfigurationUnprocessable: http.StatusUnprocessableEntity,
		CodeStripeProviderUnavailable:        http.StatusServiceUnavailable,
		CodeStripeConfigurationChanged:       http.StatusConflict,
		CodeCheckoutInvalidInput:             http.StatusBadRequest,
		CodeCheckoutConflict:                 http.StatusConflict,
		CodePaymentAlreadyCompleted:          http.StatusUnprocessableEntity,
		CodeCheckoutSessionExpired:           http.StatusUnprocessableEntity,
		CodeWalletNotRechargeable:            http.StatusConflict,
		CodeCheckoutRequestRejected:          http.StatusUnprocessableEntity,
		CodePaymentServiceUnavailable:        http.StatusServiceUnavailable,
		CodePurchasesUnavailable:             http.StatusServiceUnavailable,
		CodeRechargeHistoryLimitInvalid:      http.StatusBadRequest,
		CodeRechargeHistoryCursorInvalid:     http.StatusBadRequest,
		CodeWebhookSignatureInvalid:          http.StatusBadRequest,
		CodeWebhookConfigurationMissing:      http.StatusServiceUnavailable,
		CodeAdminPaymentOrderFilterInvalid:   http.StatusBadRequest,
		CodeAdminPaymentOrderCursorInvalid:   http.StatusBadRequest,

		CodeIdentityRequestInvalid:      http.StatusBadRequest,
		CodeIdentityUnauthorized:        http.StatusUnauthorized,
		CodeIdentityUserPublicIDInvalid: http.StatusBadRequest,
		CodeIdentityUserNotFound:        http.StatusNotFound,
		CodeIdentityForbidden:           http.StatusForbidden,
		CodeIdentityConflict:            http.StatusConflict,
		CodeIdentityUpstreamRejected:    http.StatusForbidden,
		CodeIdentityUpstreamUnavailable: http.StatusBadGateway,
		CodeIdentityOperationFailed:     http.StatusInternalServerError,
		CodeIdentityServiceUnavailable:  http.StatusServiceUnavailable,

		CodeStorageInvalidInput:       http.StatusBadRequest,
		CodeStorageServiceUnavailable: http.StatusServiceUnavailable,
		CodeStorageProfileInvalid:     http.StatusBadRequest,
		CodeStorageOperationFailed:    http.StatusInternalServerError,

		CodeSearchProviderNotFound:       http.StatusNotFound,
		CodeSearchRequestInvalid:         http.StatusBadRequest,
		CodeSearchProviderTestRequired:   http.StatusConflict,
		CodeSearchProviderAPIKeyInvalid:  http.StatusBadRequest,
		CodeSearchProviderAPIKeyRequired: http.StatusConflict,
		CodeSearchProviderTestFailed:     http.StatusBadRequest,

		CodeSearchUnavailable:   http.StatusServiceUnavailable,
		CodeSearchLimitInvalid:  http.StatusBadRequest,
		CodeSearchQueryInvalid:  http.StatusBadRequest,
		CodeSearchCursorInvalid: http.StatusBadRequest,

		CodeVisionSearchProviderError:    http.StatusBadGateway,
		CodeVisionSearchInputInvalid:     http.StatusBadRequest,
		CodeVisionSearchImageInvalid:     http.StatusBadRequest,
		CodeVisionSearchImageUnavailable: http.StatusBadRequest,
		CodeVisionSearchNotConfigured:    http.StatusServiceUnavailable,
		CodeVisionSearchUnavailable:      http.StatusServiceUnavailable,
		CodeVisionSearchSchemaError:      http.StatusBadGateway,
		CodeVisionSearchCredentials:      http.StatusBadGateway,
		CodeVisionSearchQuota:            http.StatusBadGateway,
		CodeVisionSearchTimeout:          http.StatusGatewayTimeout,
		CodeVisionSearchNetwork:          http.StatusBadGateway,
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

func TestLenientJSONIgnoresUnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	// Unknown fields bind fine.
	var ignored payload
	if err := DecodeLenient([]byte(`{"name":"w","extra":1}`), &ignored); err != nil || ignored.Name != "w" {
		t.Fatalf("lenient body err=%v", err)
	}
	// Malformed JSON and trailing payloads still fail.
	var broken payload
	if err := DecodeLenient([]byte(`{"name":}`), &broken); err == nil {
		t.Fatal("malformed body must be rejected by lenient decoding")
	}
	var trailing payload
	if err := DecodeLenient([]byte(`{"name":"w"} {"name":"x"}`), &trailing); err == nil {
		t.Fatal("trailing payload must be rejected by lenient decoding")
	}
	// The gin binder ignores unknown fields but still rejects malformed
	// bodies and trailing payloads.
	bind := func(body string) error {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/boards", strings.NewReader(body))
		var dst payload
		return BindLenientJSON(c, &dst)
	}
	if err := bind(`{"name":"w","extra":1}`); err != nil {
		t.Fatalf("BindLenientJSON unknown field err=%v", err)
	}
	if err := bind(`{"name":}`); err == nil {
		t.Fatal("BindLenientJSON must reject malformed JSON")
	}
	if err := bind(`{"name":"w"} {"name":"x"}`); err == nil {
		t.Fatal("BindLenientJSON must reject trailing payloads")
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
		"ba_abc123":     IsBillingAccountID,
		"rwd_abc123":    IsRewardCodeID,
		"rdm_abc123":    IsRewardRedemptionID,
		"rec_abc123":    IsReconciliationRunID,
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
	for _, prefix := range []string{PrefixWorkspace, PrefixBoard, PrefixUpload, PrefixAsset, PrefixBillingAccount} {
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

func TestStatusMatrixWorkspaceKernelCodes(t *testing.T) {
	// Every workspace-surface kernel code (#801-#808, tasking statistics)
	// pins its exact HTTP status here, so a registry edit that re-maps
	// a status fails loudly (wire freeze). Follows the
	// TestStatusMatrixPromotedEdgeCodes idiom.
	cases := map[Code]int{
		CodeWorkspaceNotFound:               http.StatusNotFound,
		CodeWorkspaceForbidden:              http.StatusForbidden,
		CodeWorkspaceIdempotencyConflict:    http.StatusConflict,
		CodeOrganizationNotFound:            http.StatusNotFound,
		CodeOrganizationForbidden:           http.StatusForbidden,
		CodeOrganizationIdempotencyConflict: http.StatusConflict,
		CodeOrganizationClosed:              http.StatusConflict,
		CodeOrganizationClosureBlocked:      http.StatusConflict,
		CodeWorkspaceMemberNotFound:         http.StatusNotFound,
		CodeWorkspaceMemberLimitReached:     http.StatusConflict,
		CodeMembershipTransitionForbidden:   http.StatusForbidden,

		CodeBoardNotFound:             http.StatusNotFound,
		CodeBoardForbidden:            http.StatusForbidden,
		CodeBoardArchived:             http.StatusConflict,
		CodeBoardRevisionConflict:     http.StatusConflict,
		CodeBoardAccessConflict:       http.StatusPreconditionFailed,
		CodeBoardPreconditionRequired: http.StatusPreconditionRequired,
		CodeBoardIdempotencyConflict:  http.StatusConflict,
		CodeBoardMemberNotFound:       http.StatusNotFound,

		CodeUploadNotFound:            http.StatusNotFound,
		CodeUploadExpired:             http.StatusGone,
		CodeUploadStateConflict:       http.StatusConflict,
		CodeUploadIdempotencyConflict: http.StatusConflict,
		CodeUploadValidationFailed:    http.StatusUnprocessableEntity,
		CodeMediaUnsupportedType:      http.StatusUnsupportedMediaType,
		CodeMediaNotFound:             http.StatusNotFound,
		CodeMediaNotReady:             http.StatusConflict,
		CodeMediaArchived:             http.StatusGone,
		CodeMediaIdempotencyConflict:  http.StatusConflict,
		CodeMediaValidationFailed:     http.StatusUnprocessableEntity,

		CodeBillingWalletRestricted:   http.StatusForbidden,
		CodeBillingCheckoutConflict:   http.StatusConflict,
		CodeBillingSessionNotFound:    http.StatusNotFound,
		CodeBillingQuoteUnavailable:   http.StatusServiceUnavailable,
		CodeBillingPricingUnavailable: http.StatusUnprocessableEntity,

		CodeTaskNotFound:            http.StatusNotFound,
		CodeTaskForbidden:           http.StatusForbidden,
		CodeTaskStateConflict:       http.StatusConflict,
		CodeTaskInsufficientCredits: http.StatusPaymentRequired,

		CodeMemberQuotaExhausted: http.StatusForbidden,

		CodeLibraryFolderNotFound:          http.StatusNotFound,
		CodeLibraryAssetNotFound:           http.StatusNotFound,
		CodeLibraryMaterializationNotFound: http.StatusNotFound,
		CodeLibraryIdempotencyConflict:     http.StatusConflict,

		CodeSearchProviderUnavailable: http.StatusServiceUnavailable,
		CodeSearchVisionProviderError: http.StatusBadGateway,
		CodeSearchVisionNotConfigured: http.StatusServiceUnavailable,
		CodeSearchIdempotencyConflict: http.StatusConflict,

		CodeEventNotFound:             http.StatusNotFound,
		CodeEventForbidden:            http.StatusForbidden,
		CodeEventSubscriptionNotFound: http.StatusNotFound,
		CodeEventSubscriptionConflict: http.StatusConflict,

		CodeControlNotFound: http.StatusNotFound,

		CodeCreditInvalidInput:              http.StatusBadRequest,
		CodeCreditIdempotencyConflict:       http.StatusConflict,
		CodeCreditWalletNotFound:            http.StatusNotFound,
		CodeCreditWalletFrozen:              http.StatusLocked,
		CodeCreditWalletRestricted:          http.StatusLocked,
		CodeCreditInsufficientCredits:       http.StatusUnprocessableEntity,
		CodeCreditChargeNotFound:            http.StatusNotFound,
		CodeCreditRefundExceedsCharge:       http.StatusUnprocessableEntity,
		CodeCreditReconciliationNotFound:    http.StatusNotFound,
		CodeCreditReconciliationUnavailable: http.StatusServiceUnavailable,
		CodeCreditOperationFailed:           http.StatusInternalServerError,

		CodeTaskMonitorConcurrencyLimited: http.StatusTooManyRequests,
		CodeTaskStatisticsRequestInvalid:  http.StatusBadRequest,
	}
	for code, want := range cases {
		if got := StatusOf(code); got != want {
			t.Errorf("StatusOf(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestErrorCodeRegistryConsistency(t *testing.T) {
	// Every declared wire code must appear in the single status registry
	// and vice versa. Explicit 500 entries distinguish registered server
	// failures from unknown codes, and wire strings must remain unique.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filepath.Join(filepath.Dir(thisFile), "errorcodes.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse errorcodes.go: %v", err)
	}
	declared := map[string]string{}
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeName, ok := vs.Type.(*ast.Ident)
			if !ok || typeName.Name != "Code" {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Code") {
					continue
				}
				wire := ""
				if i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						wire = strings.Trim(lit.Value, `"`)
					}
				}
				declared[name.Name] = wire
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("no Code consts found in errorcodes.go")
	}
	registryPath := filepath.Join(filepath.Dir(thisFile), "..", "errorrender", "registry.go")
	registry, err := parser.ParseFile(fset, registryPath, nil, 0)
	if err != nil {
		t.Fatalf("parse errorrender registry: %v", err)
	}
	var statusForCode *ast.FuncDecl
	for _, decl := range registry.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "StatusForCode" {
			statusForCode = fn
		}
	}
	if statusForCode == nil {
		t.Fatal("StatusForCode not found in errorrender registry")
	}
	registered := map[string]bool{}
	ast.Inspect(statusForCode.Body, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range cc.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("status registry case must name a wire string: %T", expr)
				continue
			}
			wire, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("invalid registry wire literal %q: %v", lit.Value, err)
				continue
			}
			registered[wire] = true
		}
		return true
	})
	// Wire-string uniqueness. Non-literal values are the two promoted
	// idempotency aliases; resolve them through the live consts and
	// fail loud if a third non-literal form ever appears.
	aliasValues := map[string]Code{
		"CodeIdempotencyKeyRequired": CodeIdempotencyKeyRequired,
		"CodeIdempotencyConflict":    CodeIdempotencyConflict,
	}
	seen := map[string]string{}
	for name, wire := range declared {
		if wire == "" {
			alias, ok := aliasValues[name]
			if !ok {
				t.Errorf("contract.%s has a non-literal value outside the known idempotency aliases", name)
				continue
			}
			wire = string(alias)
		}
		if prev, dup := seen[wire]; dup {
			t.Errorf("wire string %q shared by contract.%s and contract.%s", wire, prev, name)
			continue
		}
		seen[wire] = name
		if !registered[wire] {
			t.Errorf("contract.%s (%q) has no status registry mapping", name, wire)
		}
	}
	for wire := range registered {
		if _, ok := seen[wire]; !ok {
			t.Errorf("status registry maps %q which is not a declared Code", wire)
		}
	}
}

func TestHandlePreconditionNotModified(t *testing.T) {
	t.Run("empty current ETag returns false", func(t *testing.T) {
		c, rec := newContractTestContext()
		c.Request.Header.Set("If-None-Match", "rev-1")
		if HandlePreconditionNotModified(c, "") {
			t.Fatal("expected false for empty current ETag")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want default 200", rec.Code)
		}
	})

	t.Run("no client If-None-Match returns false", func(t *testing.T) {
		c, rec := newContractTestContext()
		if HandlePreconditionNotModified(c, "rev-1") {
			t.Fatal("expected false when If-None-Match is absent")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want default 200", rec.Code)
		}
	})

	t.Run("non-matching If-None-Match returns false", func(t *testing.T) {
		c, _ := newContractTestContext()
		c.Request.Header.Set("If-None-Match", "rev-old")
		if HandlePreconditionNotModified(c, "rev-new") {
			t.Fatal("expected false when ETag does not match")
		}
	})

	t.Run("matching If-None-Match returns 304 Not Modified", func(t *testing.T) {
		c, rec := newContractTestContext()
		c.Request.Header.Set("If-None-Match", "rev-1")
		if !HandlePreconditionNotModified(c, "rev-1") {
			t.Fatal("expected true when ETag matches")
		}
		if !c.IsAborted() {
			t.Fatal("expected context to be aborted")
		}
		if rec.Code != http.StatusNotModified {
			t.Fatalf("code = %d, want 304", rec.Code)
		}
		if rec.Header().Get("ETag") != "rev-1" {
			t.Fatalf("ETag = %q, want rev-1", rec.Header().Get("ETag"))
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("expected empty body for 304, got %q", rec.Body.String())
		}
	})

	t.Run("wildcard If-None-Match returns 304 Not Modified", func(t *testing.T) {
		c, rec := newContractTestContext()
		c.Request.Header.Set("If-None-Match", "*")
		if !HandlePreconditionNotModified(c, "rev-any") {
			t.Fatal("expected true when If-None-Match is wildcard *")
		}
		if !c.IsAborted() {
			t.Fatal("expected context to be aborted")
		}
		if rec.Code != http.StatusNotModified {
			t.Fatalf("code = %d, want 304", rec.Code)
		}
		if rec.Header().Get("ETag") != "rev-any" {
			t.Fatalf("ETag = %q, want rev-any", rec.Header().Get("ETag"))
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("expected empty body for 304, got %q", rec.Body.String())
		}
	})
}
