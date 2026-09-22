package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	"ideaboard/api/internal/workflow"
)

func slogInfoGatewayCall(taskID, correlationID, provider, model string, operation workflow.RuntimeOperation) {
	slog.Info("calling gateway API",
		"task_id", taskID, "correlation_id", correlationID,
		"provider", provider, "model", model, "operation", string(operation))
}

// gatewayRequestPath resolves pipeline-only routes (cutout/upscale), where
// the mapping @path is required configuration. Model-image routing moved
// to workflow.ResolveRoute (shared with preview).
func gatewayRequestPath(adapter domain.ProviderRuntimeAdapter, pathOverride string) (string, error) {
	switch adapter {
	case domain.ProviderRuntimeAdapterCutout:
		return cutoutGatewayPath(pathOverride)
	case domain.ProviderRuntimeAdapterUpscale:
		return upscaleGatewayPath(pathOverride)
	default:
		return "", fmt.Errorf("unsupported provider runtime adapter %d", adapter)
	}
}

func (c *Executor) callGatewayModelImage(
	ctx context.Context,
	provider, model, pathOverride, customParams string,
	providerReq providerRequest,
	timeout time.Duration,
) (providerCallResult, error) {
	client, err := c.gatewayClientForCall(ctx)
	if err != nil {
		return providerCallResult{}, err
	}
	contentType := "application/json"
	// Single routing seam with preview: mapping @path and catalog custom
	// fields arrive here already merged; customParams joins the chat judgment.
	resolved := workflow.ResolveRoute(provider, model, pathOverride, customParams)
	resolvedPath := resolved.Path

	body, err := BuildGatewayRequestBody(c, providerReq, provider, model, workflow.RuntimeOperationRender, customParams)
	if err != nil {
		return providerCallResult{}, err
	}
	resp, err := client.Do(ctx, runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Kind:     runtimeports.GatewayEndpointProviderNative,
			Provider: resolved.Namespace,
			Path:     resolvedPath,
		},
		Model:       provider + "/" + model,
		TaskID:      "",
		Body:        body,
		ContentType: contentType,
		Cache:       runtimeports.GatewayCachePolicy{Mode: runtimeports.GatewayCacheSkip},
		Timeout:     timeout,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("网关请求失败: %w", err)
	}
	imageURLs, err := ParseImageResponses(provider, resp.Body)
	if err != nil {
		return providerCallResult{
			Model:            model,
			RawOutputPreview: providerRawOutputPreview(resp.Body),
			ParsedOutputKind: "image-results",
		}, newProviderOutputParseError(err)
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		RawOutputPreview:  providerRawOutputPreview(resp.Body),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

func buildUpscaleGatewayPayload(c *Executor, providerReq providerRequest) (map[string]interface{}, error) {
	if len(providerReq.ImageRefs) == 0 || strings.TrimSpace(providerReq.ImageRefs[0]) == "" {
		return nil, fmt.Errorf("高清放大请求缺少原图")
	}
	scaleRequest := providerReq
	scaleRequest.CanvasRef = ""
	inputURL, err := c.imageTransformInputURL(scaleRequest, "高清放大")
	if err != nil {
		return nil, err
	}
	if providerReq.Scale < 1 || providerReq.Scale > 4 || math.Abs(providerReq.Scale*10-math.Round(providerReq.Scale*10)) > 1e-9 {
		return nil, fmt.Errorf("高清放大 scale 必须是 1.0-4.0 且步长为 0.1")
	}
	return map[string]interface{}{
		"image_url":     inputURL,
		"scale":         providerReq.Scale,
		"output_format": "png",
	}, nil
}

// cutoutGatewayPath and upscaleGatewayPath require explicit operator
// configuration: pipeline endpoint paths were per-account data with no
// static form. The mapping value carries them (provider/model@gateway-path).
func cutoutGatewayPath(pathOverride string) (string, error) {
	if path := strings.Trim(strings.TrimSpace(pathOverride), "/"); path != "" {
		return path, nil
	}
	return "v1/images/cutout", nil
}

func upscaleGatewayPath(pathOverride string) (string, error) {
	if path := strings.Trim(strings.TrimSpace(pathOverride), "/"); path != "" {
		return path, nil
	}
	return "v1/images/upscale", nil
}

func buildCutoutGatewayPayload(c *Executor, providerReq providerRequest) (map[string]interface{}, error) {
	inputURL, err := c.imageTransformInputURL(providerReq, "抠图")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"image_url": inputURL}, nil
}

// callGatewayImageBytes posts one JSON payload to an image pipeline
// route (cutout/upscale) and extracts the image response using the universal extractor.
// It seamlessly supports both binary image streams and JSON responses (URL/Base64).
func (c *Executor) callGatewayImageBytes(
	ctx context.Context,
	provider, model, path string,
	payload map[string]interface{},
	label string,
	timeout time.Duration,
) (providerCallResult, error) {
	client, err := c.gatewayClientForCall(ctx)
	if err != nil {
		return providerCallResult{}, err
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return providerCallResult{}, fmt.Errorf("序列化%s请求失败: %w", label, err)
	}
	resp, err := client.Do(ctx, runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Kind:     runtimeports.GatewayEndpointProviderNative,
			Provider: gatewayProviderPath(provider),
			Path:     path,
		},
		Model:   provider + "/" + model,
		Body:    bodyJSON,
		Cache:   runtimeports.GatewayCachePolicy{Mode: runtimeports.GatewayCacheSkip},
		Timeout: timeout,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("%s网关请求失败: %w", label, err)
	}
	if len(resp.Body) == 0 {
		return providerCallResult{}, fmt.Errorf("%s响应为空", label)
	}
	imageURLs, err := ParseImageResponses(provider, resp.Body)
	if err != nil {
		return providerCallResult{
			Model:            model,
			RawOutputPreview: providerRawOutputPreview(resp.Body),
			ParsedOutputKind: "image-results",
		}, newProviderOutputParseError(fmt.Errorf("%s响应解析失败: %w", label, err))
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		RawOutputPreview:  providerRawOutputPreview(resp.Body),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

func (c *ProviderExecutor) imageTransformInputURL(request providerRequest, label string) (string, error) {
	ref := ""
	if len(request.ImageRefs) > 0 {
		ref = request.ImageRefs[0]
	} else {
		ref = request.CanvasRef
	}
	return resolveProviderPublicImageURL(c, ref, label)
}

