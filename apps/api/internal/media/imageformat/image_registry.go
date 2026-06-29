package imageformat

import (
	"net/http"
	"path/filepath"
	"strings"
)

type imageSignature struct {
	offset int
	bytes  []byte
}

type imageFormatSpec struct {
	Ext           string
	MIME          string
	MIMEAliases   []string
	Magic         []imageSignature
	UploadAllowed bool
}

var imageFormatSpecs = []imageFormatSpec{
	{
		Ext:           ".jpg",
		MIME:          "image/jpeg",
		MIMEAliases:   []string{"image/jpg"},
		Magic:         []imageSignature{{bytes: []byte{0xFF, 0xD8, 0xFF}}},
		UploadAllowed: true,
	},
	{
		Ext:           ".png",
		MIME:          "image/png",
		Magic:         []imageSignature{{bytes: []byte{0x89, 0x50, 0x4E, 0x47}}},
		UploadAllowed: true,
	},
	{
		Ext:           ".gif",
		MIME:          "image/gif",
		Magic:         []imageSignature{{bytes: []byte{0x47, 0x49, 0x46}}},
		UploadAllowed: true,
	},
	{
		Ext:           ".bmp",
		MIME:          "image/bmp",
		MIMEAliases:   []string{"image/x-ms-bmp"},
		Magic:         []imageSignature{{bytes: []byte{0x42, 0x4D}}},
		UploadAllowed: true,
	},
	{
		Ext:  ".webp",
		MIME: "image/webp",
		Magic: []imageSignature{
			{bytes: []byte{0x52, 0x49, 0x46, 0x46}},
			{offset: 8, bytes: []byte{0x57, 0x45, 0x42, 0x50}},
		},
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

func DetectImageExtensionFromBytes(data []byte) string {
	mimeType := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if ext := ImageExtensionFromMIME(mimeType); ext != "" {
		return ext
	}
	return ""
}

func NormalizeImageExtFromFilename(filename string) (string, bool) {
	return NormalizeImageExtension(filepath.Ext(strings.TrimSpace(filename)))
}

func imageSignaturesByExtension(ext string) []imageSignature {
	normalizedExt, ok := NormalizeImageExtension(ext)
	if !ok {
		return nil
	}
	spec := imageFormatsByExt[normalizedExt]
	return spec.Magic
}
