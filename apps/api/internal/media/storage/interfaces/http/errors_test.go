package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubUserMessageError struct{ message string }

func (e stubUserMessageError) Error() string       { return "driver probe failed: " + e.message }
func (e stubUserMessageError) UserMessage() string { return e.message }

type wrappedUserMessageError struct{ cause error }

func (e wrappedUserMessageError) Error() string { return "wrapped: " + e.cause.Error() }
func (e wrappedUserMessageError) Unwrap() error { return e.cause }

func assertStorageMachineError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode, wantMsg string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != wantCode {
		t.Fatalf("code = %q, want %q body=%s", body.Code, wantCode, recorder.Body.String())
	}
	if body.Msg != wantMsg {
		t.Fatalf("msg = %q, want %q", body.Msg, wantMsg)
	}
}

func TestStorageInputErrorRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) { writeStorageInputError(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	assertStorageMachineError(t, recorder, http.StatusBadRequest, "storage_invalid_input", "参数错误")
}

func TestStorageUnavailableRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) { writeStorageUnavailable(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	assertStorageMachineError(t, recorder, http.StatusServiceUnavailable, "storage_service_unavailable", "存储服务暂不可用")
}

func TestStorageCommandErrorKeepsCuratedUserMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		writeStorageCommandError(c, "保存失败，请检查配置或稍后重试",
			wrappedUserMessageError{cause: stubUserMessageError{message: "S3 凭证校验失败：请检查 Endpoint"}})
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	assertStorageMachineError(t, recorder, http.StatusBadRequest, "storage_profile_invalid", "S3 凭证校验失败：请检查 Endpoint")
}

func TestStorageCommandErrorFallsBackToLoggedInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		writeStorageCommandError(c, "连接测试失败，请检查配置或稍后重试", errors.New("boom"))
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	assertStorageMachineError(t, recorder, http.StatusInternalServerError, "storage_operation_failed", "连接测试失败，请检查配置或稍后重试")
}

func TestStorageListErrorRendersMachineCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/err", func(c *gin.Context) { writeStorageListError(c, errors.New("boom")) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/err", nil))
	assertStorageMachineError(t, recorder, http.StatusInternalServerError, "storage_operation_failed", "获取存储列表失败")
}
