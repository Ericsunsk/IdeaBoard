package response

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code        int         `json:"code"`
	Msg         string      `json:"msg"`
	MessageSafe bool        `json:"message_safe"`
	Data        interface{} `json:"data,omitempty"`
}

// MachineErrorResponse carries a stable machine-readable error code for the
// narrow HTTP contracts that need callers to branch without matching copy.
// Generic responses retain the established numeric transport contract.
type MachineErrorResponse struct {
	Code        string `json:"code"`
	Msg         string `json:"msg"`
	MessageSafe bool   `json:"message_safe"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

// Error is a trust boundary: msg must be intentionally safe for public display.
// Unexpected I/O, database, provider, or wrapped errors must use InternalError.
func Error(c *gin.Context, code int, msg string) {
	c.JSON(code, Response{
		Code: code, Msg: msg, MessageSafe: true,
	})
}

func MachineError(c *gin.Context, httpStatus int, code, msg string) {
	c.JSON(httpStatus, MachineErrorResponse{Code: code, Msg: msg, MessageSafe: true})
}

// SuccessDTO is a typed variant of Success for callers that already have a
// struct with json tags. It avoids the implicit serialisation of gin.H.
func SuccessDTO[T any](c *gin.Context, data T) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

func InternalError(c *gin.Context, httpStatus int, publicMsg string, err error) {
	if err != nil {
		slog.Warn("HTTP error response", "message", publicMsg, "err", err)
	} else {
		slog.Warn("HTTP error response", "message", publicMsg)
	}
	Error(c, httpStatus, publicMsg)
}
