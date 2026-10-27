package dialects

import (
	"encoding/base64"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"ideaboard/api/internal/media/imageformat"
)

// InjectCustomParams merges operator custom JSON over an encoded body. Only
// the worker calls this, once, after Encode: dialects never merge, so base
// shapes stay pure and testable. Non-object JSON is ignored (never an
// error); validation already rejected it upstream.
func InjectCustomParams(encoded []byte, customParamsJSON, provider, model string) ([]byte, error) {
	trimmedCustom := strings.TrimSpace(customParamsJSON)
	if trimmedCustom == "" || trimmedCustom == "{}" {
		return encoded, nil
	}
	customParsed := gjson.Parse(trimmedCustom)
	if !customParsed.IsObject() {
		slog.Debug("ignoring non-object custom params", "provider", provider, "model", model)
		return encoded, nil
	}
	var mergeErr error
	customParsed.ForEach(func(key, value gjson.Result) bool {
		newEncoded, err := sjson.SetBytes(encoded, key.String(), value.Value())
		if err != nil {
			mergeErr = err
			return false
		}
		encoded = newEncoded
		return true
	})
	if mergeErr != nil {
		return nil, fmt.Errorf(errorrender.MsgInjectCustomParams, mergeErr)
	}
	return encoded, nil
}

// CheckVendorError maps explicit vendor error envelopes to errors. Plain
// messages (no new taxonomy): the worker's existing failure classifier
// owns retryability.
func CheckVendorError(parsed gjson.Result) error {
	if errVal := parsed.Get("error.message"); errVal.Exists() && strings.TrimSpace(errVal.String()) != "" {
		return fmt.Errorf(errorrender.MsgCheckVendorError, errVal.String())
	}
	if errVal := parsed.Get("error"); errVal.Exists() && errVal.Type == gjson.String && strings.TrimSpace(errVal.String()) != "" {
		return fmt.Errorf(errorrender.MsgCheckVendorError, errVal.String())
	}
	if detailVal := parsed.Get("detail"); detailVal.Exists() && strings.TrimSpace(detailVal.String()) != "" {
		return fmt.Errorf(errorrender.MsgCheckVendorError, detailVal.String())
	}
	return nil
}

// SniffImageStream wraps a binary image response body as a data URL.
// It reports false for non-image bytes; the caller then runs JSON decode.
// Vendor-agnostic, so the worker applies it once before dialect Decode.
// The sniffed MIME is canonicalized through imageformat.NormalizeImageMIME
// when the registry knows it; the image/ gate itself is unchanged.
func SniffImageStream(body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	contentType := http.DetectContentType(body)
	if normalized, ok := imageformat.NormalizeImageMIME(contentType); ok {
		contentType = normalized
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", false
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(body)), true
}

// Base64ImageDataURL validates a bare base64 candidate as image bytes.
// Plain text often decodes as base64 alphabet, so decoding alone would
// misclassify error messages as images; the content sniff is required.
func Base64ImageDataURL(candidate string) (string, bool) {
	if len(candidate) < 32 {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(candidate)
	if err != nil {
		if decoded, err = base64.RawStdEncoding.DecodeString(candidate); err != nil {
			return "", false
		}
	}
	if len(decoded) == 0 {
		return "", false
	}
	contentType := http.DetectContentType(decoded)
	if normalized, ok := imageformat.NormalizeImageMIME(contentType); ok {
		contentType = normalized
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", false
	}
	return "data:" + contentType + ";base64," + candidate, true
}

// candidateCollector dedupes accepted image URLs in first-seen order. Bare
// base64 converts only when it decodes to image bytes; other non-URL
// strings are dropped instead of misclassified.
type candidateCollector struct {
	urls []string
	seen map[string]bool
}

func newCandidateCollector() *candidateCollector {
	return &candidateCollector{seen: make(map[string]bool)}
}

func (c *candidateCollector) add(candidate string) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || c.seen[candidate] {
		return
	}
	if !strings.HasPrefix(candidate, "http://") && !strings.HasPrefix(candidate, "https://") && !strings.HasPrefix(candidate, "data:") {
		converted, ok := Base64ImageDataURL(candidate)
		if !ok {
			return
		}
		candidate = converted
	}
	c.seen[candidate] = true
	c.urls = append(c.urls, candidate)
}

