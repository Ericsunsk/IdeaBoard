package assets

import (
	"strconv"

	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func parseIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		response.Error(c, 400, "参数错误")
		return false
	}
	return true
}
