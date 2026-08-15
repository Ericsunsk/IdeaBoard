package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/mediarefs"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
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
	N         int
	Extra     map[string]interface{}
}

type openAIProviderParams struct {
	EditImageRefs        []string
	MaskImageRef         string
	OriginalImageRef     string
	UseEditEndpoint      bool
	DropOriginalImageRef bool
	VariationStrength    *float64
	ExtendTop            *float64
	ExtendRight          *float64
	ExtendBottom         *float64
	ExtendLeft           *float64
	HDOutputQuality      *int
}

type openAIRequestFinalizer struct {
	DropOriginalImageRef bool
}

func (f openAIRequestFinalizer) apply(reqBody *openAIImageRequestBody) {
	if reqBody == nil {
		return
	}
	f.dropSingleImageFallbackForMultipleInputs(reqBody)
	f.dropRedundantDataURLImageList(reqBody)
	f.dropRedundantStyleVariationOriginalRef(reqBody)
}

func (f openAIRequestFinalizer) dropSingleImageFallbackForMultipleInputs(reqBody *openAIImageRequestBody) {
	if len(reqBody.ImageURLs) > 1 {
		reqBody.Image = ""
	}
}

func (f openAIRequestFinalizer) dropRedundantDataURLImageList(reqBody *openAIImageRequestBody) {
	image := reqBody.Image
	if image == "" || !strings.HasPrefix(image, "data:") {
		return
	}
	if len(reqBody.ImageURLs) == 1 && reqBody.ImageURLs[0] == image {
		reqBody.ImageURLs = nil
	}
}

func (f openAIRequestFinalizer) dropRedundantStyleVariationOriginalRef(reqBody *openAIImageRequestBody) {
	if !f.DropOriginalImageRef {
		return
	}
	delete(reqBody.Extra, "original_image_url")
}

func readOpenAIProviderParams(providerReq providerRequest) openAIProviderParams {
	out := openAIProviderParams{
		EditImageRefs:        normalizeOpenAIImageURLStrings(providerReq.ImageRefs),
		OriginalImageRef:     providerReq.CanvasRef,
		UseEditEndpoint:      providerReq.OpenAI.UseEditEndpoint || providerReq.Primitive == "image-to-image",
		DropOriginalImageRef: providerReq.OpenAI.DropOriginalImageRef,
		VariationStrength:    providerReq.OpenAI.VariationStrength,
		ExtendTop:            providerReq.OpenAI.ExtendTop,
		ExtendRight:          providerReq.OpenAI.ExtendRight,
		ExtendBottom:         providerReq.OpenAI.ExtendBottom,
		ExtendLeft:           providerReq.OpenAI.ExtendLeft,
		HDOutputQuality:      providerReq.OpenAI.HDOutputQuality,
	}
	if len(providerReq.MaskRefs) > 0 {
		out.MaskImageRef = strings.TrimSpace(providerReq.MaskRefs[0])
	}
	return out
}

func (p openAIProviderParams) useEditEndpoint() bool {
	return p.UseEditEndpoint && len(p.EditImageRefs) > 0
}

func (p openAIProviderParams) applySharedExtras(reqBody *openAIImageRequestBody) {
	if p.VariationStrength != nil {
		reqBody.setExtra("variation_strength", *p.VariationStrength)
	}
	for key, value := range map[string]*float64{
		"extend_top":    p.ExtendTop,
		"extend_right":  p.ExtendRight,
		"extend_bottom": p.ExtendBottom,
		"extend_left":   p.ExtendLeft,
	} {
		if value != nil {
			reqBody.setExtra(key, *value)
		}
	}
	if p.OriginalImageRef != "" {
		reqBody.setExtra("original_image_url", p.OriginalImageRef)
	}
	if p.HDOutputQuality != nil {
		reqBody.setExtra("output_quality", *p.HDOutputQuality)
	}
}

