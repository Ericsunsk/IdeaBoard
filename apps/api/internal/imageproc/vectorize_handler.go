package imageproc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/platform/mediarefs"
	"api/internal/platform/response"
	"api/internal/platform/safehttp"
	"api/internal/storage"
	"api/internal/tasks"

	"github.com/gin-gonic/gin"
)

const (
	maxVectorizeDownloadSize = 20 * 1024 * 1024
	maxVectorizePixels       = 40_000_000
)

// potraceBin potrace 可执行文件路径，优先使用绝对路径
var potraceBin = findPotrace()

func findPotrace() string {
	// 优先检查已知路径
	known := []string{
		`/usr/bin/potrace`,
		`/usr/local/bin/potrace`,
	}
	for _, p := range known {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 回退到 PATH 中的 potrace
	if p, err := exec.LookPath("potrace"); err == nil {
		return p
	}
	return "potrace"
}

type VectorizeHandler struct {
	taskRepo       *accounts.TaskRepository
	storageService *storage.StorageService
	uploadBoards   storage.UploadBoardAccessResolver
	uploadOwners   storage.UploadOwnerStorageUIDResolver
}

func NewVectorizeHandler(taskRepo *accounts.TaskRepository, storageService *storage.StorageService, deps ...VectorizeDeps) *VectorizeHandler {
	handler := &VectorizeHandler{taskRepo: taskRepo, storageService: storageService}
	if len(deps) > 0 {
		handler.uploadBoards = deps[0].UploadBoards
		handler.uploadOwners = deps[0].UploadOwners
	}
	return handler
}

type VectorizeDeps struct {
	UploadBoards storage.UploadBoardAccessResolver
	UploadOwners storage.UploadOwnerStorageUIDResolver
}

// Vectorize 执行位图转矢量图（用户端调用）
func (h *VectorizeHandler) Vectorize(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetInt64("user_id")
	if userID == 0 {
		response.Error(c, 401, "未登录")
		return
	}

	var req struct {
		ImageURL     string  `json:"image_url" binding:"required"`
		Threshold    float64 `json:"threshold"`     // 亮度阈值 0.0-1.0, default 0.45
		TurdSize     int     `json:"turd_size"`     // 去噪点 0-100, default 2
		AlphaMax     float64 `json:"alpha_max"`     // 平滑度 0.0-1.334, default 1.0
		OptTolerance float64 `json:"opt_tolerance"` // 优化容差 0.0-5.0, default 0.2
		TurnPolicy   string  `json:"turn_policy"`   // 转向策略: minority,majority,black,white,left,right,random
		Invert       bool    `json:"invert"`        // 反转黑白
		ColorCount   int     `json:"color_count"`   // 颜色数量 0=黑白, 2-32=多色
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	// 参数默认值与校验
	if req.Threshold <= 0 || req.Threshold > 1.0 {
		req.Threshold = 0.45
	}
	if req.TurdSize < 0 || req.TurdSize > 100 {
		req.TurdSize = 2
	}
	if req.AlphaMax < 0 || req.AlphaMax > 1.334 {
		req.AlphaMax = 1.0
	}
	if req.OptTolerance < 0 || req.OptTolerance > 5.0 {
		req.OptTolerance = 0.2
	}
	validPolicies := map[string]bool{"minority": true, "majority": true, "black": true, "white": true, "left": true, "right": true, "random": true}
	if !validPolicies[req.TurnPolicy] {
		req.TurnPolicy = "minority"
	}
	if req.ColorCount < 0 || req.ColorCount > 32 {
		req.ColorCount = 0
	}

	body, err := h.loadVectorizeImage(ctx, userID, req.ImageURL)
	if err != nil {
		log.Printf("[Vectorize] load image failed: err=%v", err)
		response.Error(c, 400, err.Error())
		return
	}
	if len(body) > maxVectorizeDownloadSize {
		response.Error(c, 400, fmt.Sprintf("图片大小不能超过 %dMB", maxVectorizeDownloadSize/1024/1024))
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		response.Error(c, 400, "图片格式不受支持或内容不合法")
		return
	}
	if format != "jpeg" && format != "png" {
		response.Error(c, 400, "仅支持 JPEG / PNG 图片")
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxVectorizePixels {
		response.Error(c, 400, "图片像素过大，请缩小后重试")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		log.Printf("[Vectorize] decode image failed: err=%v", err)
		response.Error(c, 500, "解码图片失败")
		return
	}

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "vectorize-*")
	if err != nil {
		response.Error(c, 500, "创建临时目录失败")
		return
	}
	defer os.RemoveAll(tmpDir)

	var svgPath string

	if req.ColorCount >= 2 {
		// 多色模式：对每种颜色进行分层 trace
		svgPath, err = h.traceColor(img, tmpDir, req.ColorCount, req.TurdSize, req.AlphaMax, req.OptTolerance, req.TurnPolicy)
	} else {
		// 黑白模式：单次 trace
		pgmPath := filepath.Join(tmpDir, "input.pgm")
		if err2 := writePGM(img, pgmPath, req.Threshold, req.Invert); err2 != nil {
			response.Error(c, 500, "转换PGM失败: "+err2.Error())
			return
		}
		svgPath = filepath.Join(tmpDir, "output.svg")
		err = runPotrace(pgmPath, svgPath, req.TurdSize, req.AlphaMax, req.OptTolerance, req.TurnPolicy)
	}

	if err != nil {
		log.Printf("[Vectorize] trace failed: err=%v", err)
		response.Error(c, 500, "转换矢量图失败")
		return
	}

	// 读取 SVG
	svgData, err := os.ReadFile(svgPath)
	if err != nil {
		response.Error(c, 500, "读取SVG失败")
		return
	}

	// 持久化 SVG 文件
	persistedURL, err := h.persistSVG(ctx, userID, svgData)
	if err != nil {
		log.Printf("[Vectorize] persist svg failed: err=%v", err)
		response.Error(c, 500, "保存SVG失败")
		return
	}

	// 创建任务记录
	modeDesc := "黑白模式"
	if req.ColorCount >= 2 {
		modeDesc = fmt.Sprintf("%d色模式", req.ColorCount)
	}
	prompt := fmt.Sprintf("[to-vector] %s 阈值%.2f", modeDesc, req.Threshold)

	taskID := vecSecureTaskID("vec")
	now := time.Now()
	resultJSON, _ := json.Marshal(map[string]interface{}{
		"image_url":   persistedURL,
		"color_count": req.ColorCount,
		"threshold":   req.Threshold,
		"status":      "success",
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
	_ = h.taskRepo.Create(ctx, task)

	response.Success(c, gin.H{
		"svg_url": persistedURL,
		"task_id": taskID,
	})
}

// writePGM 将图片转换为 PGM 格式（灰度），并根据 threshold 二值化为 PBM
func writePGM(img image.Image, path string, threshold float64, invert bool) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写 PBM (P4) 头部 - 二值化
	// 使用 PGM P5 格式更通用
	fmt.Fprintf(f, "P5\n%d %d\n255\n", w, h)

	thresholdU8 := uint8(threshold * 255)
	buf := make([]byte, w)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// 转灰度
			gray := uint8((r*299 + g*587 + b*114 + 500) / 1000 >> 8)
			// 二值化
			if invert {
				if gray < thresholdU8 {
					buf[x-bounds.Min.X] = 255
				} else {
					buf[x-bounds.Min.X] = 0
				}
			} else {
				if gray >= thresholdU8 {
					buf[x-bounds.Min.X] = 255
				} else {
					buf[x-bounds.Min.X] = 0
				}
			}
		}
		f.Write(buf)
	}
	return nil
}

// runPotrace 调用 potrace 将 PGM/PBM 转换为 SVG
func runPotrace(inputPath, outputPath string, turdSize int, alphaMax, optTolerance float64, turnPolicy string) error {
	args := []string{
		"-s",             // SVG 输出
		"-o", outputPath, // 输出文件
		"-t", fmt.Sprintf("%d", turdSize),
		"-a", fmt.Sprintf("%.3f", alphaMax),
		"-O", fmt.Sprintf("%.3f", optTolerance),
		"-z", turnPolicy,
		inputPath,
	}

	cmd := exec.Command(potraceBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("potrace 执行失败: %v, output: %s", err, string(output))
	}
	return nil
}

// traceColor 多色矢量化：对图片进行颜色量化，然后对每层颜色分别 trace
func (h *VectorizeHandler) traceColor(img image.Image, tmpDir string, colorCount int, turdSize int, alphaMax, optTolerance float64, turnPolicy string) (string, error) {
	bounds := img.Bounds()
	imgW, imgH := bounds.Dx(), bounds.Dy()

	// 简单颜色量化：将所有像素映射到 colorCount 个灰度层
	// 对每层生成 PBM，然后 trace，最后合并 SVG
	layers := make([]string, colorCount)
	for i := 0; i < colorCount; i++ {
		pgmPath := filepath.Join(tmpDir, fmt.Sprintf("layer_%d.pgm", i))
		svgPath := filepath.Join(tmpDir, fmt.Sprintf("layer_%d.svg", i))

		lowerBound := float64(i) / float64(colorCount)
		upperBound := float64(i+1) / float64(colorCount)

		// 为该层创建 PGM：属于该亮度范围的像素为黑色
		if err := writeLayerPGM(img, pgmPath, lowerBound, upperBound, imgW, imgH, bounds); err != nil {
			return "", err
		}

		if err := runPotrace(pgmPath, svgPath, turdSize, alphaMax, optTolerance, turnPolicy); err != nil {
			return "", err
		}
		layers[i] = svgPath
	}

	// 合并所有层到一个 SVG
	outputPath := filepath.Join(tmpDir, "output.svg")
	if err := mergeSVGLayers(layers, outputPath, imgW, imgH, colorCount); err != nil {
		return "", err
	}
	return outputPath, nil
}

// writeLayerPGM 为指定亮度范围的颜色层写 PGM
func writeLayerPGM(img image.Image, path string, lowerBound, upperBound float64, w, h int, bounds image.Rectangle) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "P5\n%d %d\n255\n", w, h)

	lo := uint8(lowerBound * 255)
	hi := uint8(upperBound * 255)

	buf := make([]byte, w)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			gray := uint8((r*299 + g*587 + b*114 + 500) / 1000 >> 8)
			if gray >= lo && gray < hi {
				buf[x-bounds.Min.X] = 0 // 黑色 = 属于该层
			} else {
				buf[x-bounds.Min.X] = 255 // 白色 = 不属于该层
			}
		}
		f.Write(buf)
	}
	return nil
}

