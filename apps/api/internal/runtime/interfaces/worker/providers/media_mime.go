package providers

import "ideaboard/api/internal/media/imageformat"

func inferLocalAssetMimeType(filename string, data []byte) string {
	if len(data) > 0 {
		if extension := imageformat.DetectImageExtensionFromBytes(data); extension != "" {
			if mimeType := imageformat.ImageMIMEFromExtension(extension); mimeType != "" {
				return mimeType
			}
		}
	}
	if extension, ok := imageformat.NormalizeImageExtFromFilename(filename); ok {
		if mimeType := imageformat.ImageMIMEFromExtension(extension); mimeType != "" {
			return mimeType
		}
	}
	return "image/png"
}
