package worker

import (
	"api/internal/accounts"
	"api/internal/platform/safehttp"
	"api/internal/storage"
	"api/internal/workflow"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	localRedrawEnabledParam = "__local_redraw_enabled"
	maskLuminanceThreshold  = 16
)

type localRedrawContext struct {
	featureSlug string
	original    *image.NRGBA
	maskSampler maskCoverageSampler
	roi         image.Rectangle
	allowHosts  []string
}

type maskCoverageSampler struct {
	mask         image.Image
	maskBounds   image.Rectangle
	dstWidth     int
	dstHeight    int
	useLuminance bool
}

func (s maskCoverageSampler) coverageAt(x, y int) uint8 {
	if s.mask == nil || s.dstWidth <= 0 || s.dstHeight <= 0 {
		return 0
	}
	if x < 0 || y < 0 || x >= s.dstWidth || y >= s.dstHeight {
		return 0
	}
	mx := s.maskBounds.Min.X + (x*s.maskBounds.Dx())/s.dstWidth
	my := s.maskBounds.Min.Y + (y*s.maskBounds.Dy())/s.dstHeight
	if mx >= s.maskBounds.Max.X {
		mx = s.maskBounds.Max.X - 1
	}
	if my >= s.maskBounds.Max.Y {
		my = s.maskBounds.Max.Y - 1
	}
	r, g, b, a := s.mask.At(mx, my).RGBA()
	if !s.useLuminance {
		return uint8(a >> 8)
	}
	luminance := (299*r + 587*g + 114*b) / 1000
	return uint8(luminance >> 8)
}

func (c *Consumer) prepareLocalRedrawPipeline(ctx context.Context, account *accounts.ApiAccount, params map[string]interface{}, timeout time.Duration) (*localRedrawContext, error) {
	if !shouldUseLocalRedrawPipeline(params) {
		return nil, nil
	}
	featureSlug, _ := workflow.ReadWorkflowStringParam(params, "feature")
	originalRef, ok := readLocalRedrawOriginalImageRef(params)
	if !ok {
		return nil, fmt.Errorf("local redraw 缺少原图: feature=%s", featureSlug)
	}
	maskRef, ok := readLocalRedrawMaskRef(params)
	if !ok {
		return nil, fmt.Errorf("local redraw 缺少蒙版: feature=%s", featureSlug)
	}

	allowHosts := buildLocalRedrawAllowHosts(account)
	originalImage, err := loadImageFromRef(ctx, originalRef, timeout, allowHosts)
	if err != nil {
		return nil, fmt.Errorf("local redraw 读取原图失败: feature=%s err=%w", featureSlug, err)
	}
	maskImage, err := loadImageFromRef(ctx, maskRef, timeout, allowHosts)
	if err != nil {
		return nil, fmt.Errorf("local redraw 读取蒙版失败: feature=%s err=%w", featureSlug, err)
	}

	originalNRGBA := cloneImageToNRGBA(originalImage)
	if originalNRGBA.Bounds().Dx() == 0 || originalNRGBA.Bounds().Dy() == 0 {
		return nil, fmt.Errorf("local redraw 原图尺寸无效: feature=%s", featureSlug)
	}

	sampler, bbox, err := buildLocalRedrawMaskSampler(maskImage, originalNRGBA.Bounds())
	if err != nil {
		return nil, fmt.Errorf("local redraw 计算蒙版区域失败: feature=%s err=%w", featureSlug, err)
	}
	roi, err := expandBBoxWithPadding(bbox, originalNRGBA.Bounds())
	if err != nil {
		return nil, fmt.Errorf("local redraw 裁剪区域无效: feature=%s err=%w", featureSlug, err)
	}
	roiDataURL, err := encodeROIToDataURL(originalNRGBA, roi)
	if err != nil {
		return nil, fmt.Errorf("local redraw 裁剪 ROI 失败: feature=%s err=%w", featureSlug, err)
	}

	if err := rewriteLocalRedrawUpstreamInputs(params, featureSlug, roiDataURL, roi); err != nil {
		return nil, err
	}
	stripLocalRedrawMaskParams(params)
	params[localRedrawEnabledParam] = true

	return &localRedrawContext{
		featureSlug: featureSlug,
		original:    originalNRGBA,
		maskSampler: sampler,
		roi:         roi,
		allowHosts:  allowHosts,
	}, nil
}

