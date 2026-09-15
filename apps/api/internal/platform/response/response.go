package response

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader carries request correlation across logs and responses.
// The HTTP middleware aliases this shared wire-header constant.
const RequestIDHeader = "X-Request-ID"

// Response is the shared HTTP envelope.
//
// Contract:
//   - Success: Code 0, Msg "success"/"accepted", Data set.
//   - Error: Code mirrors the HTTP status, Msg is intentionally safe for
//     public display, MessageSafe is always true on the wire.
//   - RequestID echoes the request correlation ID when the request passed
//     through the global middleware; omitted otherwise (e.g. SSE events).
type Response struct {
	Code        int         `json:"code"`
	Msg         string      `json:"msg"`
	MessageSafe bool        `json:"message_safe"`
	RequestID   string      `json:"request_id,omitempty"`
	Data        interface{} `json:"data,omitempty"`
}

// MachineErrorResponse carries a stable machine-readable error code for the
// narrow HTTP contracts that need callers to branch without matching copy.
// Generic responses retain the established numeric transport contract.
type MachineErrorResponse struct {
	Code        string `json:"code"`
	Msg         string `json:"msg"`
	MessageSafe bool   `json:"message_safe"`
	RequestID   string `json:"request_id,omitempty"`
}

func requestIDOf(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(RequestIDHeader)
}

func Success(c *gin.Context, data interface{}) {
	SuccessDTO(c, data)
}

// Error is a trust boundary: msg must be intentionally safe for public display.
// Unexpected I/O, database, provider, or wrapped errors must use InternalError.
func Error(c *gin.Context, code int, msg string) {
	c.JSON(code, Response{
		Code: code, Msg: msg, MessageSafe: true,
		RequestID: requestIDOf(c),
	})
}

func MachineError(c *gin.Context, httpStatus int, code, msg string) {
	c.JSON(httpStatus, MachineErrorResponse{Code: code, Msg: msg, MessageSafe: true, RequestID: requestIDOf(c)})
}

// SuccessDTO is a typed variant of Success for callers that already have a
// struct with json tags. It avoids the implicit serialisation of gin.H.
func SuccessDTO[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, Response{
		Code:      0,
		Msg:       "success",
		RequestID: requestIDOf(c),
		Data:      data,
	})
}

func InternalError(c *gin.Context, httpStatus int, publicMsg string, err error) {
	attrs := []any{"message", publicMsg, "status", httpStatus, "request_id", requestIDOf(c)}
	if c != nil && c.Request != nil {
		attrs = append(attrs, "method", c.Request.Method, "path", c.Request.URL.Path)
	}
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	// Error level: these are unexpected server-side failures needing triage,
	// not caller mistakes (those go through Error with 4xx).
	slog.Error("HTTP error response", attrs...)
	Error(c, httpStatus, publicMsg)
}
