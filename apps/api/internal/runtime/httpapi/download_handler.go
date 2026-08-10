package httpapi

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/safehttp"

	"github.com/gin-gonic/gin"
)

const maxDownloadSize = 50 * 1024 * 1024

type downloadImageSource struct {
	body          io.Reader
	contentType   string
	contentLength int64
	statusCode    int
}

func downloadImageHandler(deps UploadDownloadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		imgURL := c.Query("url")
		if imgURL == "" {
			response.Error(c, 400, "缺少 url 参数")
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie")
		if resource, ok := parseManagedUploadResource(imgURL); ok {
			source, ok := readManagedUpload(c, deps, imgURL, resource)
			if !ok {
				return
			}
			serveDownloadedImage(c, source)
			return
		}
		resp, err := safehttp.Get(c.Request.Context(), imgURL, 20*time.Second)
		if err != nil {
			response.Error(c, 400, "下载失败")
			return
		}
		defer resp.Body.Close()

		serveDownloadedImage(c, downloadImageSource{
			body:          resp.Body,
			contentType:   resp.Header.Get("Content-Type"),
			contentLength: resp.ContentLength,
			statusCode:    resp.StatusCode,
		})
	}
}

func parseManagedUploadResource(rawURL string) (storage.UploadResource, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "" && (parsed.Scheme != "http" && parsed.Scheme != "https")) {
		return storage.UploadResource{}, false
	}
	resource, err := storage.ParseUploadResource(strings.TrimPrefix(parsed.Path, "/"))
	if err != nil {
		return storage.UploadResource{}, false
	}
	return resource, true
}

func readManagedUpload(
	c *gin.Context,
	deps UploadDownloadDeps,
	rawURL string,
	resource storage.UploadResource,
) (downloadImageSource, bool) {
	identity, ok := principal.Require(c)
	if !ok {
		return downloadImageSource{}, false
	}
	if deps.Reader == nil || deps.ReadAccess == nil {
		response.InternalError(c, 500, "下载服务暂不可用", nil)
		return downloadImageSource{}, false
	}
	if _, err := deps.ReadAccess.AuthorizeUploadResourceRead(c.Request.Context(), identity.UserID(), resource); err != nil {
		writeStaticUploadReadError(c, resource.Kind, err)
		return downloadImageSource{}, false
	}
	object, err := deps.Reader.ReadUploadResourceByURL(c.Request.Context(), rawURL, maxDownloadSize+1)
	if err != nil {
		response.InternalError(c, 500, "下载失败", err)
		return downloadImageSource{}, false
	}
	if object == nil {
		response.InternalError(c, 500, "下载失败", nil)
		return downloadImageSource{}, false
	}
	return downloadImageSource{
		body:          bytes.NewReader(object.Data),
		contentType:   object.ContentType,
		contentLength: int64(len(object.Data)),
		statusCode:    200,
	}, true
}

func serveDownloadedImage(c *gin.Context, source downloadImageSource) {
	contentType := strings.TrimSpace(strings.Split(source.contentType, ";")[0])
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		response.Error(c, 400, "仅支持下载图片类型文件")
		return
	}
	magicExt := imageformat.ImageExtensionFromMIME(contentType)
	if magicExt == "" {
		response.Error(c, 400, "仅支持 "+imageformat.SupportedUploadImageLabel()+" 图片")
		return
	}
	if source.contentLength > maxDownloadSize {
		response.Error(c, 400, "文件大小不能超过 50MB")
		return
	}

	tmpFile, err := os.CreateTemp("", "ideaboard-download-*")
	if err != nil {
		response.Error(c, 500, "创建临时文件失败")
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(source.body, maxDownloadSize+1))
	if err != nil {
		response.Error(c, 500, "读取图片失败")
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
		if validateErr := imageformat.ValidateImageBytes(header[:n], magicExt); validateErr != nil {
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
	c.Status(source.statusCode)
	if _, err := io.Copy(c.Writer, tmpFile); err != nil {
		c.Error(err)
	}
}
