package domain

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ideaboard/api/internal/workflow"
)

func TestExtractImageResponses(t *testing.T) {
	t.Run("OpenAI standard data url shape", func(t *testing.T) {
		jsonResp := []byte(`{"created":1234,"data":[{"url":"https://api.openai.com/v1/images/123.png"}]}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://api.openai.com/v1/images/123.png"}, urls)
	})

	t.Run("OpenAI standard data b64_json shape", func(t *testing.T) {
		b64Content := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
		jsonResp := []byte(`{"data":[{"b64_json":"` + b64Content + `"}]}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Len(t, urls, 1)
		assert.True(t, strings.HasPrefix(urls[0], "data:image/png;base64,"))
	})

	t.Run("Fal.ai images array shape", func(t *testing.T) {
		jsonResp := []byte(`{"images":[{"url":"https://fal.media/files/monkey/sample.png"}]}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://fal.media/files/monkey/sample.png"}, urls)
	})

	t.Run("Fal.ai single image object shape", func(t *testing.T) {
		jsonResp := []byte(`{"image":{"url":"https://fal.media/files/single.webp"}}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://fal.media/files/single.webp"}, urls)
	})

	t.Run("OpenRouter Chat message.images shape", func(t *testing.T) {
		jsonResp := []byte(`{
			"choices": [
				{
					"message": {
						"images": [
							{"url": "https://openrouter.ai/gen/img1.png"}
						]
					}
				}
			]
		}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://openrouter.ai/gen/img1.png"}, urls)
	})

	t.Run("OpenRouter Chat content array parts", func(t *testing.T) {
		jsonResp := []byte(`{
			"choices": [
				{
					"message": {
						"content": [
							{"type": "text", "text": "generated image:"},
							{"type": "image_url", "image_url": {"url": "https://cdn.ai/part.jpg"}}
						]
					}
				}
			]
		}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://cdn.ai/part.jpg"}, urls)
	})

	t.Run("Stability AI artifacts base64 shape", func(t *testing.T) {
		b64Content := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
		jsonResp := []byte(`{"artifacts":[{"base64":"` + b64Content + `"}]}`)
		urls, err := ExtractImageResponses(jsonResp)
		require.NoError(t, err)
		assert.Len(t, urls, 1)
		assert.True(t, strings.HasPrefix(urls[0], "data:image/png;base64,"))
	})

	t.Run("Direct Binary Stream Image Sniffing", func(t *testing.T) {
		rawPNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
		require.NoError(t, err)

		urls, err := ExtractImageResponses(rawPNG)
		require.NoError(t, err)
		assert.Len(t, urls, 1)
		assert.True(t, strings.HasPrefix(urls[0], "data:image/png;base64,"))
	})

	t.Run("Vendor explicit error extraction", func(t *testing.T) {
		jsonResp := []byte(`{"error":{"message":"Quota exceeded for model dall-e-3"}}`)
		_, err := ExtractImageResponses(jsonResp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Quota exceeded for model dall-e-3")
	})

	t.Run("Unknown structure shows top-level keys transparently", func(t *testing.T) {
		jsonResp := []byte(`{"task_id":"abc-123","status":"queued","progress":0.5}`)
		_, err := ExtractImageResponses(jsonResp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "返回字段")
		assert.Contains(t, err.Error(), "task_id")
		assert.Contains(t, err.Error(), "status")
	})
}

func TestBuildRequestBody(t *testing.T) {
	t.Run("Default image generation request", func(t *testing.T) {
		body, err := BuildRequestBody(RequestParams{
			Prompt:    "A sleek modern kitchen with natural light",
			Provider:  "openai",
			Model:     "gpt-image-1",
			Operation: workflow.RuntimeOperationGeneration,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"prompt":"A sleek modern kitchen with natural light"`)
		assert.Contains(t, string(body), `"model":"gpt-image-1"`)
		assert.Contains(t, string(body), `"n":1`)
	})

	t.Run("Chat Multimodal request for OpenRouter", func(t *testing.T) {
		body, err := BuildRequestBody(RequestParams{
			Prompt:    "A sleek modern kitchen with natural light",
			Provider:  "openrouter",
			Model:     "openai/chat-gpt-4o",
			Operation: workflow.RuntimeOperationRender,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"modalities":["text","image"]`)
		assert.Contains(t, string(body), `"messages":`)
	})

	t.Run("Fal request uses prompt", func(t *testing.T) {
		body, err := BuildRequestBody(RequestParams{
			Prompt:    "A sleek modern kitchen with natural light",
			Provider:  "fal",
			Model:     "flux-1-dev",
			Operation: workflow.RuntimeOperationRender,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"prompt":"A sleek modern kitchen with natural light"`)
	})

	t.Run("Custom parameters injection via sjson", func(t *testing.T) {
		customJSON := `{"guidance_scale": 7.5, "seed": 42, "style": "photorealistic"}`
		body, err := BuildRequestBody(RequestParams{
			Prompt:           "A sleek modern kitchen with natural light",
			Provider:         "openai",
			Model:            "custom-model",
			Operation:        workflow.RuntimeOperationGeneration,
			CustomParamsJSON: customJSON,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"guidance_scale":7.5`)
		assert.Contains(t, string(body), `"seed":42`)
		assert.Contains(t, string(body), `"style":"photorealistic"`)
	})

	t.Run("Inpaint request includes image and mask parameters", func(t *testing.T) {
		body, err := BuildRequestBody(RequestParams{
			Prompt:         "add a vintage coffee machine on the counter",
			ResolvedImages: []string{"https://example.com/base.png", "https://example.com/mask.png"},
			Provider:       "fal",
			Model:          "fal-ai/flux/dev/inpaint",
			Operation:      workflow.RuntimeOperationInpaint,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"image_url":"https://example.com/base.png"`)
		assert.Contains(t, string(body), `"mask_url":"https://example.com/mask.png"`)
	})

	t.Run("Non-object custom parameters are ignored, never an error", func(t *testing.T) {
		body, err := BuildRequestBody(RequestParams{
			Prompt:           "plain",
			Provider:         "openai",
			Model:            "gpt-image-1",
			Operation:        workflow.RuntimeOperationGeneration,
			CustomParamsJSON: `["not", "an", "object"]`,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"prompt":"plain"`)
		assert.NotContains(t, string(body), `"not"`)
	})

}
