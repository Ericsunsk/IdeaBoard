package boards

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
)

func TestBoardHandlersRequireTypedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewBoardHandler(nil, nil)
	handlers := []struct {
		name   string
		method string
		path   string
		route  string
		run    gin.HandlerFunc
	}{
		{name: "list", method: http.MethodGet, path: "/boards", route: "/boards", run: handler.ListBoards},
		{name: "upsert", method: http.MethodPut, path: "/boards/board-1", route: "/boards/:board_id", run: handler.UpsertBoard},
		{name: "delete", method: http.MethodDelete, path: "/boards/board-1", route: "/boards/:board_id", run: handler.DeleteBoard},
		{name: "list members", method: http.MethodGet, path: "/boards/board-1/members", route: "/boards/:board_id/members", run: handler.ListMembers},
		{name: "invite member", method: http.MethodPost, path: "/boards/board-1/members", route: "/boards/:board_id/members", run: handler.InviteMember},
		{name: "update member", method: http.MethodPut, path: "/boards/board-1/members/u1", route: "/boards/:board_id/members/:member_uid", run: handler.UpdateMemberRole},
		{name: "remove member", method: http.MethodDelete, path: "/boards/board-1/members/u1", route: "/boards/:board_id/members/:member_uid", run: handler.RemoveMember},
		{name: "create share", method: http.MethodPost, path: "/boards/board-1/shares", route: "/boards/:board_id/shares", run: handler.CreateShareToken},
		{name: "list shares", method: http.MethodGet, path: "/boards/board-1/shares", route: "/boards/:board_id/shares", run: handler.ListShareTokens},
		{name: "revoke share", method: http.MethodDelete, path: "/boards/board-1/shares/1", route: "/boards/:board_id/shares/:token_id", run: handler.RevokeShareToken},
		{name: "join share", method: http.MethodPost, path: "/boards/join", route: "/boards/join", run: handler.JoinByShareToken},
	}
	identityStates := []struct {
		name  string
		setup func(*gin.Context)
	}{
		{name: "missing"},
		{name: "legacy key", setup: func(c *gin.Context) { c.Set("user_id", int64(7)) }},
		{name: "invalid Principal", setup: func(c *gin.Context) { _ = principal.Set(c, principal.Principal{}) }},
	}

	for _, handlerCase := range handlers {
		for _, identityCase := range identityStates {
			t.Run(handlerCase.name+"/"+identityCase.name, func(t *testing.T) {
				router := gin.New()
				router.Handle(handlerCase.method, handlerCase.route, func(c *gin.Context) {
					if identityCase.setup != nil {
						identityCase.setup(c)
					}
					handlerCase.run(c)
				})
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(handlerCase.method, handlerCase.path, nil))
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
				}
				var envelope struct {
					Code int `json:"code"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Code != http.StatusUnauthorized || strings.Count(recorder.Body.String(), `"code"`) != 1 {
					t.Fatalf("invalid 401 envelope: error = %v body = %s", err, recorder.Body.String())
				}
			})
		}
	}
}
