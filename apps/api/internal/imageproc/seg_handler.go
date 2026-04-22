package imageproc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"api/internal/accounts"
	"api/internal/platform/response"
	"api/internal/platform/safehttp"
	"api/internal/storage"
	"api/internal/tasks"
	"api/internal/workflow"

	"github.com/gin-gonic/gin"
)

type SegHandler struct {
	segService     *SegService
	taskRepo       *accounts.TaskRepository
	storageService *storage.StorageService
}

func NewSegHandler(segService *SegService, taskRepo *accounts.TaskRepository, storageService *storage.StorageService) *SegHandler {
	return &SegHandler{segService: segService, taskRepo: taskRepo, storageService: storageService}
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
	} else if masked.AccessKeySecret != "" {
		masked.AccessKeySecret = "****"
	}
	response.Success(c, gin.H{"config": masked})
}

// SaveConfig 保存分割抠图配置
func (h *SegHandler) SaveConfig(c *gin.Context) {
	var req AliyunSegConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}
	if err := h.segService.SaveConfig(&req); err != nil {
		response.InternalError(c, 500, "保存失败", err)
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
		response.Error(c, 400, "参数错误")
		return
	}

	validSegTypes := map[string]bool{"body": true, "head": true, "clothing": true, "commodity": true, "common": true}
	if !validSegTypes[req.SegType] {
		response.Error(c, 400, "不支持的分割类型: "+req.SegType)
		return
	}

	resultURL, err := h.segService.Segment(req.ImageURL, req.SegType, req.ReturnForm)
	if err != nil {
		log.Printf("[Seg] segment failed: seg_type=%s err=%v", req.SegType, err)
		response.Error(c, 500, "图像处理失败，请稍后重试")
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
		Feature       string `json:"feature"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
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
		log.Printf("[Seg] upscale failed: mode=%s factor=%d err=%v", req.Mode, req.UpscaleFactor, err)
		response.Error(c, 500, "图像处理失败，请稍后重试")
		return
	}

	// 持久化结果图片
	persistedURL, err := h.persistImage(resultURL)
	if err != nil {
		log.Printf("[Seg] persist upscale image failed: err=%v", err)
		response.Error(c, 500, "保存结果图片失败")
		return
	}

	// 创建任务记录
	modeNames := map[string]string{"base": "普通模式", "enhancement": "增强模式"}
	modeName := modeNames[req.Mode]
	if modeName == "" {
		modeName = req.Mode
	}
	featureSlug := workflow.NormalizeWorkflowFeatureSlugOrFallback(req.Feature, "hd-upscale")
	prompt := fmt.Sprintf("[%s] %s %dx", featureSlug, modeName, req.UpscaleFactor)

	taskID := segSecureTaskID("upscale")
	now := time.Now()
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"image_url":      persistedURL,
		"mode":           req.Mode,
		"upscale_factor": req.UpscaleFactor,
		"feature":        featureSlug,
		"status":         "success",
	})
	task := &tasks.Task{
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

	response.Success(c, gin.H{"image_url": persistedURL, "task_id": taskID, "feature": featureSlug})
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
		Feature   string `json:"feature"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误")
		return
	}

	// 持久化图片：优先对象存储，否则下载到本地
	persistedURL, err := h.persistImage(req.ResultURL)
	if err != nil {
		log.Printf("[Seg] persist segmented image failed: err=%v", err)
		response.Error(c, 500, "保存图片失败")
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
	featureSlug := workflow.NormalizeWorkflowFeatureSlugOrFallback(req.Feature, "")
	prompt := typeName
	if featureSlug != "" {
		prompt = fmt.Sprintf("[%s] %s", featureSlug, typeName)
	}

	taskID := segSecureTaskID("seg")
	now := time.Now()
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"image_url": persistedURL,
		"seg_type":  req.SegType,
		"feature":   featureSlug,
		"status":    "success",
	})
	task := &tasks.Task{
		TaskID:      taskID,
		UserID:      userID,
		Prompt:      prompt,
		UserPrompt:  prompt,
		Status:      "completed",
		Result:      string(resultJSON),
		CompletedAt: &now,
	}
	if err := h.taskRepo.Create(task); err != nil {
		log.Printf("[Seg] save segment task failed: task_id=%s err=%v", taskID, err)
		response.Error(c, 500, "保存记录失败")
		return
	}

	response.Success(c, gin.H{"image_url": persistedURL, "task_id": taskID, "feature": featureSlug})
}

// persistImage 将远程图片 URL 持久化到对象存储或本地
func (h *SegHandler) persistImage(imageURL string) (string, error) {
	if _, err := safehttp.ValidateURL(imageURL); err != nil {
		return "", err
	}

	if h.storageService == nil {
		return "", fmt.Errorf("存储服务不可用")
	}

	return h.storageService.StoreRemote(context.Background(), imageURL, storage.StorageStoreOptions{
		DirPrefix: "seg",
	})
}

func segSecureTaskID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
