package dialects

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ideaboard/api/internal/workflow"
)

func TestResolveDialectDispatch(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		custom   string
		want     Dialect
	}{
		{"openrouter always chat", "openrouter", "google/gemini-3.1-flash", "", &ChatDialect{}},
		{"anthropic always chat", "anthropic", "claude-sonnet-4", "", &ChatDialect{}},
		{"chat keyword on plain provider", "openai", "gpt-image-1-chat", "", &ChatDialect{}},
		{"custom type=chat wins", "fal", "flux-dev", `{"type":"chat"}`, &ChatDialect{}},
		{"fal media route", "fal", "flux-1-dev", "", &FalDialect{}},
		{"fal alias resolves", "fal-ai", "flux-1-dev", "", &FalDialect{}},
		{"openai image model generic", "openai", "dall-e-3", "", &GenericDialect{}},
		{"custom type=image beats chat keyword", "openai", "gpt-4o-image", `{"type":"image"}`, &GenericDialect{}},
		{"unknown provider generic", "no-such-vendor", "some-model", "", &GenericDialect{}},
		{"custom slug generic", "custom-acme", "m1", "", &GenericDialect{}},
		{"vertex generic until evidence", "google-vertex-ai", "imagen-3", "", &GenericDialect{}},
		{"workers-ai generic until evidence", "workers-ai", "flux-schnell", "", &GenericDialect{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Single routing seam: dialect arrives via resolve-then-shape.
			route, err := Resolve(workflow.RuntimeOperationGeneration, tc.provider, tc.model, "", tc.custom)
			require.NoError(t, err)
			assert.IsType(t, tc.want, route.Dialect)
		})
	}
}

func TestChatEncode(t *testing.T) {
	d := NewChatDialect()
	body, err := d.Encode(Request{
		Prompt: "A sleek modern kitchen",
		Model:  "google/gemini-3.1-flash-lite-image",
		Images: []string{"https://example.com/base.png"},
	})
	require.NoError(t, err)
	var decoded struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL *struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		} `json:"messages"`
		Modalities []string `json:"modalities"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "google/gemini-3.1-flash-lite-image", decoded.Model)
	assert.Equal(t, []string{"text", "image"}, decoded.Modalities)
	require.Len(t, decoded.Messages[0].Content, 2)
	assert.Equal(t, "https://example.com/base.png", decoded.Messages[0].Content[1].ImageURL.URL)
	// Encode never merges custom params: the worker applies them after.
	body, err = d.Encode(Request{Prompt: "p", Model: "m"})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "guidance_scale")
}

func TestFalEncode(t *testing.T) {
	d := NewFalDialect()
	body, err := d.Encode(Request{
		Prompt: "add a vintage coffee machine",
		Model:  "fal-ai/flux/dev/inpaint",
		Images: []string{"https://example.com/base.png", "https://example.com/mask.png"},
	})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"prompt":"add a vintage coffee machine"`)
	assert.Contains(t, string(body), `"image_url":"https://example.com/base.png"`)
	assert.Contains(t, string(body), `"mask_url":"https://example.com/mask.png"`)
}

