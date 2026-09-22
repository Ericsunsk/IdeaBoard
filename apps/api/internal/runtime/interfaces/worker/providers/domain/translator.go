package domain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"ideaboard/api/internal/workflow"
)

// GatewayTranslator provides zero-hardcoding request translation
// and multi-path response extraction for AI Gateway suppliers.
type GatewayTranslator struct{}

// NewGatewayTranslator creates a new GatewayTranslator domain service.
func NewGatewayTranslator() *GatewayTranslator {
	return &GatewayTranslator{}
}

// ExtractImageResponses extracts image URLs or base64 payloads from
// any vendor response body using intelligent multi-path probing and stream sniffing.
func (t *GatewayTranslator) ExtractImageResponses(body []byte) ([]string, error) {
	return ExtractImageResponses(body)
}

// ExtractImageResponses is the standalone domain function for image payload extraction.
func ExtractImageResponses(body []byte) ([]string, error) {
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("供应商返回空响应")
	}

	// 1. Binary Image Stream Sniffing: Direct PNG/JPEG/WEBP/GIF stream
	contentType := http.DetectContentType(body)
	if strings.HasPrefix(contentType, "image/") {
		encoded := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(body))
		return []string{encoded}, nil
	}

	// 2. Parse JSON
	parsed := gjson.ParseBytes(body)
	if !parsed.IsObject() && !parsed.IsArray() {
		return nil, fmt.Errorf("供应商返回非标准响应内容 (格式: %s)", contentType)
	}

	// 2.1 Check explicit vendor error structures
	if errVal := parsed.Get("error.message"); errVal.Exists() && strings.TrimSpace(errVal.String()) != "" {
		return nil, fmt.Errorf("供应商返回错误: %s", errVal.String())
	}
	if errVal := parsed.Get("error"); errVal.Exists() && errVal.Type == gjson.String && strings.TrimSpace(errVal.String()) != "" {
		return nil, fmt.Errorf("供应商返回错误: %s", errVal.String())
	}
	if detailVal := parsed.Get("detail"); detailVal.Exists() && strings.TrimSpace(detailVal.String()) != "" {
		return nil, fmt.Errorf("供应商返回错误: %s", detailVal.String())
	}

	urls := make([]string, 0, 2)
	seen := make(map[string]bool)
	addCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			return
		}
		// Base64 without data prefix
		if !strings.HasPrefix(candidate, "http://") && !strings.HasPrefix(candidate, "https://") && !strings.HasPrefix(candidate, "data:") {
			if _, err := base64.StdEncoding.DecodeString(candidate); err == nil && len(candidate) > 20 {
				candidate = "data:image/png;base64," + candidate
			}
		}
		seen[candidate] = true
		urls = append(urls, candidate)
	}

	// 3. Multi-path extraction matching standard shapes across the industry
	candidatePaths := []string{
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

	for _, path := range candidatePaths {
		res := parsed.Get(path)
		if !res.Exists() {
			continue
		}
		if res.IsArray() {
			for _, item := range res.Array() {
				if item.Type == gjson.String {
					addCandidate(item.String())
				}
			}
		} else if res.Type == gjson.String {
			addCandidate(res.String())
		}
	}

	// 4. Inspect Chat Completions multimodal content parts
	contentChoices := parsed.Get("choices.#.message.content")
	if contentChoices.Exists() {
		for _, choice := range contentChoices.Array() {
			if choice.IsArray() {
				for _, part := range choice.Array() {
					if part.Get("type").String() == "image_url" {
						if u := part.Get("image_url.url").String(); u != "" {
							addCandidate(u)
						}
					}
				}
			} else if choice.Type == gjson.String {
				rawStr := choice.String()
				if strings.HasPrefix(strings.TrimSpace(rawStr), "[") {
					innerParsed := gjson.Parse(rawStr)
					if innerParsed.IsArray() {
						for _, part := range innerParsed.Array() {
							if part.Get("type").String() == "image_url" {
								if u := part.Get("image_url.url").String(); u != "" {
									addCandidate(u)
								}
							}
						}
					}
				}
			}
		}
	}

	if len(urls) > 0 {
		return urls, nil
	}

	// 5. Recursive deep leaf scan for any valid http(s) or data:image URL
	parsed.ForEach(func(key, value gjson.Result) bool {
		scanJSONLeaves(value, addCandidate)
		return len(urls) == 0
	})

	if len(urls) > 0 {
		return urls, nil
	}

	// 6. Transparent error explanation with top-level keys
	var topKeys []string
	if parsed.IsObject() {
		parsed.ForEach(func(key, value gjson.Result) bool {
			topKeys = append(topKeys, key.String())
			return true
		})
	}
	if len(topKeys) > 0 {
		return nil, fmt.Errorf("未能从供应商响应中提取图片，返回字段: [%s]", strings.Join(topKeys, ", "))
	}
	return nil, fmt.Errorf("供应商未返回有效图片数据")
}