func newOpenAIImageRequestBody(providerReq providerRequest) openAIImageRequestBody {
	return openAIImageRequestBody{
		Model:  providerReq.Model,
		Prompt: providerReq.Prompt,
		Size:   providerReq.Size,
		Extra:  map[string]interface{}{},
		N:      providerReq.OutputCount,
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
	if b.N > 1 {
		out["n"] = b.N
	}
	return out
}

func (b *openAIImageRequestBody) setExtra(key string, value interface{}) {
	if b.Extra == nil {
		b.Extra = map[string]interface{}{}
	}
	b.Extra[key] = value
}

func (c *ProviderExecutor) callOpenAIWithRequest(ctx context.Context, account *accounts.ApiAccount, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	baseURL := "https://api.openai.com"
	if account.ProxyURL != "" {
		baseURL = strings.TrimRight(account.ProxyURL, "/")
	}
	validatedBaseURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHosts(ctx)...)
	if validateErr != nil {
		return providerCallResult{}, validateErr
	}
	baseURL = validatedBaseURL

	model := strings.TrimSpace(providerReq.Model)

	size := providerReq.Size
	openAIParams := readOpenAIProviderParams(providerReq)
	useEditEndpoint := openAIParams.useEditEndpoint()
	imageInputCount := len(providerReq.ImageRefs)

	reqBody := newOpenAIImageRequestBody(providerReq)

	if useEditEndpoint {
		appendOpenAIEditParams(&reqBody, openAIParams)
	} else {
		appendOpenAIAllowedParams(&reqBody, providerReq, openAIParams)
	}
	finalizeOpenAIRequestBody(&reqBody, openAIParams)

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
		body, contentType, buildErr := c.buildOpenAIEditMultipartRequestBody(ctx, reqBody, timeout)
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

	slog.Info("OpenAI-compatible image request", "endpoint", redactLogURL(endpoint), "model", model, "size", size, "edit", useEditEndpoint, "images", imageInputCount)

	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHosts(ctx)...)
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
		return providerCallResult{}, newUpstreamHTTPErrorFromResponse(resp, truncateErrorBody(string(respBody)))
	}

	imageURLs, err := extractOpenAIImageResponses(respBody)
	if err != nil {
		return providerCallResult{
			Model:            model,
			RawOutputPreview: providerRawOutputPreview(respBody),
			ParsedOutputKind: "image-results",
		}, newProviderOutputParseError(err)
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		RawOutputPreview:  providerRawOutputPreview(respBody),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

func (c *ProviderExecutor) buildOpenAIEditMultipartRequestBody(ctx context.Context, reqBody openAIImageRequestBody, timeout time.Duration) (*bytes.Buffer, string, error) {
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
		if err := c.writeOpenAIMultipartImagePart(ctx, writer, "image", ref, index, timeout); err != nil {
			return nil, "", err
		}
	}
	if len(imageRefs) == 0 {
		return nil, "", fmt.Errorf("OpenAI 编辑请求缺少 image")
	}

	if reqBody.Mask != nil && strings.TrimSpace(reqBody.Mask.ImageURL) != "" {
		maskRef := reqBody.Mask.ImageURL
		if err := c.writeOpenAIMultipartImagePart(ctx, writer, "mask", maskRef, 0, timeout); err != nil {
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

func (c *ProviderExecutor) writeOpenAIMultipartImagePart(ctx context.Context, writer *multipart.Writer, fieldName, ref string, index int, timeout time.Duration) error {
	filename, mimeType, data, err := c.resolveOpenAIImageBytes(ctx, strings.TrimSpace(ref), index, timeout)
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

func (c *ProviderExecutor) resolveOpenAIImageBytes(ctx context.Context, ref string, index int, timeout time.Duration) (filename string, mimeType string, data []byte, err error) {
	if strings.HasPrefix(ref, "data:") {
		parsed, decodeErr := mediarefs.ParseDataImageURL(ref, maxOpenAIDataURLBytes)
		if decodeErr != nil {
			return "", "", nil, fmt.Errorf("解析 data URL 失败: %w", decodeErr)
		}
		ext := strings.TrimPrefix(parsed.Ext, ".")
		return fmt.Sprintf("image-%d.%s", index+1, ext), parsed.MIMEType, parsed.Data, nil
	}

	if obj, ok, readErr := c.readRelativeUploadObject(ctx, ref, maxOpenAIDataURLBytes); ok {
		if readErr != nil {
			return "", "", nil, fmt.Errorf("读取上传图像失败: %w", readErr)
		}
		if err := imageformat.ValidateImageDimensions(obj.Data); err != nil {
			return "", "", nil, err
		}
		mimeType = strings.TrimSpace(obj.ContentType)
		if parsedMimeType, _, parseMimeErr := mime.ParseMediaType(mimeType); parseMimeErr == nil && strings.HasPrefix(parsedMimeType, "image/") {
			mimeType = parsedMimeType
		} else {
			mimeType = inferLocalAssetMimeType(obj.Filename, obj.Data)
		}
		filename = strings.TrimSpace(obj.Filename)
		if filename == "" || filename == "." || filename == "/" {
			filename = fmt.Sprintf("image-%d.%s", index+1, inferOpenAIImageFileExtension(mimeType))
		}
		if ext := strings.TrimSpace(path.Ext(filename)); ext == "" {
			filename += "." + inferOpenAIImageFileExtension(mimeType)
		}
		return filename, mimeType, obj.Data, nil
	}

	parsed, parseErr := url.Parse(ref)
	if parseErr != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", nil, fmt.Errorf("不支持的图像引用: %q", ref)
	}

	allowHosts := trustedRuntimeAssetHosts(c.publicOrigins)
	resp, doErr := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if doErr != nil {
		return "", "", nil, fmt.Errorf("拉取图像失败: %w", doErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := readLimitedImageRef(resp.Body)
		if readErr != nil {
			slog.Warn("read error response body failed", "err", readErr)
		}
		return "", "", nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}

	body, readErr := readLimitedImageRef(resp.Body)
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
	if ext := imageformat.ImageExtensionFromMIME(mimeType); ext != "" {
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

func appendOpenAIAllowedParams(reqBody *openAIImageRequestBody, providerReq providerRequest, openAIParams openAIProviderParams) {
	if imageURLs := openAIParams.EditImageRefs; len(imageURLs) > 0 {
		if len(imageURLs) == 1 {
			reqBody.Image = imageURLs[0]
		}
		reqBody.ImageURLs = imageURLs
	}

	openAIParams.applySharedExtras(reqBody)
}

func resolveOpenAIHDUpscaleOutputQuality(detailStrength float64) int {
	detailStrength = math.Max(0, math.Min(100, math.Round(detailStrength)))
	return int(math.Max(60, math.Min(100, math.Round(60+detailStrength*0.4))))
}

func finalizeOpenAIRequestBody(reqBody *openAIImageRequestBody, openAIParams openAIProviderParams) {
	openAIRequestFinalizer{DropOriginalImageRef: openAIParams.DropOriginalImageRef}.apply(reqBody)
}

func trustedRuntimeAssetHosts(publicOrigins config.PublicOriginsConfig) []string {
	base := publicOrigins.APIParsedURL()
	if base == nil || strings.TrimSpace(base.Host) == "" {
		return nil
	}
	return []string{base.Host}
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

func appendOpenAIEditParams(reqBody *openAIImageRequestBody, openAIParams openAIProviderParams) {
	if refs := openAIImageURLRefs(openAIParams.EditImageRefs); len(refs) > 0 {
		reqBody.Images = refs
	}
	if openAIParams.MaskImageRef != "" {
		reqBody.Mask = &openAIImageURLRef{ImageURL: openAIParams.MaskImageRef}
	}

	openAIParams.applySharedExtras(reqBody)
}

func extractOpenAIImageResponses(respBody []byte) ([]string, error) {
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
			urls = append(urls, "data:image/png;base64,"+strings.TrimSpace(item.B64JSON))
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("API 未返回可用图片")
	}
	return urls, nil
}
