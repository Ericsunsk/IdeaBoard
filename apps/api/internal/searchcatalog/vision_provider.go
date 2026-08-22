package searchcatalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string              `json:"role"`
	Content []openAIContentPart `json:"content"`
}

type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func extractOpenAICompatibleVision(ctx context.Context, config VisionProviderConfig, input VisionInput) (VisionExtraction, error) {
	content := make([]openAIContentPart, 0, len(input.Images)+1)
	if userText := strings.TrimSpace(input.UserText); userText != "" {
		content = append(content, openAIContentPart{Type: "text", Text: "User text: " + userText})
	}
	for _, image := range input.Images {
		content = append(content, openAIContentPart{Type: "image_url", ImageURL: &openAIImageURL{
			URL: "data:" + imageContentType(image) + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
		}})
	}
	body, err := json.Marshal(openAIChatRequest{Model: config.Model, Messages: []openAIChatMessage{
		{Role: "system", Content: []openAIContentPart{{Type: "text", Text: visionSystemPrompt()}}},
		{Role: "user", Content: content},
	}})
	if err != nil {
		return VisionExtraction{}, err
	}
	endpoint, err := openAIChatCompletionsURL(config.EndpointURL)
	if err != nil {
		return VisionExtraction{}, &visionProviderError{kind: visionProviderFailureResponse, cause: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return VisionExtraction{}, &visionProviderError{kind: visionProviderFailureResponse, cause: err}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
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
		return VisionExtraction{}, &visionProviderError{kind: kind, cause: fmt.Errorf("status %d: %s", response.StatusCode, safeProviderErrorMessage(data))}
	}
	var envelope openAIChatResponse
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) == 0 {
		return VisionExtraction{}, fmt.Errorf("%w: invalid OpenAI-compatible response", ErrVisionSchema)
	}
	extraction, err := decodeVisionExtraction(envelope.Choices[0].Message.Content)
	if err != nil {
		return VisionExtraction{}, fmt.Errorf("%w: invalid model JSON: %v", ErrVisionSchema, err)
	}
	return extraction, nil
}

func decodeVisionExtraction(content string) (VisionExtraction, error) {
	decoder := json.NewDecoder(strings.NewReader(stripJSONFence(content)))
	decoder.DisallowUnknownFields()
	var extraction VisionExtraction
	if err := decoder.Decode(&extraction); err != nil {
		return VisionExtraction{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return VisionExtraction{}, errors.New("multiple JSON values")
		}
		return VisionExtraction{}, err
	}
	return extraction, nil
}

func imageContentType(image VisionImage) string {
	if contentType := strings.TrimSpace(image.ContentType); strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	return "image/png"
}

func openAIChatCompletionsURL(endpoint string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		return "", errors.New("vision endpoint URL is required")
	}
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed, nil
	}
	return trimmed + "/chat/completions", nil
}

func visionSystemPrompt() string {
	return "Analyze the reference image(s) and produce one concise visual search query. Return only valid JSON with exactly one key: query (string). Do not use Markdown or any extra text. The user's text is supplementary intent; preserve the visual evidence and do not replace visual analysis with the text."
}

func stripJSONFence(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(strings.TrimSpace(trimmed), "```")
	return strings.TrimSpace(trimmed)
}

func safeProviderErrorMessage(data []byte) string {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && len(envelope.Error) > 0 {
		var message struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &message) == nil && strings.TrimSpace(message.Message) != "" {
			return strings.TrimSpace(message.Message)
		}
	}
	return strings.TrimSpace(string(data))
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
