package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"log/slog"
	"math"
	"strings"
	"time"

	"ideaboard/api/internal/runtime/interfaces/worker/providers/dialects"
	runtimeports "ideaboard/api/internal/runtime/ports"
	"ideaboard/api/internal/workflow"
)

func slogInfoGatewayCall(taskID, correlationID, provider, model string, operation workflow.RuntimeOperation) {
	slog.Info("calling gateway API",
		"task_id", taskID, "correlation_id", correlationID,
		"provider", provider, "model", model, "operation", string(operation))
}

func (c *Executor) callGatewayModelImage(
	ctx context.Context,
	taskID, feature string,
	provider, model, customParams string,
	route dialects.Route,
	providerReq providerRequest,
	operation workflow.RuntimeOperation,
	timeout time.Duration,
) (providerCallResult, error) {
	// Routing arrives pre-resolved from dialects.Resolve (shared with
	// preview): mapping @path and catalog custom fields already merged;
	// customParams already joined the chat judgment. No re-derivation here.
	dialect := route.Dialect
	imageURLs, err := resolveDialectImageURLs(c, providerReq)
	if err != nil {
		return providerCallResult{}, err
	}
	body, err := dialect.Encode(dialects.Request{
		Prompt:    providerReq.Prompt,
		Model:     model,
		Operation: operation,
		Images:    imageURLs,
	})
	if err != nil {
		return providerCallResult{}, err
	}
	// Operator custom params win over shaped defaults; applied once here,
	// never inside dialects, so base shapes stay pure and testable.
	if body, err = dialects.InjectCustomParams(body, customParams, provider, model); err != nil {
		return providerCallResult{}, err
	}
	resp, err := doGatewayImageCall(ctx, c, taskID, feature, provider, model, route.Namespace, route.Path, body, "application/json", timeout, errorrender.MsgCallGatewayModelImage)
	if err != nil {
		return providerCallResult{Receipt: resp.Receipt}, err
	}
	respBody := resp.Body
	imageURLs, err = decodeGatewayImageResponse(dialect, respBody)
	if err != nil {
		return providerCallResult{
			Model:            model,
			Receipt:          resp.Receipt,
			RawOutputPreview: providerRawOutputPreview(respBody),
			ParsedOutputKind: "image-results",
		}, runtimeports.NewProviderOutputParseError(err)
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		Receipt:           resp.Receipt,
		RawOutputPreview:  providerRawOutputPreview(respBody),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

// resolveDialectImageURLs resolves input and mask media refs to public URLs.
// Dialects index this order exactly as documented per shape.
func resolveDialectImageURLs(c *Executor, providerReq providerRequest) ([]string, error) {
	refs := append(append([]string{}, providerReq.ImageRefs...), providerReq.MaskRefs...)
	var resolvedPublicURLs []string
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		pubURL, err := resolveProviderPublicImageURL(c, ref, errorrender.MsgResolveDialectImageURLs)
		if err != nil {
			return nil, err
		}
		resolvedPublicURLs = append(resolvedPublicURLs, pubURL)
	}
	return resolvedPublicURLs, nil
}

// decodeGatewayImageResponse extracts image URLs from one gateway response:
// binary streams sniff first (vendor-agnostic), otherwise the dialect's
// JSON decode (shared universal recall across dialects).
func decodeGatewayImageResponse(dialect dialects.Dialect, respBody []byte) ([]string, error) {
	if dataURL, ok := dialects.SniffImageStream(respBody); ok {
		return []string{dataURL}, nil
	}
	return dialect.Decode(respBody)
}

func buildUpscaleGatewayPayload(c *Executor, providerReq providerRequest) (map[string]interface{}, error) {
	if len(providerReq.ImageRefs) == 0 || strings.TrimSpace(providerReq.ImageRefs[0]) == "" {
		return nil, errors.New(errorrender.MsgBuildUpscaleGatewayPayload)
	}
	scaleRequest := providerReq
	inputURL, err := c.imageTransformInputURL(scaleRequest, errorrender.MsgBuildUpscaleGatewayPayload2)
	if err != nil {
		return nil, err
	}
	if providerReq.Scale < 1 || providerReq.Scale > 4 || math.Abs(providerReq.Scale*10-math.Round(providerReq.Scale*10)) > 1e-9 {
		return nil, errors.New(errorrender.MsgBuildUpscaleGatewayPayload3)
	}
	return map[string]interface{}{
		"image_url":     inputURL,
		"scale":         providerReq.Scale,
		"output_format": "png",
	}, nil
}

func buildCutoutGatewayPayload(c *Executor, providerReq providerRequest) (map[string]interface{}, error) {
	inputURL, err := c.imageTransformInputURL(providerReq, errorrender.MsgBuildCutoutGatewayPayload)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"image_url": inputURL}, nil
}

