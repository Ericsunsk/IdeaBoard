package response

import (
	"log"
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

func ErrorWithStatus(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, Response{
		Code: code,
		Msg:  msg,
	})
}

func InternalError(c *gin.Context, httpStatus int, publicMsg string, err error) {
	if err != nil {
		log.Printf("[HTTP] %s: %v", publicMsg, err)
	} else {
		log.Printf("[HTTP] %s", publicMsg)
	}
	Error(c, httpStatus, publicMsg)
}
