package httpapi

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/safehttp"

	"github.com/gin-gonic/gin"
)

func downloadImageHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		const maxDownloadSize = 50 * 1024 * 1024 // 50 MB

		imgURL := c.Query("url")
		if imgURL == "" {
			response.Error(c, 400, "缺少 url 参数")
			return
		}
		resp, err := safehttp.Get(c.Request.Context(), imgURL, 20*time.Second)
		if err != nil {
			response.Error(c, 400, "下载失败")
			return
		}
		defer resp.Body.Close()

		contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
			response.Error(c, 400, "仅支持下载图片类型文件")
			return
		}
		magicExt := media.ImageExtensionFromMIME(contentType)
		if magicExt == "" {
			response.Error(c, 400, "仅支持 "+media.SupportedUploadImageLabel()+" 图片")
			return
		}

		if resp.ContentLength > maxDownloadSize {
			response.Error(c, 400, "文件大小不能超过 50MB")
			return
		}

		tmpFile, err := os.CreateTemp("", "feng-download-*")
		if err != nil {
			response.Error(c, 500, "创建临时文件失败")
			return
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize+1))
		if err != nil {
			response.Error(c, 500, "读取远程文件失败")
			return
		}
		if written > maxDownloadSize {
			response.Error(c, 400, "文件大小不能超过 50MB")
			return
		}
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			response.Error(c, 500, "读取临时文件失败")
			return
		}

		header := make([]byte, 16)
		if n, readErr := tmpFile.Read(header); readErr == nil && n > 0 {
			if validateErr := media.ValidateImageBytes(header[:n], magicExt); validateErr != nil {
				response.Error(c, 400, "文件内容与类型不匹配")
				return
			}
		}
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			response.Error(c, 500, "读取临时文件失败")
			return
		}

		c.Header("Content-Disposition", "attachment; filename=image.png")
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.FormatInt(written, 10))
		c.Status(resp.StatusCode)
		if _, err := io.Copy(c.Writer, tmpFile); err != nil {
			c.Error(err)
		}
	}
}