func (c *Consumer) finalizeLocalRedrawResult(ctx context.Context, redraw *localRedrawContext, upstreamImageRef string, timeout time.Duration) (string, bool, error) {
	if redraw == nil {
		return upstreamImageRef, false, nil
	}
	upstreamImage, err := loadImageFromRef(ctx, upstreamImageRef, timeout, redraw.allowHosts)
	if err != nil {
		return "", false, fmt.Errorf("local redraw 读取上游结果失败: feature=%s err=%w", redraw.featureSlug, err)
	}
	roiWidth := redraw.roi.Dx()
	roiHeight := redraw.roi.Dy()
	if roiWidth <= 0 || roiHeight <= 0 {
		return "", false, fmt.Errorf("local redraw 回贴 ROI 尺寸无效: feature=%s", redraw.featureSlug)
	}

	upstreamScaled := scaleImageToNRGBA(upstreamImage, roiWidth, roiHeight)
	composed, err := applyMaskedComposite(redraw.original, redraw.maskSampler, redraw.roi, upstreamScaled)
	if err != nil {
		return "", false, fmt.Errorf("local redraw 回贴失败: feature=%s err=%w", redraw.featureSlug, err)
	}
	composedBytes, err := encodeImagePNG(composed)
	if err != nil {
		return "", false, fmt.Errorf("local redraw 编码结果失败: feature=%s err=%w", redraw.featureSlug, err)
	}

	if c.storageService == nil {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(composedBytes), false, nil
	}
	filename := fmt.Sprintf("local-redraw-%s-%d.png", sanitizeFeatureSlug(redraw.featureSlug), time.Now().UnixNano())
	storedURL, err := c.storageService.StoreBytes(filename, composedBytes, storage.StorageStoreOptions{})
	if err != nil {
		return "", false, fmt.Errorf("local redraw 持久化失败: feature=%s err=%w", redraw.featureSlug, err)
	}
	return storedURL, true, nil
}

func shouldUseLocalRedrawPipeline(params map[string]interface{}) bool {
	if params == nil {
		return false
	}
	featureSlug, ok := workflow.ReadWorkflowStringParam(params, "feature")
	if !ok {
		return false
	}
	if featureSlug == workflow.FeatureSlugFaceSwap {
		return false
	}
	if !hasOpenAIMaskInput(params) {
		return false
	}
	switch featureSlug {
	case workflow.FeatureSlugDirectedRecolor,
		workflow.FeatureSlugSeriesColor,
		workflow.FeatureSlugPartialEdit,
		workflow.FeatureSlugBgSwap,
		workflow.FeatureSlugFabricOnBody,
		workflow.FeatureSlugHDUpscale:
		return true
	default:
		return false
	}
}

func buildLocalRedrawAllowHosts(account *accounts.ApiAccount) []string {
	hosts := map[string]struct{}{}
	appendHost := func(raw string) {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
			hosts[strings.TrimSpace(parsed.Hostname())] = struct{}{}
			return
		}
		host := strings.TrimSpace(raw)
		if host != "" {
			hosts[host] = struct{}{}
		}
	}

	appendHost(account.ProxyURL)
	for _, host := range trustedOpenAIAssetHosts() {
		appendHost(host)
	}
	out := make([]string, 0, len(hosts))
	for host := range hosts {
		out = append(out, host)
	}
	return out
}

func readLocalRedrawOriginalImageRef(params map[string]interface{}) (string, bool) {
	if ref, ok := workflow.ReadWorkflowStringParam(params, "original_image_url"); ok {
		return ref, true
	}
	for _, ref := range readImageURLValues(params) {
		if strings.TrimSpace(ref) != "" {
			return strings.TrimSpace(ref), true
		}
	}
	if ref, ok := workflow.ReadWorkflowStringParam(params, "image_url"); ok {
		return ref, true
	}
	return "", false
}

func readLocalRedrawMaskRef(params map[string]interface{}) (string, bool) {
	if ref, ok := workflow.ReadWorkflowStringParam(params, "mask_image_url"); ok {
		return ref, true
	}
	switch values := params["mask_image_urls"].(type) {
	case []string:
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed, true
			}
		}
	case []interface{}:
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				return trimmed, true
			}
		}
	}
	return "", false
}

