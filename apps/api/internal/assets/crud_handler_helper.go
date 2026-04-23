package assets

import (
	"api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

func handleListEnabled(c *gin.Context, listFn func() (any, error), errMsg string) {
	list, err := listFn()
	if err != nil {
		response.Error(c, 500, errMsg)
		return
	}
	response.Success(c, gin.H{"list": list})
}

func handlePagedList(c *gin.Context, maxPageSize int, listFn func(page, pageSize int) (any, int64, error)) {
	page, pageSize := parsePageQuery(c, maxPageSize)
	list, total, err := listFn(page, pageSize)
	if err != nil {
		response.Error(c, 500, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"total": total, "list": list})
}

func handleCreate[TReq any, TEntity any](
	c *gin.Context,
	buildEntity func(req *TReq) *TEntity,
	createFn func(entity *TEntity) error,
	successKey string,
) {
	var req TReq
	if !bindJSON(c, &req) {
		return
	}
	entity := buildEntity(&req)
	if err := createFn(entity); err != nil {
		response.InternalError(c, 500, "创建失败", err)
		return
	}
	response.Success(c, gin.H{successKey: entity})
}

func handleUpdate[TReq any, TEntity any](
	c *gin.Context,
	getFn func(id uint64) (*TEntity, error),
	applyUpdate func(entity *TEntity, req *TReq),
	updateFn func(entity *TEntity) error,
	notFoundMsg string,
) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var req TReq
	if !bindJSON(c, &req) {
		return
	}
	entity, err := getFn(id)
	if err != nil {
		response.Error(c, 404, notFoundMsg)
		return
	}
	applyUpdate(entity, &req)
	if err := updateFn(entity); err != nil {
		response.InternalError(c, 500, "更新失败", err)
		return
	}
	response.Success(c, nil)
}

func handleDelete(c *gin.Context, deleteFn func(id uint64) error) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := deleteFn(id); err != nil {
		response.InternalError(c, 500, "删除失败", err)
		return
	}
	response.Success(c, nil)
}
