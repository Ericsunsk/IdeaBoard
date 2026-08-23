package adminbranding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeStore struct {
	overrides   []Override
	upserted    Override
	deleted     bool
	err         error
	listCalls   int
	upsertCalls int
	deleteCalls int
}

func (s *fakeStore) List(context.Context) ([]Override, error) {
	s.listCalls++
	return s.overrides, s.err
}
func (s *fakeStore) Upsert(_ context.Context, scope Scope, targetKey, logoKey string) (Override, error) {
	s.upsertCalls++
	s.upserted = Override{Scope: scope, TargetKey: targetKey, LogoKey: logoKey}
	if s.err != nil {
		return Override{}, s.err
	}
	return s.upserted, nil
}
func (s *fakeStore) Delete(context.Context, Scope, string) (bool, error) {
	s.deleteCalls++
	return s.deleted, s.err
}

func TestHandlerRequiresAdminForEveryOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			store := &fakeStore{}
			response := serveHandler(t, method, "user", store, `{"logo_key":"flux"}`)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status=%d, want %d", response.Code, http.StatusForbidden)
			}
			if store.listCalls != 0 || store.upsertCalls != 0 || store.deleteCalls != 0 {
				t.Fatal("store must not be called without admin authority")
			}
		})
	}
}

func TestHandlerListsAndMutatesValidatedOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	store := &fakeStore{overrides: []Override{{Scope: ScopeStorageProfile, TargetKey: "r2-main", LogoKey: "cloudflare-r2", CreatedAt: now, UpdatedAt: now}}}
	list := serveHandler(t, http.MethodGet, "admin", store, "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"scope":"storage-profile"`) {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}
	update := serveHandler(t, http.MethodPut, "admin", store, `{"logo_key":"flux"}`)
	if update.Code != http.StatusOK || store.upserted != (Override{Scope: ScopeProviderEndpoint, TargetKey: "42", LogoKey: "flux"}) {
		t.Fatalf("update response=%d override=%+v", update.Code, store.upserted)
	}
	remove := serveHandler(t, http.MethodDelete, "admin", store, "")
	if remove.Code != http.StatusOK {
		t.Fatalf("delete response = %d", remove.Code)
	}
}

func TestHandlerRejectsUnsupportedScopeLogoAndTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name string
		path string
		body string
	}{
		{name: "scope", path: "/unsupported/42", body: `{"logo_key":"flux"}`},
		{name: "target", path: "/provider-endpoint/invalid%20target", body: `{"logo_key":"flux"}`},
		{name: "logo", path: "/provider-endpoint/42", body: `{"logo_key":"not-real"}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &fakeStore{}
			response := serveHandlerAt(t, http.MethodPut, testCase.path, "admin", store, testCase.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func serveHandler(t *testing.T, method, role string, store Store, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveHandlerAt(t, method, "/provider-endpoint/42", role, store, body)
}

func serveHandlerAt(t *testing.T, method, path, role string, store Store, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	context.Params = gin.Params{{Key: "scope", Value: parts[0]}, {Key: "target_key", Value: parts[1]}}
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	if role != "" {
		identity, err := principal.New(1, uuid.New(), "auth-admin", "Test Admin", role)
		if err != nil {
			t.Fatal(err)
		}
		if err := principal.Set(context, identity); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(store)
	switch method {
	case http.MethodGet:
		handler.List(context)
	case http.MethodPut:
		handler.Upsert(context)
	case http.MethodDelete:
		handler.Delete(context)
	}
	return recorder
}

func TestValidateRejectsUnknownValues(t *testing.T) {
	if err := Validate(ScopeProviderEndpoint, "endpoint", "not-real"); err == nil {
		t.Fatal("expected unknown logo rejection")
	}
	if err := ValidateTarget(Scope("other"), "endpoint"); err == nil {
		t.Fatal("expected unknown scope rejection")
	}
}