// mergeSVGLayers 合并多层 SVG 到一个文件，每层着不同灰度色
func mergeSVGLayers(layerPaths []string, outputPath string, w, h, colorCount int) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, `<?xml version="1.0" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 20010904//EN" "http://www.w3.org/TR/2001/REC-SVG-20010904/DTD/svg10.dtd">
<svg version="1.0" xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
`, w, h, w, h)

	for i, svgPath := range layerPaths {
		data, err := os.ReadFile(svgPath)
		if err != nil {
			continue
		}
		content := string(data)

		// 计算该层的灰度颜色
		grayVal := int(float64(colorCount-1-i) / float64(colorCount-1) * 255)
		fillColor := fmt.Sprintf("#%02x%02x%02x", grayVal, grayVal, grayVal)

		// 提取 SVG 内部的 path 元素
		// 简单处理：提取 <g> 和 <path> 部分
		startIdx := strings.Index(content, "<g")
		if startIdx < 0 {
			continue
		}
		endIdx := strings.LastIndex(content, "</svg>")
		if endIdx < 0 {
			endIdx = len(content)
		}
		inner := content[startIdx:endIdx]

		// 将 fill="#000000" 替换为该层颜色
		inner = strings.ReplaceAll(inner, `fill="#000000"`, fmt.Sprintf(`fill="%s"`, fillColor))
		inner = strings.ReplaceAll(inner, `fill="black"`, fmt.Sprintf(`fill="%s"`, fillColor))

		fmt.Fprintf(f, `<g id="layer%d" opacity="1">
%s
</g>
`, i, inner)
	}

	fmt.Fprintf(f, "</svg>\n")
	return nil
}

