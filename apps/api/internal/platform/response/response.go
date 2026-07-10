package response

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: data,
	})
}

func Error(c *gin.Context, code int, msg string) {
	c.JSON(code, Response{
		Code: code,
		Msg:  msg,
	})
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