func rewriteLocalRedrawUpstreamInputs(params map[string]interface{}, featureSlug, roiDataURL string, roi image.Rectangle) error {
	imageURLs := readImageURLValues(params)
	switch featureSlug {
	case workflow.FeatureSlugFabricOnBody:
		if len(imageURLs) == 0 {
			return fmt.Errorf("local redraw 缺少 body 图输入: feature=%s", featureSlug)
		}
		imageURLs[0] = roiDataURL
	default:
		imageURLs = []string{roiDataURL}
	}
	params["image_urls"] = toInterfaceSlice(imageURLs)
	params["image_url"] = roiDataURL
	params["original_image_url"] = roiDataURL
	params["custom_width"] = float64(roi.Dx())
	params["custom_height"] = float64(roi.Dy())
	return nil
}

func stripLocalRedrawMaskParams(params map[string]interface{}) {
	for _, key := range []string{
		"mask_image_url",
		"mask_image_urls",
		"use_mask",
	} {
		delete(params, key)
	}
}

func loadImageFromRef(ctx context.Context, ref string, timeout time.Duration, allowHosts []string) (image.Image, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return nil, fmt.Errorf("图像引用为空")
	}
	data, err := loadImageBytesFromRef(ctx, trimmed, timeout, allowHosts)
	if err != nil {
		return nil, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码图像失败: %w", err)
	}
	return decoded, nil
}

func loadImageBytesFromRef(ctx context.Context, ref string, timeout time.Duration, allowHosts []string) ([]byte, error) {
	if strings.HasPrefix(ref, "data:") {
		_, data, err := decodeOpenAIDataURL(ref)
		if err != nil {
			return nil, fmt.Errorf("解析 data URL 失败: %w", err)
		}
		return data, nil
	}
	resp, err := safehttp.GetWithAllowHosts(ctx, ref, timeout, allowHosts...)
	if err != nil {
		return nil, fmt.Errorf("拉取图像失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := readLimitedBody(resp.Body)
		return nil, fmt.Errorf("拉取图像返回 %d: %s", resp.StatusCode, truncateErrorBody(string(body)))
	}
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("拉取图像为空")
	}
	return body, nil
}

func buildLocalRedrawMaskSampler(mask image.Image, targetBounds image.Rectangle) (maskCoverageSampler, image.Rectangle, error) {
	sampler := maskCoverageSampler{
		mask:       mask,
		maskBounds: mask.Bounds(),
		dstWidth:   targetBounds.Dx(),
		dstHeight:  targetBounds.Dy(),
	}

	alphaBBox, hasAlpha := computeMaskBBox(targetBounds, sampler, 0)
	if hasAlpha && !isFullBounds(alphaBBox, targetBounds) {
		return sampler, alphaBBox, nil
	}

	sampler.useLuminance = true
	luminanceBBox, hasLuminance := computeMaskBBox(targetBounds, sampler, maskLuminanceThreshold)
	if hasLuminance {
		return sampler, luminanceBBox, nil
	}
	if hasAlpha {
		return sampler, alphaBBox, nil
	}
	return sampler, image.Rectangle{}, fmt.Errorf("bbox 为空")
}

