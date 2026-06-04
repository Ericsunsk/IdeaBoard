package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/mediarefs"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

const maxOpenAIDataURLBytes = 10 * 1024 * 1024

type openAIImageURLRef struct {
	ImageURL string `json:"image_url"`
}

type openAIImageRequestBody struct {
	Model     string
	Prompt    string
	Size      string
	Image     string
	ImageURLs []string
	Images    []openAIImageURLRef
	Mask      *openAIImageURLRef
	Extra     map[string]interface{}
}

func newOpenAIImageRequestBody(providerReq providerRequest) openAIImageRequestBody {
	return openAIImageRequestBody{
		Model:  providerReq.Model,
		Prompt: providerReq.Prompt,
		Size:   providerReq.Size,
		Extra:  map[string]interface{}{},
	}
}

func (b openAIImageRequestBody) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.toMap())
}

func (b openAIImageRequestBody) toMap() map[string]interface{} {
	out := make(map[string]interface{}, len(b.Extra)+7)
	for key, value := range b.Extra {
		out[key] = value
	}
	if b.Model != "" {
		out["model"] = b.Model
	}
	if b.Prompt != "" {
		out["prompt"] = b.Prompt
	}
	if b.Size != "" {
		out["size"] = b.Size
	}
	if b.Image != "" {
		out["image"] = b.Image
	}
	if len(b.ImageURLs) > 0 {
		out["image_urls"] = b.ImageURLs
	}
	if len(b.Images) > 0 {
		out["images"] = b.Images
	}
	if b.Mask != nil && strings.TrimSpace(b.Mask.ImageURL) != "" {
		out["mask"] = b.Mask
	}
	return out
}

func (b *openAIImageRequestBody) setExtra(key string, value interface{}) {
	if b.Extra == nil {
		b.Extra = map[string]interface{}{}
	}
	b.Extra[key] = value
}

// callOpenAI 调用 OpenAI DALL-E 风格接口（也兼容其他 OpenAI 兼容平台）
func (c *ProviderExecutor) callOpenAI(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration, params map[string]interface{}, resolvedModel string) (providerCallResult, error) {
	req := newProviderRequest(prompt, width, height, params, resolvedModel)
	return c.callOpenAIWithRequest(ctx, account, req, timeout)
}

func (c *ProviderExecutor) callOpenAIWithRequest(ctx context.Context, account *accounts.ApiAccount, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	baseURL := "https://api.openai.com"
	if account.ProxyURL != "" {
		baseURL = strings.TrimRight(account.ProxyURL, "/")
	}
	validatedBaseURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHostsFromEnv()...)
	if validateErr != nil {
		return providerCallResult{}, validateErr
	}
	baseURL = validatedBaseURL

	model := strings.TrimSpace(providerReq.Model)

	size := providerReq.Size
	useEditEndpoint := shouldUseOpenAIEditEndpoint(providerReq.Params)
	imageInputCount := len(providerReq.MediaRefs.OpenAIImages)
	featureSlug, _ := workflow.ReadWorkflowStringParam(providerReq.Params, "feature")

	reqBody := newOpenAIImageRequestBody(providerReq)

	if useEditEndpoint {
		appendOpenAIEditParams(&reqBody, providerReq)
	} else {
		appendOpenAIAllowedParams(&reqBody, providerReq)
	}
	finalizeOpenAIRequestBody(&reqBody, providerReq.Params)

	// 如果 ProxyURL 已包含 /v1，则不再拼接 /v1
	endpoint := baseURL + "/v1/images/generations"
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/images/generations"
	}
	if useEditEndpoint {
		endpoint = baseURL + "/v1/images/edits"
		if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/images/edits"
		}
	}

	var req *http.Request
	var err error
	if useEditEndpoint {
		body, contentType, buildErr := buildOpenAIEditMultipartRequestBody(ctx, reqBody, timeout)
		if buildErr != nil {
			return providerCallResult{}, buildErr
		}
		req, err = http.NewRequestWithContext(ctx, "POST", endpoint, body)
		if err != nil {
			return providerCallResult{}, err
		}
		req.Header.Set("Content-Type", contentType)
	} else {
		bodyJSON, marshalErr := json.Marshal(reqBody)
		if marshalErr != nil {
			return providerCallResult{}, fmt.Errorf("序列化 OpenAI 请求失败: %w", marshalErr)
		}
		req, err = http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyJSON))
		if err != nil {
			return providerCallResult{}, err
		}
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)

	slog.Info("OpenAI-compatible image request", "feature", featureSlug, "endpoint", redactLogURL(endpoint), "model", model, "size", size, "edit", useEditEndpoint, "images", imageInputCount)

	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHostsFromEnv()...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("读取 API 响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return providerCallResult{}, newUpstreamHTTPError(resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	imageURLs, err := extractOpenAIImageResponses(respBody, providerReq.Params)
	if err != nil {
		return providerCallResult{}, err
	}
	return providerCallResult{
		ImageURL:  imageURLs[0],
		ImageURLs: imageURLs,
		Model:     model,
	}, nil
}

