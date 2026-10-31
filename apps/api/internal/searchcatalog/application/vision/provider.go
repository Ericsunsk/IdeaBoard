package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
)

// visionSearchTimeout bounds one vision extraction call through the gateway.
const visionSearchTimeout = 30 * time.Second

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

func extractOpenAICompatibleVision(ctx context.Context, gateway ports.VisionGatewayClient, model string, input domain.VisionInput) (domain.VisionExtraction, error) {
	if gateway == nil {
		return domain.VisionExtraction{}, &domain.VisionProviderError{Kind: domain.VisionProviderFailureNetwork, Err: errors.New("vision gateway client is not initialized")}
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return domain.VisionExtraction{}, &domain.VisionProviderError{Kind: domain.VisionProviderFailureResponse, Err: errors.New("vision model is required")}
	}
	body, err := marshalOpenAIChatRequest(model, input)
	if err != nil {
		return domain.VisionExtraction{}, err
	}
	resp, err := gateway.Do(ctx, ports.VisionGatewayRequest{Model: model, Body: body, Timeout: visionSearchTimeout})
	if err != nil {
		return domain.VisionExtraction{}, classifyVisionNetworkError(err)
	}
	if err := validateOpenAIResponseStatus(resp.StatusCode, resp.Body); err != nil {
		return domain.VisionExtraction{}, err
	}
	return decodeOpenAIChatExtraction(resp.Body)
}

func marshalOpenAIChatRequest(model string, input domain.VisionInput) ([]byte, error) {
	content := openAIUserContent(input)
	return json.Marshal(openAIChatRequest{Model: model, Messages: []openAIChatMessage{
		{Role: "system", Content: []openAIContentPart{{Type: "text", Text: visionSystemPrompt()}}},
		{Role: "user", Content: content},
	}})
}

func openAIUserContent(input domain.VisionInput) []openAIContentPart {
	content := make([]openAIContentPart, 0, len(input.Images)+1)
	if userText := strings.TrimSpace(input.UserText); userText != "" {
		content = append(content, openAIContentPart{Type: "text", Text: "<user_intent>\n" + userText + "\n</user_intent>"})
	}
	for _, image := range input.Images {
		content = append(content, openAIContentPart{Type: "image_url", ImageURL: &openAIImageURL{
			URL: "data:" + imageContentType(image) + ";base64," + base64.StdEncoding.EncodeToString(image.Data),
		}})
	}
	return content
}

func classifyVisionNetworkError(err error) error {
	kind := domain.VisionProviderFailureNetwork
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutError(err) {
		kind = domain.VisionProviderFailureTimeout
	}
	return &domain.VisionProviderError{Kind: kind, Err: err}
}

func validateOpenAIResponseStatus(statusCode int, data []byte) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	kind := domain.VisionProviderFailureResponse
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = domain.VisionProviderFailureCredentials
	case http.StatusTooManyRequests:
		kind = domain.VisionProviderFailureQuota
	}
	return &domain.VisionProviderError{Kind: kind, Err: fmt.Errorf("status %d: %s", statusCode, safeProviderErrorMessage(data))}
}

func decodeOpenAIChatExtraction(data []byte) (domain.VisionExtraction, error) {
	var envelope openAIChatResponse
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Choices) == 0 {
		return domain.VisionExtraction{}, fmt.Errorf("%w: invalid OpenAI-compatible response", domain.ErrVisionSchema)
	}
	extraction, err := decodeVisionExtraction(envelope.Choices[0].Message.Content)
	if err != nil {
		return domain.VisionExtraction{}, fmt.Errorf("%w: invalid model JSON: %w", domain.ErrVisionSchema, err)
	}
	return extraction, nil
}

func decodeVisionExtraction(content string) (domain.VisionExtraction, error) {
	decoder := json.NewDecoder(strings.NewReader(stripJSONFence(content)))
	decoder.DisallowUnknownFields()
	var extraction domain.VisionExtraction
	if err := decoder.Decode(&extraction); err != nil {
		return domain.VisionExtraction{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.VisionExtraction{}, errors.New("multiple JSON values")
		}
		return domain.VisionExtraction{}, err
	}
	return extraction, nil
}

func imageContentType(image domain.VisionImage) string {
	if contentType := strings.TrimSpace(image.ContentType); strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	return "image/png"
}

func visionSystemPrompt() string {
	return buildVisionSearchPrompt("Reference images are provided in selection order.")
}

func buildVisionSearchPrompt(referenceNote string) string {
	prompt := `You write visual search queries for Unsplash and Pexels.

<task>
Turn the reference image(s) into one searchable query.
</task>

<reference_input>
%s
</reference_input>

<evidence>
Treat all images as one visual direction. Identify the 2-4 most distinctive, visible, and searchable traits across the set: subject or setting, composition, color or light, material or texture, and style. Keep specific traits that define the references; do not make the query generic. If images conflict, choose the clearest coherent direction instead of listing or merging unrelated descriptions.
</evidence>

<intent_policy>
User text is supplementary. Use it only when compatible with visible evidence; never let it replace image analysis.
</intent_policy>

<query_rules>
Use concrete nouns and common visual modifiers that stock-photo search can match. Omit names, brands, readable text, speculation, and incidental details. Write one natural-language phrase, usually 4-12 searchable words.
</query_rules>

<output>
Return exactly one JSON object: {"query":"..."}. JSON only, with no Markdown or extra text.
</output>`
	return strings.TrimSpace(fmt.Sprintf(prompt, strings.TrimSpace(referenceNote)))
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
