package library

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func newLibraryTestRouter(t testing.TB, handler *Handler) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleUser)
		c.Next()
	})
	router.GET("/assets", handler.ListAssets)
	router.POST("/assets", handler.SaveAsset)
	return router
}

func TestListAssetsHandlerAcceptsCanonicalFolderSelectors(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	folder, err := service.CreateFolder(t.Context(), 1, "Travel")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	router := newLibraryTestRouter(t, NewHandler(service))

	for _, selector := range []string{"all", "quick-saves", ""} {
		t.Run("selector="+selector, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			target := "/assets"
			if selector != "" {
				target += "?folder_id=" + selector
			}
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets?folder_id="+stringID(folder.ID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("user folder status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestListAssetsHandlerRejectsInvalidFolderSelector(t *testing.T) {
	router := newLibraryTestRouter(t, NewHandler(nil))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets?folder_id=not-a-folder", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveAssetHandlerUsesLibraryFolderSelection(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	folder, err := service.CreateFolder(t.Context(), 1, "Travel")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	router := newLibraryTestRouter(t, NewHandler(service))
	payload := `{"folder_id":` + stringID(folder.ID) + `,"asset":{"provider":"unsplash","source_id":"handler-asset","source_url":"https://unsplash.com/photos/handler-asset","preview_url":"https://images.unsplash.com/handler-asset","original_url":"https://images.unsplash.com/handler-asset"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveAssetHandlerDefaultsToQuickSaves(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewRepository(db))
	router := newLibraryTestRouter(t, NewHandler(service))
	payload := `{"asset":{"provider":"pexels","source_id":"quick-handler-asset","source_url":"https://www.pexels.com/photo/quick-handler-asset","preview_url":"https://images.pexels.com/quick-handler-asset","original_url":"https://images.pexels.com/quick-handler-asset"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestSaveAssetRequestJSONBinding(t *testing.T) {
	raw := `{"folder_id":12,"new_folder_name":"Inspire","asset":{"provider":"unsplash","source_id":"photo-abc","source_url":"https://unsplash.com/photos/abc","preview_url":"https://images.unsplash.com/abc","original_url":"https://images.unsplash.com/abc-raw","title":"A title","author":"Author","width":1000,"height":800}}`
	var req saveAssetRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.Asset.Provider != "unsplash" || req.Asset.SourceID != "photo-abc" || req.Asset.SourceURL != "https://unsplash.com/photos/abc" || req.Asset.PreviewURL != "https://images.unsplash.com/abc" || req.Asset.OriginalURL != "https://images.unsplash.com/abc-raw" {
		t.Fatalf("unmarshaled asset = %#v", req.Asset)
	}
}

func stringID(id int64) string {
	return strconv.FormatInt(id, 10)
}
