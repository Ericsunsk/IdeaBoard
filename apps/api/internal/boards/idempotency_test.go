package boards

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func newBoardIdempotencyRouter(t *testing.T, handler *Handler, method, route string, run gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, route, func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 42, principal.RoleUser)
		run(c)
	})
	return router
}

func serveBoardWrite(t *testing.T, router *gin.Engine, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertBoardMachineError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var payload struct {
		Code        string `json:"code"`
		MessageSafe bool   `json:"message_safe"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode machine error: %v; body=%s", err, recorder.Body.String())
	}
	if payload.Code != wantCode || !payload.MessageSafe {
		t.Fatalf("machine error = %+v, want code=%q safe=true", payload, wantCode)
	}
}

func TestBoardPostWritesRequireIdempotencyKey(t *testing.T) {
	handler := NewHandler(nil, config.PublicOriginsConfig{})
	cases := []struct {
		name   string
		method string
		path   string
		route  string
		run    gin.HandlerFunc
		body   string
	}{
		{
			name: "invite member", method: http.MethodPost,
			path: "/boards/board-1/members/invite", route: "/boards/:board_id/members/invite",
			run:  handler.InviteMember,
			body: `{"member_public_id":"00000000-0000-0000-0000-000000000001","role":"editor"}`,
		},
		{
			name: "create share token", method: http.MethodPost,
			path: "/boards/board-1/share-tokens", route: "/boards/:board_id/share-tokens",
			run: handler.CreateShareToken, body: `{"role":"viewer"}`,
		},
		{
			name: "revoke share token", method: http.MethodPost,
			path: "/boards/board-1/share-tokens/7/revoke", route: "/boards/:board_id/share-tokens/:token_id/revoke",
			run: handler.RevokeShareToken,
		},
		{
			name: "join by share token", method: http.MethodPost,
			path: "/boards/share-tokens/join", route: "/boards/share-tokens/join",
			run: handler.JoinByShareToken, body: `{"token":"join-token"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newBoardIdempotencyRouter(t, handler, tc.method, tc.route, tc.run)
			recorder := serveBoardWrite(t, router, tc.method, tc.path, tc.body, "")
			assertBoardMachineError(t, recorder, http.StatusBadRequest, "idempotency_key_required")
		})
	}
}

func TestBoardInviteMemberReplaysFirstResult(t *testing.T) {
	handler := NewHandler(nil, config.PublicOriginsConfig{})
	const boardID = "board-1"
	const memberPublicID = "00000000-0000-0000-0000-000000000001"
	requestHash := idempotency.HashRequest(boardID, memberPublicID, "editor")
	seeded := boardMemberResponse{Member: &MemberWithUser{Role: RoleEditor, DisplayName: "Seeded"}}
	handler.replay.StoreResult(boardInviteMemberScope, 42, "invite-key", requestHash, seeded)

	router := newBoardIdempotencyRouter(t, handler, http.MethodPost, "/boards/:board_id/members/invite", handler.InviteMember)
	recorder := serveBoardWrite(t, router, http.MethodPost, "/boards/board-1/members/invite",
		`{"member_public_id":"00000000-0000-0000-0000-000000000001","role":"editor"}`, "invite-key")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Data boardMemberResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode replay: %v; body=%s", err, recorder.Body.String())
	}
	if payload.Data.Member == nil || payload.Data.Member.DisplayName != "Seeded" {
		t.Fatalf("replayed member = %+v, want seeded first result", payload.Data)
	}
}

func TestBoardCreateShareTokenReplaysFirstResult(t *testing.T) {
	handler := NewHandler(nil, config.PublicOriginsConfig{})
	requestHash := idempotency.HashRequest("board-1", "viewer", "")
	seeded := boardShareTokenResponse{Token: boardShareTokenDTO{ID: 99, TokenPrefix: "seed"}}
	handler.replay.StoreResult(boardCreateShareScope, 42, "share-key", requestHash, seeded)

	router := newBoardIdempotencyRouter(t, handler, http.MethodPost, "/boards/:board_id/share-tokens", handler.CreateShareToken)
	recorder := serveBoardWrite(t, router, http.MethodPost, "/boards/board-1/share-tokens", `{"role":"viewer"}`, "share-key")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Data boardShareTokenResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode replay: %v; body=%s", err, recorder.Body.String())
	}
	if payload.Data.Token.ID != 99 || payload.Data.Token.TokenPrefix != "seed" {
		t.Fatalf("replayed token = %+v, want seeded first result", payload.Data)
	}
}

func TestBoardPostWritesConflictOnKeyReuse(t *testing.T) {
	handler := NewHandler(nil, config.PublicOriginsConfig{})
	handler.replay.StoreResult(boardInviteMemberScope, 42, "reuse-key",
		idempotency.HashRequest("board-1", "00000000-0000-0000-0000-000000000001", "editor"), boardMemberResponse{})

	router := newBoardIdempotencyRouter(t, handler, http.MethodPost, "/boards/:board_id/members/invite", handler.InviteMember)
	recorder := serveBoardWrite(t, router, http.MethodPost, "/boards/board-1/members/invite",
		`{"member_public_id":"00000000-0000-0000-0000-000000000002","role":"viewer"}`, "reuse-key")
	assertBoardMachineError(t, recorder, http.StatusConflict, "idempotency_conflict")
}