func buildOpenAIEditMultipartRequestBody(ctx context.Context, reqBody openAIImageRequestBody, timeout time.Duration) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range reqBody.toMap() {
		if key == "images" || key == "mask" {
			continue
		}
		if err := writeOpenAIMultipartField(writer, key, value); err != nil {
			return nil, "", err
		}
	}

	imageRefs := openAIImageURLRefStrings(reqBody.Images)
	for index, ref := range imageRefs {
		if err := writeOpenAIMultipartImagePart(ctx, writer, "image", ref, index, timeout); err != nil {
			return nil, "", err
		}
	}
	if len(imageRefs) == 0 {
		return nil, "", fmt.Errorf("OpenAI 编辑请求缺少 image")
	}

	if reqBody.Mask != nil && strings.TrimSpace(reqBody.Mask.ImageURL) != "" {
		maskRef := reqBody.Mask.ImageURL
		if err := writeOpenAIMultipartImagePart(ctx, writer, "mask", maskRef, 0, timeout); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &body, writer.FormDataContentType(), nil
}

func writeOpenAIMultipartField(writer *multipart.Writer, key string, value interface{}) error {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return writer.WriteField(key, typed)
	case bool:
		return writer.WriteField(key, strconv.FormatBool(typed))
	case float32:
		return writer.WriteField(key, strconv.FormatFloat(float64(typed), 'f', -1, 32))
	case float64:
		return writer.WriteField(key, strconv.FormatFloat(typed, 'f', -1, 64))
	case int:
		return writer.WriteField(key, strconv.Itoa(typed))
	case int8:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int16:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int32:
		return writer.WriteField(key, strconv.FormatInt(int64(typed), 10))
	case int64:
		return writer.WriteField(key, strconv.FormatInt(typed, 10))
	case uint:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint8:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint16:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint32:
		return writer.WriteField(key, strconv.FormatUint(uint64(typed), 10))
	case uint64:
		return writer.WriteField(key, strconv.FormatUint(typed, 10))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("序列化 multipart 字段失败 %s: %w", key, err)
		}
		return writer.WriteField(key, string(encoded))
	}
}

func writeOpenAIMultipartImagePart(ctx context.Context, writer *multipart.Writer, fieldName, ref string, index int, timeout time.Duration) error {
	filename, mimeType, data, err := resolveOpenAIImageBytes(ctx, strings.TrimSpace(ref), index, timeout)
	if err != nil {
		return err
	}
	filename, mimeType, data, err = normalizeOpenAIEditMultipartImage(filename, mimeType, data)
	if err != nil {
		return fmt.Errorf("归一化图像失败: %w", err)
	}

	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
	headers.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

func resolveOpenAIImageBytes(ctx context.Context, ref string, index int, timeout time.Duration) (filename string, mimeType string, data []byte, err error) {
	if strings.HasPrefix(ref, "data:") {
		parsed, decodeErr := mediarefs.ParseDataImageURL(ref, maxOpenAIDataURLBytes)
		if decodeErr != nil {
			return "", "", nil, fmt.Errorf("解析 data URL 失败: %w", decodeErr)
		}
		ext := strings.TrimPrefix(parsed.Ext, ".")
		return fmt.Sprintf("image-%d.%s", index+1, ext), parsed.MIMEType, parsed.Data, nil
	}

	parsed, parseErr := url.Parse(ref)
	if parseErr != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", nil, fmt.Errorf("不支持的图像引用: %q", ref)
	}

	allowHosts := trustedOpenAIAssetHosts()
	resp, doErr := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if doErr != nil {
		return "", "", nil, fmt.Errorf("拉取图像失败: %w", doErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimitedBody(resp.Body)
		return "", "", nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}

	body, readErr := readLimitedBody(resp.Body)
	if readErr != nil {
		return "", "", nil, readErr
	}
	if len(body) == 0 {
		return "", "", nil, fmt.Errorf("拉取图像为空: %q", ref)
	}

	detectedMimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if parsedMimeType, _, parseMimeErr := mime.ParseMediaType(detectedMimeType); parseMimeErr == nil && strings.HasPrefix(parsedMimeType, "image/") {
		detectedMimeType = parsedMimeType
	} else {
		detectedMimeType = inferLocalAssetMimeType(path.Base(parsed.Path), body)
	}

	filename = path.Base(parsed.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = fmt.Sprintf("image-%d.%s", index+1, inferOpenAIImageFileExtension(detectedMimeType))
	}
	if ext := strings.TrimSpace(path.Ext(filename)); ext == "" {
		filename += "." + inferOpenAIImageFileExtension(detectedMimeType)
	}

	return filename, detectedMimeType, body, nil
}

func inferOpenAIImageFileExtension(mimeType string) string {
	if ext := media.ImageExtensionFromMIME(mimeType); ext != "" {
		return strings.TrimPrefix(ext, ".")
	}
	return "png"
}

func openAIImageURLRefStrings(refs []openAIImageURLRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if text := strings.TrimSpace(ref.ImageURL); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func openAIImageURLRefs(refs []string) []openAIImageURLRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]openAIImageURLRef, 0, len(refs))
	for _, ref := range refs {
		if text := strings.TrimSpace(ref); text != "" {
			out = append(out, openAIImageURLRef{ImageURL: text})
		}
	}
	return out
}

