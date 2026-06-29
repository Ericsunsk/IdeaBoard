package assets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockLineDraftRepo struct {
	listFn    func(ctx context.Context) ([]*LineDraftMode, error)
	enabledFn func(ctx context.Context) ([]*LineDraftMode, error)
	createFn  func(ctx context.Context, m *LineDraftMode) error
	getByIDFn func(ctx context.Context, id int64) (*LineDraftMode, error)
	updateFn  func(ctx context.Context, m *LineDraftMode) error
	deleteFn  func(ctx context.Context, id int64) error
}

func (m *mockLineDraftRepo) List(ctx context.Context) ([]*LineDraftMode, error) {
	if m.listFn == nil {
		return nil, nil
	}
	return m.listFn(ctx)
}
func (m *mockLineDraftRepo) ListEnabled(ctx context.Context) ([]*LineDraftMode, error) {
	if m.enabledFn == nil {
		return nil, nil
	}
	return m.enabledFn(ctx)
}
func (m *mockLineDraftRepo) Create(ctx context.Context, mode *LineDraftMode) error {
	if m.createFn == nil {
		return nil
	}
	return m.createFn(ctx, mode)
}
func (m *mockLineDraftRepo) GetByID(ctx context.Context, id int64) (*LineDraftMode, error) {
	if m.getByIDFn == nil {
		return nil, nil
	}
	return m.getByIDFn(ctx, id)
}
func (m *mockLineDraftRepo) Update(ctx context.Context, mode *LineDraftMode) error {
	if m.updateFn == nil {
		return nil
	}
	return m.updateFn(ctx, mode)
}
func (m *mockLineDraftRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn == nil {
		return nil
	}
	return m.deleteFn(ctx, id)
}

func TestListEnabledHidesPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockLineDraftRepo{
		enabledFn: func(ctx context.Context) ([]*LineDraftMode, error) {
			return []*LineDraftMode{
				{ID: 1, Name: "草稿", CoverImage: "/img/draft.png", Prompt: "secret prompt"},
			}, nil
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.GET("/drafts", handler.ListEnabled)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/drafts", nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID         int64  `json:"id"`
				Name       string `json:"name"`
				CoverImage string `json:"cover_image"`
				Prompt     string `json:"prompt"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data.List) != 1 {
		t.Fatalf("list len=%d", len(body.Data.List))
	}
	if body.Data.List[0].Prompt != "" {
		t.Fatal("prompt should be hidden from user view")
	}
}

func TestListReturnsAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockLineDraftRepo{
		listFn: func(ctx context.Context) ([]*LineDraftMode, error) {
			return []*LineDraftMode{
				{ID: 1, Name: "草稿", Prompt: "secret"},
			}, nil
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.GET("/drafts", handler.List)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/drafts", nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	var body struct {
		Data struct {
			List []struct {
				Prompt string `json:"prompt"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(res.Body.Bytes(), &body)
	if len(body.Data.List) != 1 || body.Data.List[0].Prompt != "secret" {
		t.Fatal("admin list should include prompt")
	}
}

func TestListEnabledDBError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockLineDraftRepo{
		enabledFn: func(ctx context.Context) ([]*LineDraftMode, error) {
			return nil, errors.New("db down")
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.GET("/drafts", handler.ListEnabled)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/drafts", nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(res.Body.String(), "获取线稿模式失败") {
		t.Fatal("expected error message")
	}
}

func TestUpdateNonexistentReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockLineDraftRepo{
		getByIDFn: func(ctx context.Context, id int64) (*LineDraftMode, error) {
			return nil, errors.New("not found")
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.PUT("/drafts/:id", handler.Update)

	body := strings.NewReader(`{"name":"x","prompt":"p"}`)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/drafts/999", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusNotFound)
	}
	if !strings.Contains(res.Body.String(), "模式不存在") {
		t.Fatal("expected not found message")
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var deletedID int64
	repo := &mockLineDraftRepo{
		deleteFn: func(ctx context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.DELETE("/drafts/:id", handler.Delete)

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/drafts/42", nil)
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
	if deletedID != 42 {
		t.Fatalf("deletedID=%d", deletedID)
	}
}

func TestCreateRequiresName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mockLineDraftRepo{
		createFn: func(ctx context.Context, m *LineDraftMode) error {
			return nil
		},
	}
	handler := NewLineDraftHandler(repo)
	router := gin.New()
	router.POST("/drafts", handler.Create)

	body := strings.NewReader(`{"prompt":"p"}`) // missing name
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/drafts", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(res, req)

	if !strings.Contains(res.Body.String(), "参数错误") {
		t.Fatal("should reject missing required field")
	}
}
