package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/pkg/response"
	"backend/pkg/safehttp"

	"github.com/gin-gonic/gin"
)

type SegHandler struct {
	segService  *service.SegService
	taskRepo    *repository.TaskRepository
	kodoService *service.KodoService
}

func NewSegHandler(segService *service.SegService, taskRepo *repository.TaskRepository, kodoService *service.KodoService) *SegHandler {
	return &SegHandler{segService: segService, taskRepo: taskRepo, kodoService: kodoService}
}

// GetConfig 获取分割抠图配置
func (h *SegHandler) GetConfig(c *gin.Context) {
	cfg, err := h.segService.GetConfig()
	if err != nil {
		response.Error(c, 500, "获取配置失败")
		return
	}
	// 隐藏 secret
	masked := *cfg
	if len(masked.AccessKeySecret) > 6 {
		masked.AccessKeySecret = masked.AccessKeySecret[:3] + "****" + masked.AccessKeySecret[len(masked.AccessKeySecret)-3:]
	}
	response.Success(c, gin.H{"config": masked})
}

// SaveConfig 保存分割抠图配置
func (h *SegHandler) SaveConfig(c *gin.Context) {
	var req model.AliyunSegConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.segService.SaveConfig(&req); err != nil {
		response.Error(c, 500, "保存失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "保存成功"})
}

// Segment 执行分割（用户端调用），只返回结果，不保存
func (h *SegHandler) Segment(c *gin.Context) {
	var req struct {
		ImageURL   string `json:"image_url" binding:"required"`
		SegType    string `json:"seg_type" binding:"required"`
		ReturnForm string `json:"return_form"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	resultURL, err := h.segService.Segment(req.ImageURL, req.SegType, req.ReturnForm)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}

	response.Success(c, gin.H{"result_url": resultURL})
}

// Upscale 执行图像超分（用户端调用），持久化结果并创建任务记录
func (h *SegHandler) Upscale(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}

	var req struct {
		ImageURL      string `json:"image_url" binding:"required"`
		Mode          string `json:"mode"`
		UpscaleFactor int64  `json:"upscale_factor"`
		OutputFormat  string `json:"output_format"`
		OutputQuality int64  `json:"output_quality"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	// 参数默认值与校验
	if req.Mode == "" {
		req.Mode = "base"
	}
	if req.Mode != "base" && req.Mode != "enhancement" {
		req.Mode = "base"
	}
	if req.UpscaleFactor < 1 || req.UpscaleFactor > 4 {
		req.UpscaleFactor = 2
	}
	if req.OutputFormat == "" {
		req.OutputFormat = "jpg"
	}
	if req.OutputQuality < 30 || req.OutputQuality > 100 {
		req.OutputQuality = 95
	}

	resultURL, err := h.segService.Upscale(req.ImageURL, req.Mode, req.UpscaleFactor, req.OutputFormat, req.OutputQuality)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}

	// 持久化结果图片
	persistedURL, err := h.persistImage(resultURL)
	if err != nil {
		response.Error(c, 500, "保存结果图片失败: "+err.Error())
		return
	}

	// 创建任务记录
	modeNames := map[string]string{"base": "普通模式", "enhancement": "增强模式"}
	modeName := modeNames[req.Mode]
	if modeName == "" {
		modeName = req.Mode
	}
	prompt := fmt.Sprintf("[pattern-upscale] %s %dx", modeName, req.UpscaleFactor)

	taskID := fmt.Sprintf("upscale_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
	now := time.Now()
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"image_url":      persistedURL,
		"mode":           req.Mode,
		"upscale_factor": req.UpscaleFactor,
		"status":         "success",
	})
	task := &model.Task{
		TaskID:      taskID,
		UserID:      userID,
		Prompt:      prompt,
		UserPrompt:  prompt,
		Status:      "completed",
		Result:      string(resultJSON),
		CompletedAt: &now,
	}
	if err := h.taskRepo.Create(task); err != nil {
		// 不影响返回结果
	}

	response.Success(c, gin.H{"image_url": persistedURL, "task_id": taskID})
}

// SaveSegResult 保存分割结果到画廊（先持久化图片到对象存储或本地）
func (h *SegHandler) SaveSegResult(c *gin.Context) {
	userID := c.GetUint64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}

	var req struct {
		ResultURL string `json:"result_url" binding:"required"`
		SegType   string `json:"seg_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	// 持久化图片：优先对象存储，否则下载到本地
	persistedURL, err := h.persistImage(req.ResultURL)
	if err != nil {
		response.Error(c, 500, "保存图片失败: "+err.Error())
		return
	}

	segTypeNames := map[string]string{
		"body": "人体分割", "head": "头像分割",
		"clothing": "服饰分割", "commodity": "商品分割", "common": "通用分割",
	}
	typeName := segTypeNames[req.SegType]
	if typeName == "" {
		typeName = "智能分割"
	}

	taskID := fmt.Sprintf("seg_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
	now := time.Now()
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"image_url": persistedURL,
		"seg_type":  req.SegType,
		"status":    "success",
	})
	task := &model.Task{
		TaskID:      taskID,
		UserID:      userID,
		Prompt:      typeName,
		UserPrompt:  typeName,
		Status:      "completed",
		Result:      string(resultJSON),
		CompletedAt: &now,
	}
	if err := h.taskRepo.Create(task); err != nil {
		response.Error(c, 500, "保存记录失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"image_url": persistedURL})
}

// persistImage 将远程图片 URL 持久化到对象存储或本地
func (h *SegHandler) persistImage(imageURL string) (string, error) {
	if _, err := safehttp.ValidateURL(imageURL); err != nil {
		return "", err
	}

	// 优先使用对象存储
	if h.kodoService != nil && h.kodoService.IsEnabled() {
		return h.kodoService.FetchURL(imageURL)
	}

	// 回退：下载到本地
	resp, err := safehttp.Get(context.Background(), imageURL, 20*time.Second)
	if err != nil {
		return "", fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	ext := ".png"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg") {
		ext = ".jpg"
	}

	dir := filepath.Join("uploads", "seg", time.Now().Format("2006/01/02"))
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%d_%d%s", time.Now().UnixNano(), rand.Intn(10000), ext)
	dst := filepath.Join(dir, filename)

	file, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return "/" + strings.ReplaceAll(dst, "\\", "/"), nil
}