func (h *VectorizeHandler) loadVectorizeImage(ctx context.Context, userID int64, rawImageURL string) ([]byte, error) {
	imageURL := strings.TrimSpace(rawImageURL)
	if imageURL == "" {
		return nil, fmt.Errorf("图片地址不能为空")
	}
	if strings.HasPrefix(imageURL, "/uploads/") {
		return h.loadLocalUploadImage(ctx, userID, imageURL)
	}
	return downloadVectorizeImage(ctx, imageURL)
}

func (h *VectorizeHandler) loadLocalUploadImage(ctx context.Context, userID int64, rawPath string) ([]byte, error) {
	if h.uploadBoards == nil || h.uploadOwners == nil {
		return nil, fmt.Errorf("无法校验上传资源权限")
	}
	cleaned, err := mediarefs.ValidateUploadPath(rawPath)
	if err != nil {
		return nil, err
	}
	resource, err := storage.AuthorizeUploadPathRead(
		ctx,
		userID,
		cleaned,
		h.uploadBoards,
		h.uploadOwners,
	)
	if err != nil {
		return nil, fmt.Errorf("无权读取该上传资源")
	}
	localPath := filepath.FromSlash(resource.ObjectKey)
	info, err := os.Stat(localPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("图片不存在")
	}
	if info.Size() > maxVectorizeDownloadSize {
		return nil, fmt.Errorf("图片大小不能超过 %dMB", maxVectorizeDownloadSize/1024/1024)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("读取图片失败")
	}
	if len(data) > maxVectorizeDownloadSize {
		return nil, fmt.Errorf("图片大小不能超过 %dMB", maxVectorizeDownloadSize/1024/1024)
	}
	return data, nil
}

