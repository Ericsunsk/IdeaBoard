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

func TestHandlerGetCurrentUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &usageReaderStub{result: Usage{
		Timezone: ShanghaiTimezone,
		Lifetime: LifetimeSummary{TotalCredits: 12},
		Trend:    usageTrendForRange(Range7Days, 7),
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(service).GetCurrent)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage?range=7d&user_id=99", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.userID != 42 || service.usageRange != Range7Days || !strings.Contains(recorder.Body.String(), `"total_credits":12`) ||
		!strings.Contains(recorder.Body.String(), `"trend":`) ||
		!strings.Contains(recorder.Body.String(), `"credits":7`) {
		t.Fatalf("user id=%d body=%s", service.userID, recorder.Body.String())
	}
}

func TestHandlerGetCurrentDefaultsToThirtyDays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &usageReaderStub{result: Usage{
		Timezone: ShanghaiTimezone,
		Trend:    usageTrendForRange(Range30Days, 0),
	}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(service).GetCurrent)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.usageRange != Range30Days {
		t.Fatalf("range = %q, want %q", service.usageRange, Range30Days)
	}
}

func TestHandlerGetCurrentRejectsUnsupportedRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &usageReaderStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(service).GetCurrent)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage?range=90d", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 0 {
		t.Fatalf("service calls = %d, want 0", service.calls)
	}
	if !strings.Contains(recorder.Body.String(), "7d") || !strings.Contains(recorder.Body.String(), "30d") || !strings.Contains(recorder.Body.String(), "1y") {
		t.Fatalf("validation response does not list supported ranges: %s", recorder.Body.String())
	}
}

func TestHandlerGetCurrentReturnsRetryableInlineFailureStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(&usageReaderStub{err: errors.New("database unavailable")}).GetCurrent)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerGetCurrentRejectsUnsafeNumbers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/usage", NewHandler(&usageReaderStub{result: Usage{
		Timezone: ShanghaiTimezone,
		Trend:    []DailyUsage{{Date: "2026-07-13", Credits: MaxSafeInteger + 1}},
	}}).GetCurrent)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/usage", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "9007199254740992") {
		t.Fatalf("unsafe number leaked through API: %s", recorder.Body.String())
	}
}

type usageReaderStub struct {
	result     Usage
	err        error
	userID     int64
	usageRange Range
	calls      int
}

func (s *usageReaderStub) GetCurrent(_ context.Context, userID int64, usageRange Range) (Usage, error) {
	s.calls++
	s.userID = userID
	s.usageRange = usageRange
	return s.result, s.err
}

func usageTrendForRange(usageRange Range, lastDayCredits int64) []DailyUsage {
	trend := make([]DailyUsage, usageRange.Days())
	for index := range trend {
		trend[index] = DailyUsage{Date: "2026-07-13", Credits: 0}
	}
	trend[len(trend)-1].Credits = lastDayCredits
	return trend
}
