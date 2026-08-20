package searchcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/safehttp"
)

type visionProviderFailure string

const (
	visionProviderFailureNetwork     visionProviderFailure = "network"
	visionProviderFailureTimeout     visionProviderFailure = "timeout"
	visionProviderFailureCredentials visionProviderFailure = "credentials"
	visionProviderFailureQuota       visionProviderFailure = "quota"
	visionProviderFailureResponse    visionProviderFailure = "response"
)

type visionProviderError struct {
	kind  visionProviderFailure
	cause error
}

func (e *visionProviderError) Error() string {
	if e == nil || e.cause == nil {
		return "vision provider request failed"
	}
	return fmt.Sprintf("vision provider request failed: %v", e.cause)
}

func (e *visionProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type multipartVisionProviderClient struct{}

func (multipartVisionProviderClient) Extract(ctx context.Context, config VisionProviderConfig, input VisionInput) (VisionExtraction, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", config.Model); err != nil {
		return VisionExtraction{}, err
	}
	if err := writer.WriteField("user_text", input.UserText); err != nil {
		return VisionExtraction{}, err
	}
	for _, image := range input.Images {
		part, err := writer.CreateFormFile("images[]", image.Filename)
		if err != nil {
			return VisionExtraction{}, err
		}
		if _, err := part.Write(image.Data); err != nil {
			return VisionExtraction{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return VisionExtraction{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.EndpointURL, &body)
	if err != nil {
		return VisionExtraction{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(config.APIKey) != "" {
		request.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	response, err := safehttp.DoRequestWithAllowHosts(ctx, request, visionSearchTimeout, outbound.AllowHosts(ctx)...)
	if err != nil {
		kind := visionProviderFailureNetwork
		if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
			kind = visionProviderFailureTimeout
		}
		return VisionExtraction{}, &visionProviderError{kind: kind, cause: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return VisionExtraction{}, &visionProviderError{kind: visionProviderFailureResponse, cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		kind := visionProviderFailureResponse
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			kind = visionProviderFailureCredentials
		case http.StatusTooManyRequests:
			kind = visionProviderFailureQuota
		}
		return VisionExtraction{}, &visionProviderError{kind: kind, cause: fmt.Errorf("status %d", response.StatusCode)}
	}
	var extraction VisionExtraction
	if err := json.Unmarshal(data, &extraction); err != nil {
		return VisionExtraction{}, fmt.Errorf("%w: %v", ErrVisionSchema, err)
	}
	return extraction, nil
}

func isTimeoutError(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

func classifyVisionProviderFailure(err error) FailureCategory {
	if errors.Is(err, ErrVisionSchema) {
		return FailureResponse
	}
	var providerErr *visionProviderError
	if !errors.As(err, &providerErr) {
		return FailureUnknown
	}
	switch providerErr.kind {
	case visionProviderFailureCredentials:
		return FailureCredentials
	case visionProviderFailureQuota:
		return FailureQuota
	case visionProviderFailureResponse:
		return FailureResponse
	case visionProviderFailureNetwork, visionProviderFailureTimeout:
		return FailureNetwork
	default:
		return FailureUnknown
	}
}

func getVisionImage(ctx context.Context, parsed *url.URL) (*http.Response, error) {
	return safehttp.GetWithAllowHosts(ctx, parsed.String(), visionSearchTimeout, parsed.Hostname())
}
