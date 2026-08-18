package httpapi

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultImagePreviewMaxSide = 1280
	maxImagePreviewMaxSide     = 2048
	imagePreviewJPEGQuality    = 82
)

func imagePreviewHandler(deps UploadDownloadDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawURL := strings.TrimSpace(c.Query("url"))
		resource, managed := parseManagedUploadResource(rawURL)
		if !managed {
			response.Error(c, http.StatusBadRequest, "仅支持受控上传图片预览")
			return
		}

		maxSide, ok := parseImagePreviewMaxSide(c.Query("max_side"))
		if !ok {
			response.Error(c, http.StatusBadRequest, "图片预览尺寸无效")
			return
		}

		if !authorizeManagedUploadRead(c, deps, resource) {
			return
		}
		if deps.PreviewStore != nil {
			cached, found, err := deps.PreviewStore.ReadImagePreview(c.Request.Context(), rawURL, maxSide, maxDownloadSize+1)
			if err != nil {
				slog.WarnContext(c.Request.Context(), "image preview cache read failed", "max_side", maxSide, "err", err)
			} else if found && cached != nil {
				writeImagePreviewResponse(c, cached.Data, cached.ContentType, "cache")
				return
			}
		}

		object, ok := readManagedUploadObjectAfterAuthorization(c, deps, rawURL)
		if !ok {
			return
		}
		if len(object.Data) == 0 || len(object.Data) > maxDownloadSize {
			response.Error(c, http.StatusBadRequest, "图片大小无效")
			return
		}
		if err := imageformat.ValidateImageDimensions(object.Data); err != nil {
			response.Error(c, http.StatusBadRequest, "图片尺寸无效")
			return
		}

		img, format, err := image.Decode(bytes.NewReader(object.Data))
		if err != nil {
			response.Error(c, http.StatusBadRequest, "图片内容无效")
			return
		}

		bounds := img.Bounds()
		longestSide := max(bounds.Dx(), bounds.Dy())
		if longestSide <= maxSide {
			writeImagePreviewResponse(c, object.Data, resolvePreviewContentType(object.ContentType, format), "original")
			return
		}

		scale := float64(maxSide) / float64(longestSide)
		width := max(1, int(float64(bounds.Dx())*scale+0.5))
		height := max(1, int(float64(bounds.Dy())*scale+0.5))
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)

		var encoded bytes.Buffer
		contentType := "image/jpeg"
		if imageHasTransparency(resized) {
			contentType = "image/png"
			if err := png.Encode(&encoded, resized); err != nil {
				response.InternalError(c, http.StatusInternalServerError, "生成图片预览失败", err)
				return
			}
		} else if err := jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: imagePreviewJPEGQuality}); err != nil {
			response.InternalError(c, http.StatusInternalServerError, "生成图片预览失败", err)
			return
		}
		preview := encoded.Bytes()
		if deps.PreviewStore != nil {
			if err := deps.PreviewStore.StoreImagePreview(c.Request.Context(), rawURL, maxSide, preview, contentType); err != nil {
				slog.WarnContext(c.Request.Context(), "image preview cache write failed", "max_side", maxSide, "err", err)
			}
		}
		writeImagePreviewResponse(c, preview, contentType, "generated")
	}
}

func parseImagePreviewMaxSide(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return defaultImagePreviewMaxSide, true
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > maxImagePreviewMaxSide {
		return 0, false
	}
	return value, true
}

func resolvePreviewContentType(raw, format string) string {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0]))
	if _, ok := imageformat.NormalizeImageMIME(contentType); ok {
		return contentType
	}
	if mimeType := imageformat.ImageMIMEFromExtension(format); mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}

func imageHasTransparency(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A < 255 {
				return true
			}
		}
	}
	return false
}

func writeImagePreviewResponse(c *gin.Context, data []byte, contentType string, previewKind string) {
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("Vary", "Cookie")
	c.Header("Content-Disposition", "inline")
	c.Header("X-IdeaBoard-Image-Preview", previewKind)
	c.Data(http.StatusOK, contentType, data)
}
