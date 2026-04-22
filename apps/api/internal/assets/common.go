package assets

import (
	"strconv"

	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

const (
	defaultPage     = 1
	defaultPageSize = 50
	defaultMaxSize  = 100
)

func parseIDParam(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "无效的ID")
		return 0, false
	}
	return id, true
}

func parsePageQuery(c *gin.Context, maxPageSize int) (int, int) {
	if maxPageSize <= 0 {
		maxPageSize = defaultMaxSize
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if page < 1 {
		page = defaultPage
	}
	if pageSize < 1 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}
	return page, pageSize
}

func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return false
	}
	return true
}
