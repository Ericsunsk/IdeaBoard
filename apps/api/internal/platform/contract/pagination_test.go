package contract

import (
	"errors"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/platform/pagination"

	"github.com/gin-gonic/gin"
)

func TestLegacyLimitDialectMirrorsPaginationBounds(t *testing.T) {
	if DefaultLimit != pagination.DefaultLimit || MaxLimit != pagination.MaxLimit {
		t.Fatalf("limit bounds drifted: default=%d max=%d, want %d/%d",
			DefaultLimit, MaxLimit, pagination.DefaultLimit, pagination.MaxLimit)
	}
	for _, raw := range []string{"", "  "} {
		if got, err := ParseLimit(raw); err != nil || got != DefaultLimit {
			t.Fatalf("ParseLimit(%q) = %d, %v; want %d, nil", raw, got, err, DefaultLimit)
		}
	}
	if got, err := ParseLimit("200"); err != nil || got != 200 {
		t.Fatalf("ParseLimit(200) = %d, %v; want 200, nil", got, err)
	}
	for _, raw := range []string{"0", "-1", "201", "abc"} {
		if _, err := ParseLimit(raw); !errors.Is(err, pagination.ErrInvalidLimit) {
			t.Fatalf("ParseLimit(%q) error = %v, want ErrInvalidLimit", raw, err)
		}
	}
	if got, err := NormalizeLimit(0); err != nil || got != DefaultLimit {
		t.Fatalf("NormalizeLimit(0) = %d, %v; want %d, nil", got, err, DefaultLimit)
	}
}

func TestParseSearchLimitBounds(t *testing.T) {
	if DefaultSearchLimit != 24 || MaxSearchLimit != 48 {
		t.Fatalf("search bounds drifted: default=%d max=%d, want 24/48", DefaultSearchLimit, MaxSearchLimit)
	}
	if got, err := ParseSearchLimit(""); err != nil || got != DefaultSearchLimit {
		t.Fatalf("ParseSearchLimit(\"\") = %d, %v; want %d, nil", got, err, DefaultSearchLimit)
	}
	if got, err := ParseSearchLimit("48"); err != nil || got != 48 {
		t.Fatalf("ParseSearchLimit(48) = %d, %v; want 48, nil", got, err)
	}
	for _, raw := range []string{"0", "-1", "49", "abc"} {
		if _, err := ParseSearchLimit(raw); !errors.Is(err, ErrInvalidSearchLimit) {
			t.Fatalf("ParseSearchLimit(%q) error = %v, want ErrInvalidSearchLimit", raw, err)
		}
	}
}

func TestParseRewardLimitBounds(t *testing.T) {
	if DefaultRewardLimit != 20 || MaxRewardLimit != 100 {
		t.Fatalf("reward bounds drifted: default=%d max=%d, want 20/100", DefaultRewardLimit, MaxRewardLimit)
	}
	if got, err := ParseRewardLimit(""); err != nil || got != DefaultRewardLimit {
		t.Fatalf("ParseRewardLimit(\"\") = %d, %v; want %d, nil", got, err, DefaultRewardLimit)
	}
	if got, err := NormalizeRewardLimit(0); err != nil || got != DefaultRewardLimit {
		t.Fatalf("NormalizeRewardLimit(0) = %d, %v; want %d, nil", got, err, DefaultRewardLimit)
	}
	if _, err := NormalizeRewardLimit(101); !errors.Is(err, ErrInvalidRewardLimit) {
		t.Fatalf("NormalizeRewardLimit(101) error = %v, want ErrInvalidRewardLimit", err)
	}
	for _, raw := range []string{"0", "-1", "101", "abc"} {
		if _, err := ParseRewardLimit(raw); !errors.Is(err, ErrInvalidRewardLimit) {
			t.Fatalf("ParseRewardLimit(%q) error = %v, want ErrInvalidRewardLimit", raw, err)
		}
	}
}

func TestParseBeforeIDBounds(t *testing.T) {
	if got, err := ParseBeforeID(""); err != nil || got != 0 {
		t.Fatalf("ParseBeforeID(\"\") = %d, %v; want 0, nil", got, err)
	}
	if got, err := ParseBeforeID("42"); err != nil || got != 42 {
		t.Fatalf("ParseBeforeID(42) = %d, %v; want 42, nil", got, err)
	}
	for _, raw := range []string{"0", "-1", "abc", "1.5"} {
		if _, err := ParseBeforeID(raw); !errors.Is(err, ErrInvalidBeforeID) {
			t.Fatalf("ParseBeforeID(%q) error = %v, want ErrInvalidBeforeID", raw, err)
		}
	}
}

func TestGinQueryHelpersReadSingleParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(query string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
		return c
	}
	if got, err := PageSizeFromGin(newContext("page_size=20")); err != nil || got != 20 {
		t.Fatalf("PageSizeFromGin = %d, %v; want 20, nil", got, err)
	}
	if _, err := PageSizeFromGin(newContext("page_size=101")); !errors.Is(err, ErrInvalidPageSize) {
		t.Fatalf("PageSizeFromGin(101) error = %v, want ErrInvalidPageSize", err)
	}
	if got, err := LimitFromGin(newContext("")); err != nil || got != DefaultLimit {
		t.Fatalf("LimitFromGin(absent) = %d, %v; want %d, nil", got, err, DefaultLimit)
	}
	if got, err := SearchLimitFromGin(newContext("limit=10")); err != nil || got != 10 {
		t.Fatalf("SearchLimitFromGin = %d, %v; want 10, nil", got, err)
	}
	if got, err := RewardLimitFromGin(newContext("limit=50")); err != nil || got != 50 {
		t.Fatalf("RewardLimitFromGin = %d, %v; want 50, nil", got, err)
	}
	if got, err := BeforeIDFromGin(newContext("before_id=7")); err != nil || got != 7 {
		t.Fatalf("BeforeIDFromGin = %d, %v; want 7, nil", got, err)
	}
	if _, err := BeforeIDFromGin(newContext("before_id=0")); !errors.Is(err, ErrInvalidBeforeID) {
		t.Fatalf("BeforeIDFromGin(0) error = %v, want ErrInvalidBeforeID", err)
	}
	if LimitParam != "limit" || BeforeIDParam != "before_id" {
		t.Fatalf("query param names drifted: limit=%q before_id=%q", LimitParam, BeforeIDParam)
	}
}
