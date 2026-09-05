package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/platform/outbound"

	"github.com/gin-gonic/gin"
)

func TestProviderEndpointCreateRejectsLocalhostAPIURL(t *testing.T) {
	setProviderEndpointTestAllowHosts(t)
	if _, err := outbound.ValidateConfiguredEndpoint("http://127.0.0.1:8080/v1"); err == nil {
		t.Fatal("expected localhost base_url to be rejected")
	}
}

func TestRespondProviderEndpointConflict(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "duplicate name",
			err:     fmt.Errorf("wrapped: %w", ErrNameConflict),
			wantMsg: "ProviderEndpoint 名称已存在，请使用其他名称",
		},
		{
			name:    "duplicate api type and base url",
			err:     fmt.Errorf("wrapped: %w", ErrURLConflict),
			wantMsg: "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			if !respondProviderEndpointConflict(c, tt.err) {
				t.Fatal("respondProviderEndpointConflict returned false")
			}
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
	}
}

func TestProviderEndpointHandlerCreateDuplicateReturnsConflict(t *testing.T) {
	tests := []struct {
		name    string
		seed    *Endpoint
		request CreateEndpointRequest
		wantMsg string
	}{
		{
			name: "duplicate name",
			seed: &Endpoint{
				APIType: "openai-compatible",
				Name:    "Duplicate Create",
				BaseURL: "http://127.0.0.1:19081/v1",
				Status:  1,
			},
			request: CreateEndpointRequest{
				APIType:  "openai-compatible",
				Protocol: ProtocolModelAPI,
				Name:     "duplicate create",
				BaseURL:  "http://127.0.0.1:19082/v1",
				Status:   1,
			},
			wantMsg: "ProviderEndpoint 名称已存在，请使用其他名称",
		},
		{
			name: "duplicate api type and base url",
			seed: &Endpoint{
				APIType: "openai-compatible",
				Name:    "Duplicate URL A",
				BaseURL: "http://127.0.0.1:19083/v1",
				Status:  1,
			},
			request: CreateEndpointRequest{
				APIType:  "openai-compatible",
				Protocol: ProtocolModelAPI,
				Name:     "Duplicate URL B",
				BaseURL:  "http://127.0.0.1:19083/v1",
				Status:   1,
			},
			wantMsg: "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, repo := newProviderEndpointHandlerTestRouter(t)
			seedProviderEndpoint(t, context.Background(), repo, tt.seed)

			rec := serveProviderEndpointJSON(t, router, http.MethodPost, "/provider-endpoints", tt.request)
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
	}
}

func TestProviderEndpointHandlerUpdateDuplicateReturnsConflict(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(first *Endpoint, second *Endpoint) UpdateEndpointRequest
		wantMsg   string
		firstURL  string
		secondURL string
	}{
		{
			name: "duplicate name",
			mutate: func(first *Endpoint, second *Endpoint) UpdateEndpointRequest {
				return UpdateEndpointRequest{
					APIType:  second.APIType,
					Protocol: second.Protocol,
					Name:     first.Name,
					BaseURL:  second.BaseURL,
					Status:   second.Status,
				}
			},
			wantMsg:   "ProviderEndpoint 名称已存在，请使用其他名称",
			firstURL:  "http://127.0.0.1:19084/v1",
			secondURL: "http://127.0.0.1:19085/v1",
		},
		{
			name: "duplicate api type and base url",
			mutate: func(first *Endpoint, second *Endpoint) UpdateEndpointRequest {
				return UpdateEndpointRequest{
					APIType:  second.APIType,
					Protocol: second.Protocol,
					Name:     second.Name,
					BaseURL:  first.BaseURL,
					Status:   second.Status,
				}
			},
			wantMsg:   "同一接口类型下已存在相同 Base URL 的 ProviderEndpoint",
			firstURL:  "http://127.0.0.1:19086/v1",
			secondURL: "http://127.0.0.1:19087/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, repo := newProviderEndpointHandlerTestRouter(t)
			first := seedProviderEndpoint(t, context.Background(), repo, &Endpoint{
				APIType: "openai-compatible",
				Name:    "Update First " + tt.name,
				BaseURL: tt.firstURL,
				Status:  1,
			})
			second := seedProviderEndpoint(t, context.Background(), repo, &Endpoint{
				APIType: "openai-compatible",
				Name:    "Update Second " + tt.name,
				BaseURL: tt.secondURL,
				Status:  1,
			})

			target := fmt.Sprintf("/provider-endpoints/%d", second.ID)
			rec := serveProviderEndpointJSON(t, router, http.MethodPut, target, tt.mutate(first, second))
			assertProviderEndpointConflictResponse(t, rec, tt.wantMsg)
		})
	}
}

func TestProviderEndpointHandlerUpdateStatusValidatesInput(t *testing.T) {
	router, repo := newProviderEndpointHandlerTestRouter(t)
	endpoint := seedProviderEndpoint(t, context.Background(), repo, &Endpoint{
		APIType: "openai-compatible",
		Name:    "Status validation",
		BaseURL: "http://127.0.0.1:19088/v1",
		Status:  1,
	})

	for _, payload := range []any{map[string]any{}, map[string]any{"status": 2}} {
		rec := serveProviderEndpointJSON(t, router, http.MethodPatch, fmt.Sprintf("/provider-endpoints/%d/status", endpoint.ID), payload)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status update response = %d, want 400; body=%s", rec.Code, rec.Body.String())
		}
	}
}

func newProviderEndpointHandlerTestRouter(t *testing.T) (*gin.Engine, *Repository) {
	t.Helper()
	setProviderEndpointTestAllowHosts(t, "127.0.0.1")
	db := newProviderEndpointRepositoryTestDB(t)
	repo := NewRepository(db)
	handler := NewHandler(repo, "production")
	router := gin.New()
	router.POST("/provider-endpoints", handler.Create)
	router.PUT("/provider-endpoints/:id", handler.Update)
	router.PATCH("/provider-endpoints/:id/status", handler.UpdateStatus)
	return router, repo
}

func serveProviderEndpointJSON(t *testing.T, router http.Handler, method string, target string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertProviderEndpointConflictResponse(t *testing.T, rec *httptest.ResponseRecorder, wantMsg string) {
	t.Helper()
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response failed: %v; body=%s", err, rec.Body.String())
	}
	if body.Code != http.StatusConflict {
		t.Fatalf("response code = %d, want 409", body.Code)
	}
	if body.Msg != wantMsg {
		t.Fatalf("response msg = %q, want %q", body.Msg, wantMsg)
	}
}