// collectPaths extracts string leaves along the given gjson paths in order.
func (c *candidateCollector) collectPaths(parsed gjson.Result, paths []string) {
	for _, path := range paths {
		res := parsed.Get(path)
		if !res.Exists() {
			continue
		}
		if res.IsArray() {
			for _, item := range res.Array() {
				if item.Type == gjson.String {
					c.add(item.String())
				}
			}
		} else if res.Type == gjson.String {
			c.add(res.String())
		}
	}
}

// scanLeaves collects any http(s)/data:image URL in the JSON tree. It is
// the recall fallback shared by every dialect, not a primary strategy.
func (c *candidateCollector) scanLeaves(val gjson.Result) {
	switch {
	case val.IsArray():
		for _, item := range val.Array() {
			c.scanLeaves(item)
		}
	case val.IsObject():
		val.ForEach(func(_, v gjson.Result) bool {
			c.scanLeaves(v)
			return true
		})
	case val.Type == gjson.String:
		if s := strings.TrimSpace(val.String()); strings.HasPrefix(s, "http://") ||
			strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "data:image/") {
			if _, err := url.Parse(s); err == nil {
				c.add(s)
			}
		}
	}
}

// topLevelKeysError explains an imageless response with its top-level keys.
func topLevelKeysError(parsed gjson.Result) error {
	var topKeys []string
	if parsed.IsObject() {
		parsed.ForEach(func(key, _ gjson.Result) bool {
			topKeys = append(topKeys, key.String())
			return true
		})
	}
	if len(topKeys) > 0 {
		return fmt.Errorf(errorrender.MsgTopLevelKeysError, strings.Join(topKeys, ", "))
	}
	return errors.New(errorrender.MsgTopLevelKeysError2)
}

// universalResponsePaths is the extraction order shared by every
// dialect: standard shapes first, vendor leaves after, chat content via the
// hook, deep-leaf scan last. Per-dialect decode specialization waits for
// vendor evidence; today the dialects differ in Encode only.
var universalResponsePaths = []string{
	"data.#.url",
	"data.#.b64_json",
	"images.#.url",
	"images.#",
	"image.url",
	"image",
	"choices.#.message.images.#.image_url.url",
	"choices.#.message.images.#.url",
	"artifacts.#.base64",
	"output.#.image",
	"output.#",
	"result.#.url",
	"results.#.url",
}

// collectChatContentParts handles message.content part arrays, including the
// stringified-JSON array variant some gateways return.
func collectChatContentParts(parsed gjson.Result, collect *candidateCollector) {
	for _, choice := range parsed.Get("choices.#.message.content").Array() {
		if choice.IsArray() {
			collectChatParts(choice.Array(), collect)
			continue
		}
		if choice.Type == gjson.String {
			if inner := gjson.Parse(choice.String()); inner.IsArray() {
				collectChatParts(inner.Array(), collect)
			}
		}
	}
}

func collectChatParts(parts []gjson.Result, collect *candidateCollector) {
	for _, part := range parts {
		if part.Get("type").String() == "image_url" {
			if u := part.Get("image_url.url").String(); u != "" {
				collect.add(u)
			}
		}
	}
}

// Decode runs the shared JSON decode skeleton: error envelope,
// ordered vendor paths, chat content parts, deep-leaf fallback,
// transparent failure.
// It is the single decode implementation for every dialect: Decode stays a
// package function, never a per-dialect override.
func Decode(body []byte) ([]string, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New(errorrender.MsgDecodeWithPaths)
	}
	parsed := gjson.ParseBytes(body)
	if !parsed.IsObject() && !parsed.IsArray() {
		return nil, fmt.Errorf(errorrender.MsgDecodeWithPaths2, http.DetectContentType(body))
	}
	if err := CheckVendorError(parsed); err != nil {
		return nil, err
	}
	collect := newCandidateCollector()
	collect.collectPaths(parsed, universalResponsePaths)
	if len(collect.urls) == 0 {
		collectChatContentParts(parsed, collect)
	}
	if len(collect.urls) == 0 {
		parsed.ForEach(func(_, value gjson.Result) bool {
			collect.scanLeaves(value)
			return len(collect.urls) == 0
		})
	}
	if len(collect.urls) == 0 {
		return nil, topLevelKeysError(parsed)
	}
	return collect.urls, nil
}
