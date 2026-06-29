package httpapi

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// validateUploadFileHeader centralises the three-step upload validation:
// 1. File size limit.
// 2. Extension allow-list (lowercase, with leading dot).
// 3. Magic bytes content check (prevents content-type spoofing).
// On failure it writes an error response to c and returns ("", false).
func validateUploadFileHeader(c *gin.Context, fh *multipart.FileHeader, maxBytes int64, allowedExts map[string]bool) (ext string, ok bool) {
	if fh.Size > maxBytes {
		response.Error(c, 400, fmt.Sprintf("文件大小不能超过 %dMB", maxBytes/1024/1024))
		return "", false
	}
	ext = strings.ToLower(filepath.Ext(fh.Filename))
	if !allowedExts[ext] {
		response.Error(c, 400, "仅支持 "+imageformat.SupportedUploadImageLabel()+" 格式")
		return "", false
	}
	if err := imageformat.ValidateImageContent(fh, ext); err != nil {
		response.Error(c, 400, err.Error())
		return "", false
	}
	return ext, true
}

func decodeUploadImageMetadata(fh *multipart.FileHeader, ext string) (uploadImageMetadata, bool) {
	f, err := fh.Open()
	if err != nil {
		return uploadImageMetadata{}, false
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return uploadImageMetadata{}, false
	}
	formatExt, ok := imageformat.NormalizeImageExtension(format)
	if !ok {
		formatExt = ext
	}
	mimeType := imageformat.ImageMIMEFromExtension(formatExt)
	if mimeType == "" {
		mimeType = imageformat.ImageMIMEFromExtension(ext)
	}
	return uploadImageMetadata{
		Width:  cfg.Width,
		Height: cfg.Height,
		Mime:   mimeType,
		Ext:    strings.TrimPrefix(formatExt, "."),
	}, true
}