func appendOpenAIAllowedParams(reqBody *openAIImageRequestBody, providerReq providerRequest) {
	if providerReq.Params == nil {
		return
	}

	if imageURLs := normalizeOpenAIImageURLStrings(providerReq.MediaRefs.OpenAIImages); len(imageURLs) > 0 {
		// 默认保留首图 image 字段以兼容部分只接收单图的上游实现。
		// 但多图输入必须避免回退成单图字段，防止上游优先读取首图导致退化。
		if len(imageURLs) == 1 {
			reqBody.Image = imageURLs[0]
		}
		reqBody.ImageURLs = imageURLs
	}

	for _, key := range openAISharedAllowedParamKeys() {
		if v, ok := providerReq.Params[key]; ok {
			reqBody.setExtra(key, v)
		}
	}
}

func openAISharedAllowedParamKeys() []string {
	return []string{
		"variation_strength",
		"viewpoint",
		"extend_top",
		"extend_right",
		"extend_bottom",
		"extend_left",
		"original_image_url",
		"upscale_mode",
		"upscale_factor",
		"output_format",
		"output_quality",
	}
}

func finalizeOpenAIRequestBody(reqBody *openAIImageRequestBody, params map[string]interface{}) {
	if reqBody == nil || params == nil {
		return
	}
	// 兜底保护：当 image_urls 已是多图时，确保不会携带退化的单图 image 字段。
	if len(reqBody.ImageURLs) > 1 {
		reqBody.Image = ""
	}
	image := reqBody.Image
	if image == "" || !strings.HasPrefix(image, "data:") {
		return
	}
	if len(reqBody.ImageURLs) == 1 && reqBody.ImageURLs[0] == image {
		reqBody.ImageURLs = nil
	}
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	if featureSlug != workflow.FeatureSlugStyleVariation {
		return
	}
	if original, ok := reqBody.Extra["original_image_url"].(string); ok && original == image {
		delete(reqBody.Extra, "original_image_url")
	}
}

func shouldUseOpenAIEditEndpoint(params map[string]interface{}) bool {
	if len(collectOpenAIEditImageRefs(params)) == 0 {
		return false
	}
	featureSlug, ok := workflow.ReadWorkflowStringParam(params, "feature")
	if !ok {
		return false
	}
	if featureSlug == workflow.FeatureSlugImageExtend ||
		featureSlug == workflow.FeatureSlugPartialEdit ||
		featureSlug == workflow.FeatureSlugRemoveBg ||
		featureSlug == workflow.FeatureSlugStyleFusion {
		return true
	}
	return false
}

func trustedOpenAIAssetHosts() []string {
	base := resolvePublicAssetBaseURL()
	if base == nil || strings.TrimSpace(base.Hostname()) == "" {
		return nil
	}
	return []string{base.Hostname()}
}

func normalizeOpenAIImageURLStrings(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	urls := make([]string, 0, len(refs))
	for _, ref := range refs {
		url := strings.TrimSpace(ref)
		if url == "" {
			continue
		}
		urls = append(urls, url)
	}
	return urls
}

func appendOpenAIEditParams(reqBody *openAIImageRequestBody, providerReq providerRequest) {
	if providerReq.Params == nil {
		return
	}
	if refs := openAIImageURLRefs(providerReq.MediaRefs.OpenAIImages); len(refs) > 0 {
		reqBody.Images = refs
	}
	if maskURL, ok := workflow.ReadWorkflowStringParam(providerReq.Params, "mask_image_url"); ok {
		reqBody.Mask = &openAIImageURLRef{ImageURL: maskURL}
	}

	allowedKeys := openAISharedAllowedParamKeys()
	for _, key := range allowedKeys {
		if v, ok := providerReq.Params[key]; ok {
			reqBody.setExtra(key, v)
		}
	}
}

func collectOpenAIEditImageRefs(params map[string]interface{}) []string {
	return mediarefs.CollectUniqueParamRefs(params, openAIImageInputRefKeys)
}

func extractOpenAIImageResponses(respBody []byte, params map[string]interface{}) ([]string, error) {
	var respData struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	if len(respData.Data) == 0 {
		return nil, fmt.Errorf("API 未返回图片")
	}
	urls := make([]string, 0, len(respData.Data))
	for _, item := range respData.Data {
		if strings.TrimSpace(item.URL) != "" {
			urls = append(urls, strings.TrimSpace(item.URL))
			continue
		}
		if strings.TrimSpace(item.B64JSON) != "" {
			urls = append(urls, "data:"+inferOpenAIImageMimeType(params)+";base64,"+strings.TrimSpace(item.B64JSON))
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("API 未返回可用图片")
	}
	return urls, nil
}

func inferOpenAIImageMimeType(params map[string]interface{}) string {
	if params != nil {
		if format, ok := params["output_format"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "jpg", "jpeg":
				return "image/jpeg"
			case "webp":
				return "image/webp"
			}
		}
	}
	return "image/png"
}
