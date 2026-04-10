package handler

import (
	"strconv"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/response"

	"github.com/gin-gonic/gin"
)

type ChannelHandler struct {
	channelRepo *repository.ChannelRepository
}

func NewChannelHandler(channelRepo *repository.ChannelRepository) *ChannelHandler {
	return &ChannelHandler{channelRepo: channelRepo}
}

func (h *ChannelHandler) List(c *gin.Context) {
	channels, err := h.channelRepo.List()
	if err != nil {
		response.Error(c, 500, "获取渠道商列表失败")
		return
	}
	response.Success(c, gin.H{"list": channels})
}

func (h *ChannelHandler) Create(c *gin.Context) {
	var req model.CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	channel := &model.Channel{
		Type:   req.Type,
		Name:   req.Name,
		ApiURL: req.ApiURL,
		Models: req.Models,
		Status: 1,
	}

	if err := h.channelRepo.Create(channel); err != nil {
		response.Error(c, 500, "创建渠道商失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"channel": channel})
}

func (h *ChannelHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	var req model.UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	channel, err := h.channelRepo.GetByID(id)
	if err != nil {
		response.Error(c, 404, "渠道商不存在")
		return
	}

	channel.Type = req.Type
	channel.Name = req.Name
	channel.ApiURL = req.ApiURL
	channel.Models = req.Models

	if err := h.channelRepo.Update(channel); err != nil {
		response.Error(c, 500, "更新渠道商失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"channel": channel})
}

func (h *ChannelHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, "无效的ID")
		return
	}

	if err := h.channelRepo.Delete(id); err != nil {
		response.Error(c, 500, "删除渠道商失败: "+err.Error())
		return
	}

	response.Success(c, nil)
}