func computeMaskBBox(targetBounds image.Rectangle, sampler maskCoverageSampler, threshold uint8) (image.Rectangle, bool) {
	minX := targetBounds.Max.X
	minY := targetBounds.Max.Y
	maxX := targetBounds.Min.X
	maxY := targetBounds.Min.Y
	found := false

	for y := targetBounds.Min.Y; y < targetBounds.Max.Y; y++ {
		for x := targetBounds.Min.X; x < targetBounds.Max.X; x++ {
			if sampler.coverageAt(x-targetBounds.Min.X, y-targetBounds.Min.Y) <= threshold {
				continue
			}
			if !found {
				minX, maxX = x, x+1
				minY, maxY = y, y+1
				found = true
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX, maxY), true
}

func expandBBoxWithPadding(bbox image.Rectangle, imageBounds image.Rectangle) (image.Rectangle, error) {
	if bbox.Empty() {
		return image.Rectangle{}, fmt.Errorf("bbox 为空")
	}
	maxSide := max(bbox.Dx(), bbox.Dy())
	pad := int(math.Round(float64(maxSide) * 0.25))
	pad = min(192, max(64, pad))

	expanded := image.Rect(
		max(imageBounds.Min.X, bbox.Min.X-pad),
		max(imageBounds.Min.Y, bbox.Min.Y-pad),
		min(imageBounds.Max.X, bbox.Max.X+pad),
		min(imageBounds.Max.Y, bbox.Max.Y+pad),
	)
	if expanded.Empty() {
		return image.Rectangle{}, fmt.Errorf("裁剪结果为空")
	}
	return expanded, nil
}

func encodeROIToDataURL(original *image.NRGBA, roi image.Rectangle) (string, error) {
	if original == nil {
		return "", fmt.Errorf("原图为空")
	}
	if roi.Empty() {
		return "", fmt.Errorf("ROI 为空")
	}
	roiImage := image.NewNRGBA(image.Rect(0, 0, roi.Dx(), roi.Dy()))
	draw.Draw(roiImage, roiImage.Bounds(), original, roi.Min, draw.Src)
	encoded, err := encodeImagePNG(roiImage)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded), nil
}

func encodeImagePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cloneImageToNRGBA(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

func scaleImageToNRGBA(src image.Image, width, height int) *image.NRGBA {
	if width <= 0 || height <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	srcBounds := src.Bounds()
	srcWidth := max(1, srcBounds.Dx())
	srcHeight := max(1, srcBounds.Dy())

	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := srcBounds.Min.Y + (y*srcHeight)/height
		if srcY >= srcBounds.Max.Y {
			srcY = srcBounds.Max.Y - 1
		}
		for x := 0; x < width; x++ {
			srcX := srcBounds.Min.X + (x*srcWidth)/width
			if srcX >= srcBounds.Max.X {
				srcX = srcBounds.Max.X - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func applyMaskedComposite(original *image.NRGBA, maskSampler maskCoverageSampler, roi image.Rectangle, generatedROI *image.NRGBA) (*image.NRGBA, error) {
	if original == nil || generatedROI == nil {
		return nil, fmt.Errorf("图像为空")
	}
	if roi.Empty() {
		return nil, fmt.Errorf("ROI 为空")
	}
	if roi.Dx() != generatedROI.Bounds().Dx() || roi.Dy() != generatedROI.Bounds().Dy() {
		return nil, fmt.Errorf("ROI 尺寸不匹配")
	}
	output := cloneImageToNRGBA(original)
	originalBounds := output.Bounds()
	if !roi.Eq(originalBounds.Intersect(roi)) {
		return nil, fmt.Errorf("ROI 超出原图范围")
	}

	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			maskAlpha := maskSampler.coverageAt(x-originalBounds.Min.X, y-originalBounds.Min.Y)
			if maskAlpha == 0 {
				continue
			}
			srcColor := generatedROI.NRGBAAt(x-roi.Min.X, y-roi.Min.Y)
			effectiveAlpha := int(maskAlpha) * int(srcColor.A) / 255
			if effectiveAlpha <= 0 {
				continue
			}

			dstColor := output.NRGBAAt(x-originalBounds.Min.X, y-originalBounds.Min.Y)
			blended := color.NRGBA{
				R: blendChannel(dstColor.R, srcColor.R, effectiveAlpha),
				G: blendChannel(dstColor.G, srcColor.G, effectiveAlpha),
				B: blendChannel(dstColor.B, srcColor.B, effectiveAlpha),
				A: blendChannel(dstColor.A, srcColor.A, int(maskAlpha)),
			}
			output.SetNRGBA(x-originalBounds.Min.X, y-originalBounds.Min.Y, blended)
		}
	}
	return output, nil
}

func blendChannel(dst, src uint8, alpha int) uint8 {
	if alpha <= 0 {
		return dst
	}
	if alpha >= 255 {
		return src
	}
	return uint8((int(dst)*(255-alpha) + int(src)*alpha + 127) / 255)
}

func readImageURLValues(params map[string]interface{}) []string {
	if params == nil {
		return nil
	}
	raw, ok := params["image_urls"]
	if !ok {
		return nil
	}
	values := make([]string, 0, 4)
	switch typed := raw.(type) {
	case []string:
		for _, value := range typed {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				values = append(values, trimmed)
			}
		}
	case []interface{}:
		for _, value := range typed {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				values = append(values, trimmed)
			}
		}
	}
	return values
}

func toInterfaceSlice(values []string) []interface{} {
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func sanitizeFeatureSlug(featureSlug string) string {
	trimmed := strings.TrimSpace(featureSlug)
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(trimmed)
}

func isFullBounds(rect image.Rectangle, bounds image.Rectangle) bool {
	return rect.Min.X <= bounds.Min.X &&
		rect.Min.Y <= bounds.Min.Y &&
		rect.Max.X >= bounds.Max.X &&
		rect.Max.Y >= bounds.Max.Y
}
