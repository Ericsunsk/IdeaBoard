package httpapi

import (
	"fmt"
	"mime/multipart"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// validateUploadFileHeader centralises upload validation:
// 1. File size limit.
// 2. Format detection and image-config validation from actual bytes.
// 3. Detected-format allow-list enforcement (prevents metadata spoofing).
// On failure it writes an error response to c and returns ("", false).
func validateUploadFileHeader(c *gin.Context, fh *multipart.FileHeader, maxBytes int64, allowedExts map[string]bool) (ext string, ok bool) {
	if fh == nil {
		response.Error(c, 400, "请选择文件")
		return "", false
	}
	if fh.Size > maxBytes {
		response.Error(c, 400, fmt.Sprintf("文件大小不能超过 %dMB", maxBytes/1024/1024))
		return "", false
	}
	facts, err := imageformat.InspectFileHeader(fh)
	if err != nil || !allowedExts[facts.Extension] {
		if err != nil {
			response.Error(c, 400, err.Error())
		} else {
			response.Error(c, 400, "仅支持 "+imageformat.SupportedUploadImageLabel()+" 格式")
		}
		return "", false
	}
	ext = facts.Extension
	return ext, true
}
