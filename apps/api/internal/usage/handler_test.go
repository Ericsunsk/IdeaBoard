package usage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerGetCurrentLifetimeUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &lifetimeReaderStub{result: LifetimeUsage{Timezone: ShanghaiTimezone, Lifetime: LifetimeSummary{TotalCredits: 12}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(service).GetCurrentLifetime)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage?user_id=99", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.userID != 42 || !strings.Contains(recorder.Body.String(), `"total_credits":12`) {
		t.Fatalf("user id=%d body=%s", service.userID, recorder.Body.String())
	}
}

func TestHandlerGetCurrentLifetimeReturnsRetryableInlineFailureStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(&lifetimeReaderStub{err: errors.New("database unavailable")}).GetCurrentLifetime)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerGetCurrentLifetimeRejectsUnsafeNumbers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(&lifetimeReaderStub{result: LifetimeUsage{
		Timezone: ShanghaiTimezone,
		Lifetime: LifetimeSummary{TaskCount: MaxSafeInteger + 1},
	}}).GetCurrentLifetime)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "9007199254740992") {
		t.Fatalf("unsafe number leaked through API: %s", recorder.Body.String())
	}
}

type lifetimeReaderStub struct {
	result LifetimeUsage
	err    error
	userID int64
}

func (s *lifetimeReaderStub) GetLifetime(_ context.Context, userID int64) (LifetimeUsage, error) {
	s.userID = userID
	return s.result, s.err
}
