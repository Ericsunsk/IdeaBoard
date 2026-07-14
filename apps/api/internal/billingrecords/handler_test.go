package billingrecords

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandlerListUsesAuthenticatedUserAndDefaultPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &readerStub{page: Page{Records: []Record{{
		RecordID:              "credit_42",
		OccurredAt:            time.Date(2026, 7, 14, 8, 30, 0, 0, time.UTC),
		Kind:                  KindReward,
		Title:                 "欢迎奖励",
		CreditDelta:           100,
		AvailableCreditsAfter: 100,
	}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Next()
	})
	router.GET("/records", NewHandler(reader).List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/records?user_id=99", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.input != (ListInput{UserID: 42, Limit: DefaultPageLimit}) {
		t.Fatalf("input=%+v", reader.input)
	}
	for _, expected := range []string{
		`"record_id":"credit_42"`,
		`"kind":"reward"`,
		`"title":"欢迎奖励"`,
		`"credit_delta":100`,
		`"available_credits_after":100`,
	} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("body=%s, want %s", recorder.Body.String(), expected)
		}
	}
}

func TestHandlerListParsesCursorAndLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &readerStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", int64(7))
		c.Next()
	})
	router.GET("/records", NewHandler(reader).List)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/records?before_sequence=91&limit=35", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.input != (ListInput{UserID: 7, BeforeSequence: 91, Limit: 35}) {
		t.Fatalf("input=%+v", reader.input)
	}
}

func TestHandlerListRejectsInvalidPaginationWithoutQuerying(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{
		"before_sequence=0",
		"before_sequence=bad",
		"limit=0",
		"limit=201",
		"limit=bad",
	} {
		t.Run(query, func(t *testing.T) {
			reader := &readerStub{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("user_id", int64(7))
				c.Next()
			})
			router.GET("/records", NewHandler(reader).List)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/records?"+query, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if reader.calls != 0 {
				t.Fatalf("reader calls=%d", reader.calls)
			}
		})
	}
}

func TestHandlerListRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &readerStub{}
	router := gin.New()
	router.GET("/records", NewHandler(reader).List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/records", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.calls != 0 {
		t.Fatalf("reader calls=%d", reader.calls)
	}
}

type readerStub struct {
	page  Page
	err   error
	input ListInput
	calls int
}

func (s *readerStub) List(_ context.Context, input ListInput) (Page, error) {
	s.calls++
	s.input = input
	return s.page, s.err
}