func TestGenericEncode(t *testing.T) {
	d := NewGenericDialect()
	t.Run("generation", func(t *testing.T) {
		body, err := d.Encode(Request{
			Prompt:    "A sleek modern kitchen",
			Model:     "gpt-image-1",
			Operation: workflow.RuntimeOperationGeneration,
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"prompt":"A sleek modern kitchen"`)
		assert.Contains(t, string(body), `"model":"gpt-image-1"`)
		assert.Contains(t, string(body), `"n":1`)
	})
	t.Run("inpaint edit shape", func(t *testing.T) {
		body, err := d.Encode(Request{
			Prompt:    "fill the hole",
			Model:     "gpt-image-1",
			Operation: workflow.RuntimeOperationInpaint,
			Images:    []string{"https://example.com/base.png", "https://example.com/mask.png"},
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"image":"https://example.com/base.png"`)
		assert.Contains(t, string(body), `"mask":"https://example.com/mask.png"`)
	})
}

func TestDecodeShapes(t *testing.T) {
	g := NewGenericDialect()
	t.Run("OpenAI data url", func(t *testing.T) {
		urls, err := g.Decode([]byte(`{"created":1234,"data":[{"url":"https://api.openai.com/v1/images/123.png"}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"https://api.openai.com/v1/images/123.png"}, urls)
	})
	t.Run("OpenAI b64_json converts with image prefix", func(t *testing.T) {
		b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
		urls, err := g.Decode([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
		require.NoError(t, err)
		require.Len(t, urls, 1)
		assert.True(t, strings.HasPrefix(urls[0], "data:image/png;base64,"))
	})
	t.Run("plain text is not an image", func(t *testing.T) {
		_, err := g.Decode([]byte(`{"output":["this is just a status message for humans"]}`))
		require.Error(t, err)
	})
	t.Run("fal images array", func(t *testing.T) {
		f := NewFalDialect()
		urls, err := f.Decode([]byte(`{"images":[{"url":"https://fal.media/files/monkey/sample.png"}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"https://fal.media/files/monkey/sample.png"}, urls)
	})
	t.Run("fal single image object", func(t *testing.T) {
		f := NewFalDialect()
		urls, err := f.Decode([]byte(`{"image":{"url":"https://fal.media/files/single.webp"}}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"https://fal.media/files/single.webp"}, urls)
	})
	t.Run("chat message images", func(t *testing.T) {
		c := NewChatDialect()
		urls, err := c.Decode([]byte(`{"choices":[{"message":{"images":[{"url":"https://openrouter.ai/gen/img1.png"}]}}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"https://openrouter.ai/gen/img1.png"}, urls)
	})
	t.Run("chat content parts", func(t *testing.T) {
		c := NewChatDialect()
		urls, err := c.Decode([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"generated image:"},{"type":"image_url","image_url":{"url":"https://cdn.ai/part.jpg"}}]}}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"https://cdn.ai/part.jpg"}, urls)
	})
	t.Run("stability artifacts base64", func(t *testing.T) {
		b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
		urls, err := g.Decode([]byte(`{"artifacts":[{"base64":"` + b64 + `"}]}`))
		require.NoError(t, err)
		require.Len(t, urls, 1)
		assert.True(t, strings.HasPrefix(urls[0], "data:image/png;base64,"))
	})
	t.Run("vendor error surfaces", func(t *testing.T) {
		_, err := g.Decode([]byte(`{"error":{"message":"Quota exceeded for model dall-e-3"}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Quota exceeded for model dall-e-3")
	})
	t.Run("unknown structure lists keys", func(t *testing.T) {
		_, err := g.Decode([]byte(`{"task_id":"abc-123","status":"queued","progress":0.5}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "返回字段")
		assert.Contains(t, err.Error(), "task_id")
	})
}

func TestSniffImageStream(t *testing.T) {
	rawPNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	dataURL, ok := SniffImageStream(rawPNG)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(dataURL, "data:image/png;base64,"))
	_, ok = SniffImageStream([]byte(`{"data":[]}`))
	assert.False(t, ok)
}

func TestBase64ImageDataURLRejectsText(t *testing.T) {
	_, ok := Base64ImageDataURL("this is just a status message for humans, not base64!")
	assert.False(t, ok)
	_, ok = Base64ImageDataURL("aGk=")
	assert.False(t, ok)
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	converted, ok := Base64ImageDataURL(png)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(converted, "data:image/png;base64,"))
}

func TestInjectCustomParams(t *testing.T) {
	base := []byte(`{"prompt":"plain","model":"m"}`)
	merged, err := InjectCustomParams(base, `{"guidance_scale":7.5,"seed":42}`, "openai", "m")
	require.NoError(t, err)
	assert.Contains(t, string(merged), `"guidance_scale":7.5`)
	assert.Contains(t, string(merged), `"seed":42`)
	// Non-object custom params never error, never merge.
	merged, err = InjectCustomParams(base, `["not","an","object"]`, "openai", "m")
	require.NoError(t, err)
	assert.NotContains(t, string(merged), `"not"`)
	// Empty passthrough.
	merged, err = InjectCustomParams(base, "", "openai", "m")
	require.NoError(t, err)
	assert.Equal(t, string(base), string(merged))
}
