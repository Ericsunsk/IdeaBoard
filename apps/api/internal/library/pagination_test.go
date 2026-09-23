package library

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/pagination"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestLibraryPaginationLimitTracksSharedStandard(t *testing.T) {
	if DefaultPageSize != pagination.DefaultLimit {
		t.Fatalf("DefaultPageSize = %d, want shared %d", DefaultPageSize, pagination.DefaultLimit)
	}
	if MaxPageSize != pagination.MaxLimit {
		t.Fatalf("MaxPageSize = %d, want shared %d", MaxPageSize, pagination.MaxLimit)
	}
	if got, err := parseLimit(""); err != nil || got != pagination.DefaultLimit {
		t.Fatalf("parseLimit(\"\") = %d, %v, want %d, nil", got, err, pagination.DefaultLimit)
	}
	if got, err := parseLimit("20"); err != nil || got != 20 {
		t.Fatalf("parseLimit(20) = %d, %v, want 20, nil", got, err)
	}
	for _, raw := range []string{"0", "-1", "201", "bad", "1.5"} {
		if _, err := parseLimit(raw); err != ErrInvalidPageSize {
			t.Fatalf("parseLimit(%q) error = %v, want ErrInvalidPageSize", raw, err)
		}
	}
}

func TestLibraryCursorRoundTripThroughSharedCodec(t *testing.T) {
	want := Cursor{CreatedAt: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: 42}
	raw, err := encodeLibraryCursor(want)
	if err != nil {
		t.Fatalf("encodeLibraryCursor returned error: %v", err)
	}
	got, err := decodeLibraryCursor(raw)
	if err != nil {
		t.Fatalf("decodeLibraryCursor returned error: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
	if decoded, err := decodeLibraryCursor(""); err != nil || decoded != nil {
		t.Fatalf("decodeLibraryCursor(\"\") = %#v, %v, want nil, nil", decoded, err)
	}
}

func TestLibraryCursorRejectsInvalidInput(t *testing.T) {
	oversized := strings.Repeat("a", pagination.MaxCursorLen+1)
	for _, raw := range []string{"not-base64", "e30", oversized} {
		if _, err := decodeLibraryCursor(raw); err != ErrInvalidCursor {
			t.Fatalf("decodeLibraryCursor(%q) error = %v, want ErrInvalidCursor", raw, err)
		}
	}
	// Unknown fields and trailing payloads must not smuggle semantics.
	unknown := base64.RawURLEncoding.EncodeToString([]byte(`{"CreatedAt":"2026-08-20T12:00:00Z","ID":1,"extra":1}`))
	if _, err := decodeLibraryCursor(unknown); err != ErrInvalidCursor {
		t.Fatalf("decodeLibraryCursor(unknown fields) error = %v, want ErrInvalidCursor", err)
	}
	valid, err := encodeLibraryCursor(Cursor{CreatedAt: time.Now().UTC(), ID: 7})
	if err != nil {
		t.Fatalf("encodeLibraryCursor returned error: %v", err)
	}
	trailingRaw, _ := base64.RawURLEncoding.DecodeString(valid)
	trailing := base64.RawURLEncoding.EncodeToString(append(trailingRaw, []byte(" {}")...))
	if _, err := decodeLibraryCursor(trailing); err != ErrInvalidCursor {
		t.Fatalf("decodeLibraryCursor(trailing payload) error = %v, want ErrInvalidCursor", err)
	}
	for _, cursor := range []Cursor{{}, {ID: 1}, {CreatedAt: time.Now()}} {
		if _, err := encodeLibraryCursor(cursor); err != ErrInvalidCursor {
			t.Fatalf("encodeLibraryCursor(%#v) error = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

func TestListAssetsHandlerRejectsInvalidPaginationWithStableCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := NewService(nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 1, principal.RoleUser)
		c.Next()
	})
	router.GET("/assets", NewHandler(service).ListAssets)

	for _, testCase := range []struct {
		name     string
		target   string
		wantCode string
	}{
		{name: "limit zero", target: "/assets?limit=0", wantCode: "library_page_size_invalid"},
		{name: "limit over max", target: "/assets?limit=201", wantCode: "library_page_size_invalid"},
		{name: "limit malformed", target: "/assets?limit=bad", wantCode: "library_page_size_invalid"},
		{name: "cursor malformed", target: "/assets?cursor=bad", wantCode: "library_cursor_invalid"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.target, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusBadRequest)
			}
			var machineError struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &machineError); err != nil {
				t.Fatalf("decode error response: %v body=%s", err, recorder.Body.String())
			}
			if machineError.Code != testCase.wantCode {
				t.Fatalf("code = %q, want %q body=%s", machineError.Code, testCase.wantCode, recorder.Body.String())
			}
		})
	}
}
