package imageformat

import (
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

type imageFormatSpec struct {
	Ext           string
	MIME          string
	MIMEAliases   []string
	UploadAllowed bool
}

var imageFormatSpecs = []imageFormatSpec{
	{
		Ext:           ".jpg",
		MIME:          "image/jpeg",
		MIMEAliases:   []string{"image/jpg"},
		UploadAllowed: true,
	},
	{
		Ext:           ".png",
		MIME:          "image/png",
		MIMEAliases:   []string{"image/apng", "image/vnd.mozilla.apng"},
		UploadAllowed: true,
	},
	{
		Ext:           ".gif",
		MIME:          "image/gif",
		UploadAllowed: true,
	},
	{
		Ext:           ".bmp",
		MIME:          "image/bmp",
		MIMEAliases:   []string{"image/x-ms-bmp"},
		UploadAllowed: true,
	},
	{
		Ext:           ".webp",
		MIME:          "image/webp",
		UploadAllowed: true,
	},
	{
		Ext:           ".avif",
		MIME:          "image/avif",
		UploadAllowed: true,
	},
}

var (
	imageFormatsByExt  map[string]imageFormatSpec
	imageFormatsByMIME map[string]imageFormatSpec
)

func init() {
	imageFormatsByExt = make(map[string]imageFormatSpec, len(imageFormatSpecs)+1)
	imageFormatsByMIME = make(map[string]imageFormatSpec, len(imageFormatSpecs)*2)
	for _, spec := range imageFormatSpecs {
		imageFormatsByExt[spec.Ext] = spec
		imageFormatsByMIME[spec.MIME] = spec
		for _, alias := range spec.MIMEAliases {
			imageFormatsByMIME[alias] = spec
		}
		if spec.Ext == ".jpg" {
			imageFormatsByExt[".jpeg"] = spec
		}
	}
}

func SupportedUploadImageLabel() string {
	labels := make([]string, 0, len(imageFormatSpecs))
	for _, spec := range imageFormatSpecs {
		if !spec.UploadAllowed {
			continue
		}
		labels = append(labels, strings.ToUpper(strings.TrimPrefix(spec.Ext, ".")))
	}
	return strings.Join(labels, " / ")
}

func UploadAllowedImageExtensions() map[string]bool {
	out := make(map[string]bool, len(imageFormatSpecs)+1)
	for _, spec := range imageFormatSpecs {
		if spec.UploadAllowed {
			out[spec.Ext] = true
		}
	}
	out[".jpeg"] = out[".jpg"]
	return out
}

func NormalizeImageExtension(ext string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(ext))
	if normalized == "" {
		return "", false
	}
	if !strings.HasPrefix(normalized, ".") {
		normalized = "." + normalized
	}
	spec, ok := imageFormatsByExt[normalized]
	if !ok {
		return "", false
	}
	return spec.Ext, true
}

func NormalizeImageMIME(mimeType string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if normalized == "" {
		return "", false
	}
	spec, ok := imageFormatsByMIME[normalized]
	if !ok {
		return "", false
	}
	return spec.MIME, true
}

func ImageExtensionFromMIME(mimeType string) string {
	normalizedMIME, ok := NormalizeImageMIME(mimeType)
	if !ok {
		return ""
	}
	return imageFormatsByMIME[normalizedMIME].Ext
}

func ImageMIMEFromExtension(ext string) string {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return ""
	}
	return imageFormatsByExt[normalizedExt].MIME
}

// DetectImageExtensionFromBytes sniffs the canonical extension with the shared
// magic database, gated through the upload allowlist registry. Unknown formats
// and truncated headers resolve to ""; no per-format fallback is applied.
func DetectImageExtensionFromBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return ImageExtensionFromMIME(mimetype.Detect(data).String())
}