func scanJSONLeaves(val gjson.Result, collect func(string)) {
	if val.IsArray() {
		for _, item := range val.Array() {
			scanJSONLeaves(item, collect)
		}
		return
	}
	if val.IsObject() {
		val.ForEach(func(k, v gjson.Result) bool {
			scanJSONLeaves(v, collect)
			return true
		})
		return
	}
	if val.Type == gjson.String {
		s := strings.TrimSpace(val.String())
		if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "data:image/") {
			if _, err := url.Parse(s); err == nil {
				collect(s)
			}
		}
	}
}

// RequestParams holds generic parameters for building an AI Gateway request payload.
type RequestParams struct {
	Prompt           string
	ResolvedImages   []string
	Provider         string
	Model            string
	Operation        workflow.RuntimeOperation
	CustomParamsJSON string
}

// BuildRequestBody builds an adaptive JSON payload for the target provider and model.
func (t *GatewayTranslator) BuildRequestBody(params RequestParams) ([]byte, error) {
	return BuildRequestBody(params)
}

// imagePayloadBuilder renders one vendor payload shape.
type imagePayloadBuilder func(params RequestParams) map[string]interface{}

// selectImagePayloadBuilder picks the vendor shape; extraction order is part
// of the contract (chat first, then fal, then edit, then generation).
func selectImagePayloadBuilder(params RequestParams) imagePayloadBuilder {
	providerLower := strings.ToLower(strings.TrimSpace(params.Provider))
	modelLower := strings.ToLower(strings.TrimSpace(params.Model))

	// Strategy A: Chat Multimodal (e.g. OpenRouter or models with chat/vision)
	if workflow.IsChatModel(params.Provider, params.Model, params.CustomParamsJSON) {
		return buildChatPayload
	}
	// Strategy B: Fal.ai Media Route (supports Generation, Inpaint and Outpaint)
	if providerLower == "fal" || strings.Contains(modelLower, "fal") {
		return buildFalPayload
	}
	// Strategy C: Universal Inpaint / Edit shape (compatible with both OpenAI and HTTP JSON inpaint routes)
	if len(params.ResolvedImages) > 0 && (params.Operation == workflow.RuntimeOperationInpaint || params.Operation == workflow.RuntimeOperationOutpaint) {
		return buildEditPayload
	}
	// Strategy D: Standard Universal Image Generation (OpenAI / Compatible)
	return buildGenerationPayload
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

func buildChatPayload(params RequestParams) map[string]interface{} {
	content := []chatContentPart{{Type: "text", Text: params.Prompt}}
	for _, u := range params.ResolvedImages {
		content = append(content, chatContentPart{
			Type:     "image_url",
			ImageURL: &chatImageURL{URL: u},
		})
	}
	return map[string]interface{}{
		"model": params.Model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": content},
		},
		"modalities": []string{"text", "image"},
	}
}

func buildFalPayload(params RequestParams) map[string]interface{} {
	body := map[string]interface{}{
		"prompt": params.Prompt,
	}
	if len(params.ResolvedImages) > 0 {
		body["image_urls"] = params.ResolvedImages
		body["image_url"] = params.ResolvedImages[0]
		if len(params.ResolvedImages) > 1 {
			body["mask_url"] = params.ResolvedImages[1]
		}
	}
	return body
}

func buildEditPayload(params RequestParams) map[string]interface{} {
	body := map[string]interface{}{
		"model":     params.Model,
		"prompt":    params.Prompt,
		"image":     params.ResolvedImages[0],
		"image_url": params.ResolvedImages[0],
	}
	if len(params.ResolvedImages) > 1 {
		body["mask"] = params.ResolvedImages[1]
		body["mask_url"] = params.ResolvedImages[1]
	}
	return body
}

func buildGenerationPayload(params RequestParams) map[string]interface{} {
	body := map[string]interface{}{
		"model":  params.Model,
		"prompt": params.Prompt,
		"n":      1,
	}
	if len(params.ResolvedImages) > 0 {
		body["image_url"] = params.ResolvedImages[0]
	}
	return body
}

// BuildRequestBody is the standalone domain function for building gateway request bodies.
func BuildRequestBody(params RequestParams) ([]byte, error) {
	baseBody := selectImagePayloadBuilder(params)(params)

	encoded, err := json.Marshal(baseBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求基础结构失败: %w", err)
	}

	// Dynamic Injection: Merge custom parameters via sjson if provided.
	// Non-object JSON is ignored (never an error): dialog and catalog
	// validation already reject it upstream; here robustness wins and the
	// skip is logged for diagnosis instead of failing silently.
	if trimmedCustom := strings.TrimSpace(params.CustomParamsJSON); trimmedCustom != "" && trimmedCustom != "{}" {
		customParsed := gjson.Parse(trimmedCustom)
		if !customParsed.IsObject() {
			slog.Debug("ignoring non-object custom params", "provider", params.Provider, "model", params.Model)
		} else {
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
				return nil, fmt.Errorf("注入自定义参数失败: %w", mergeErr)
			}
		}
	}

	return encoded, nil
}
