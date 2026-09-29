package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// PageMeta is the required pagination meta for list responses
// (Baseline 5.5): {"next_cursor": "...", "has_more": true}.
type PageMeta struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// SuccessEnvelope is the normal JSON success shape (Baseline 5.2):
// {"data": ..., "meta"?}. Meta is omitted for single-resource responses
// and required for paginated responses (use RenderPaginated).
type SuccessEnvelope[T any] struct {
	Data T         `json:"data"`
	Meta *PageMeta `json:"meta,omitempty"`
}

// ErrorDetail is the machine-readable error body (Baseline 5.3).
// Details carries field-level validation failures as {"fields": {...}}.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorEnvelope is the error shape: {"error": {...}} (Baseline 5.3).
type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

// FieldErrors carries field-level validation failures for Details
// (Baseline 5.3: details.fields).
type FieldErrors struct {
	Fields map[string]string `json:"fields,omitempty"`
}

// requestIDOf resolves the correlation ID: contract Request-Id first,
// legacy X-Request-ID fallback (set by the global RequestIDMiddleware).
func requestIDOf(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v := c.GetString(RequestIDHeader); v != "" {
		return v
	}
	if v := c.GetString(response.RequestIDHeader); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.GetHeader(RequestIDHeader)); v != "" {
		return v
	}
	return strings.TrimSpace(c.GetHeader(response.RequestIDHeader))
}

// RenderSuccess renders 200 with the success envelope (Baseline 5.4).
func RenderSuccess[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, SuccessEnvelope[T]{Data: data})
}

// RenderCreated renders 201 for synchronous resource creation
// (Baseline 5.4).
func RenderCreated[T any](c *gin.Context, data T) {
	c.JSON(http.StatusCreated, SuccessEnvelope[T]{Data: data})
}

// RenderAccepted renders 202 for asynchronous task/job creation
// (Baseline 5.4).
func RenderAccepted[T any](c *gin.Context, data T) {
	c.JSON(http.StatusAccepted, SuccessEnvelope[T]{Data: data})
}

// RenderPaginated renders 200 with the required pagination meta
// (Baseline 5.5).
func RenderPaginated[T any](c *gin.Context, data T, meta PageMeta) {
	metaCopy := meta
	c.JSON(http.StatusOK, SuccessEnvelope[T]{Data: data, Meta: &metaCopy})
}

// RenderPaginatedWithMeta renders 200 with a caller-supplied meta
// payload under the unified {data, meta} envelope (Baseline 5.5 shape,
// extended meta for endpoints whose list meta carries more than the
// cursor, e.g. control task status counts). Prefer RenderPaginated;
// use this only when PageMeta cannot carry the endpoint's meta.
func RenderPaginatedWithMeta[T, M any](c *gin.Context, data T, meta M) {
	c.JSON(http.StatusOK, struct {
		Data T `json:"data"`
		Meta M `json:"meta"`
	}{Data: data, Meta: meta})
}

// RenderError renders the error envelope with an explicit status.
func RenderError(c *gin.Context, status int, code Code, message string) {
	RenderErrorWithDetails(c, status, code, message, nil)
}

// RenderErrorWithDetails renders the error envelope with details
// (e.g. FieldErrors for validation failures).
func RenderErrorWithDetails(c *gin.Context, status int, code Code, message string, details any) {
	c.JSON(status, ErrorEnvelope{
		Error: ErrorDetail{
			Code:      string(code),
			Message:   message,
			Details:   details,
			RequestID: requestIDOf(c),
		},
	})
}

// RenderCode renders a kernel code using the StatusOf mapping, so callers
// cannot mismatch code and HTTP status.
func RenderCode(c *gin.Context, code Code, message string) {
	RenderError(c, StatusOf(code), code, message)
}

// ErrTrailingPayload reports a second JSON value after the document body.
var ErrTrailingPayload = errors.New("contract: unexpected trailing JSON payload")

// DecodeStrict unmarshals data while rejecting unknown fields (Baseline
// 5.1: unknown fields are rejected by default; additionalProperties only
// where the contract defines an extensible map) and trailing payloads.
// Callers render failures as 400 (CodeBadRequest) with details.fields
// describing the offending fields.
func DecodeStrict(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ErrTrailingPayload
		}
		return err
	}
	return nil
}

// BindStrictJSON binds the request body with unknown fields rejected
// (Baseline 5.1 default). It reads the body as-is; the global request
// body-limit middleware still bounds its size.
func BindStrictJSON(c *gin.Context, dst any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return errors.New("contract: missing request body")
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return DecodeStrict(raw, dst)
}