// callGatewayImageBytes posts one JSON payload to an image pipeline
// route (cutout/upscale). Pipeline payloads are provider-agnostic by design
// and stay outside dialect shaping; responses decode through the resolved
// dialect (universal recall) with the shared binary sniff first. The route
// arrives pre-resolved from dialects.Resolve: no path or dialect
// re-derivation here.
func (c *Executor) callGatewayImageBytes(
	ctx context.Context,
	taskID, feature string,
	provider, model string,
	route dialects.Route,
	payload map[string]interface{},
	label string,
	timeout time.Duration,
) (providerCallResult, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return providerCallResult{}, fmt.Errorf(errorrender.MsgCallGatewayImageBytes, label, err)
	}
	resp, err := doGatewayImageCall(ctx, c, taskID, feature, provider, model, route.Namespace, route.Path, bodyJSON, "application/json", timeout, label)
	if err != nil {
		return providerCallResult{Receipt: resp.Receipt}, err
	}
	respBody := resp.Body
	imageURLs, err := decodeGatewayImageResponse(route.Dialect, respBody)
	if err != nil {
		return providerCallResult{
			Model:            model,
			Receipt:          resp.Receipt,
			RawOutputPreview: providerRawOutputPreview(respBody),
			ParsedOutputKind: "image-results",
		}, runtimeports.NewProviderOutputParseError(fmt.Errorf(errorrender.MsgCallGatewayImageBytes2, label, err))
	}
	return providerCallResult{
		ImageURL:          imageURLs[0],
		ImageURLs:         imageURLs,
		Model:             model,
		Receipt:           resp.Receipt,
		RawOutputPreview:  providerRawOutputPreview(respBody),
		ParsedOutputKind:  "image-results",
		ParsedOutputCount: len(imageURLs),
	}, nil
}

// doGatewayImageCall is the single gateway send + empty-body guard for image
// routes. Transport errors carry the human label prefix; context.Canceled
// passes through untouched so interruption converges as requeue, not failure.
func doGatewayImageCall(
	ctx context.Context,
	c *Executor,
	taskID, feature, provider, model, namespace, path string,
	body []byte,
	contentType string,
	timeout time.Duration,
	label string,
) (runtimeports.GatewayResponse, error) {
	client, err := c.gatewayClientForCall(ctx)
	if err != nil {
		return runtimeports.GatewayResponse{}, err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	resp, err := client.Do(ctx, runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Kind:     runtimeports.GatewayEndpointProviderNative,
			Provider: namespace,
			Path:     path,
		},
		Model:       provider + "/" + model,
		TaskID:      taskID,
		Feature:     feature,
		Body:        body,
		ContentType: contentType,
		Cache:       runtimeports.GatewayCachePolicy{Mode: runtimeports.GatewayCacheSkip},
		Timeout:     timeout,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return resp, err
		}
		return resp, fmt.Errorf(errorrender.MsgDoGatewayImageCall, label, err)
	}
	if len(resp.Body) == 0 {
		return resp, fmt.Errorf(errorrender.MsgDoGatewayImageCall2, label)
	}
	return resp, nil
}

func (c *ProviderExecutor) imageTransformInputURL(request providerRequest, label string) (string, error) {
	if len(request.ImageRefs) == 0 {
		return "", fmt.Errorf("%s: image input is required", label)
	}
	return resolveProviderPublicImageURL(c, request.ImageRefs[0], label)
}
