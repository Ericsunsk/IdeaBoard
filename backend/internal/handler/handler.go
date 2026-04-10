package handler

import (
	"backend/pkg/redis"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewHandler(db *gorm.DB, rdb *redis.Client) *Handler {
	return &Handler{
		db:  db,
		rdb: rdb,
	}
}

func (h *Handler) Health(c *gin.Context) {
	response.Success(c, gin.H{
		"status": "ok",
	})
}