func downloadVectorizeImage(ctx context.Context, imageURL string) ([]byte, error) {
	downloadCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	imgResp, err := safehttp.Get(downloadCtx, imageURL, 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("下载图片失败")
	}
	defer imgResp.Body.Close()
	if imgResp.StatusCode != 200 {
		return nil, fmt.Errorf("下载图片返回 %d", imgResp.StatusCode)
	}
	if imgResp.ContentLength > maxVectorizeDownloadSize {
		return nil, fmt.Errorf("图片大小不能超过 %dMB", maxVectorizeDownloadSize/1024/1024)
	}
	contentType := strings.TrimSpace(strings.Split(imgResp.Header.Get("Content-Type"), ";")[0])
	if contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, fmt.Errorf("仅支持位图图片输入")
	}
	body, err := io.ReadAll(io.LimitReader(imgResp.Body, maxVectorizeDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片失败")
	}
	if len(body) > maxVectorizeDownloadSize {
		return nil, fmt.Errorf("图片大小不能超过 %dMB", maxVectorizeDownloadSize/1024/1024)
	}
	return body, nil
}

// persistSVG 将 SVG 数据保存到存储
func (h *VectorizeHandler) persistSVG(ctx context.Context, userID int64, svgData []byte) (string, error) {
	if h.storageService == nil {
		return "", fmt.Errorf("存储服务不可用")
	}
	if h.uploadOwners == nil {
		return "", fmt.Errorf("用户服务不可用")
	}
	storageUID, err := h.uploadOwners.GetStorageUIDByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	dirPrefix, err := storage.ProfileVectorUploadDirPrefix(storageUID)
	if err != nil {
		return "", err
	}

	return h.storageService.StoreBytes(ctx, "vector.svg", svgData, storage.StorageStoreOptions{
		DirPrefix: dirPrefix,
	})
}

// CheckPotrace 检查 potrace 是否已安装
func (h *VectorizeHandler) CheckPotrace(c *gin.Context) {
	cmd := exec.Command(potraceBin, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		response.Error(c, 500, "potrace 未安装或不可用，请在服务器上安装 potrace。\nLinux: apt install potrace\nmacOS: brew install potrace\nWindows: 下载 potrace 并添加到 PATH")
		return
	}
	version := strings.TrimSpace(string(output))
	response.Success(c, gin.H{"version": version})
}

func vecSecureTaskID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
