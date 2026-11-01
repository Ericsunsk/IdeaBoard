package providers

import (
	"bytes"
	"context"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"mime"
	"net/url"
	"path"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/platform/safehttp"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// trustedImageDownloadHosts pins Worker result egress to the
// executor-supplied allowlist (exact host, or exact host:port). The
// transport SSRF guard still applies underneath, but on its own it permits
// any public host; this closed-world gate keeps result downloads on the
// deployment egress policy. Worker data URLs never reach the fetcher.
func trustedImageDownloadHosts(ctx context.Context) []string {
	hosts := map[string]struct{}{}
	appendHost := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && strings.TrimSpace(parsed.Host) != "" {
			hosts[strings.TrimSpace(parsed.Host)] = struct{}{}
			return
		}
		host := strings.TrimSpace(raw)
		if host != "" {
			hosts[host] = struct{}{}
		}
	}

	// Worker result URLs point at the presigned slot hosts; the default
	// safehttp policy already rejects private targets. Explicit entries
	// cover the deployment egress policy.
	for _, host := range safehttp.AllowHosts(ctx) {
		appendHost(host)
	}

	out := make([]string, 0, len(hosts))
	for host := range hosts {
		out = append(out, host)
	}
	return out
}

func normalizeOpenAIEditMultipartImage(filename, mimeType string, data []byte) (string, string, []byte, error) {
	if _, err := imageformat.InspectDetectedBytes(data); err != nil {
		return "", "", nil, err
	}
	normalizedMimeType := normalizeImageMIMEType(mimeType, data)
	switch normalizedMimeType {
	case "image/png", "image/jpeg":
		return ensureFilenameHasImageExt(filename, normalizedMimeType), normalizedMimeType, data, nil
	default:
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", "", nil, fmt.Errorf(errorrender.MsgNormalizeOpenAIEditMultipartImage, err)
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, decoded); err != nil {
			return "", "", nil, fmt.Errorf(errorrender.MsgNormalizeOpenAIEditMultipartImage2, err)
		}
		return ensureFilenameHasImageExt(filename, "image/png"), "image/png", encoded.Bytes(), nil
	}
}

func normalizeImageMIMEType(mimeType string, data []byte) string {
	if parsed, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType)); err == nil && strings.HasPrefix(parsed, "image/") {
		if normalized, ok := imageformat.NormalizeImageMIME(parsed); ok {
			return normalized
		}
	}
	if ext := imageformat.DetectImageExtensionFromBytes(data); ext != "" {
		if normalized := imageformat.ImageMIMEFromExtension(ext); normalized != "" {
			return normalized
		}
	}
	return "image/png"
}

func ensureFilenameHasImageExt(filename string, mimeType string) string {
	base := strings.TrimSpace(path.Base(filename))
	if base == "" || base == "." || base == "/" {
		base = "image"
	}
	ext := ".png"
	if mimeType == "image/jpeg" {
		ext = ".jpg"
	}
	if strings.EqualFold(path.Ext(base), ext) {
		return base
	}
	return strings.TrimSuffix(base, path.Ext(base)) + ext
}
