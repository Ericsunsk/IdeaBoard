package dialects

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	providerdomain "ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	"ideaboard/api/internal/workflow"
)

func TestResolveAdapterDispatch(t *testing.T) {
	route, err := Resolve(workflow.RuntimeOperationInpaint, "openai", "gpt-image-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, providerdomain.ProviderRuntimeAdapterModelImage, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationCutout, "fal", "birefnet", "", "")
	require.NoError(t, err)
	assert.Equal(t, providerdomain.ProviderRuntimeAdapterCutout, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "", "")
	require.NoError(t, err)
	assert.Equal(t, providerdomain.ProviderRuntimeAdapterUpscale, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationRender, "openai", "gpt-image-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, providerdomain.ProviderRuntimeAdapterModelImage, route.Adapter)

	_, err = Resolve("invalid_op", "openai", "m", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "网关不支持操作")
}

func TestServiceImageSharesResolveRoute(t *testing.T) {
	route, err := Resolve(workflow.RuntimeOperationGeneration, "openai", "dall-e-3", "", "")
	require.NoError(t, err)
	want := workflow.ResolveRoute("openai", "dall-e-3", "", "")
	assert.Equal(t, want.Namespace, route.Namespace)
	assert.Equal(t, want.Path, route.Path)
	assert.Equal(t, want.Source, route.Source)
	assert.Equal(t, want.IsChat, route.IsChat)
	assert.IsType(t, &GenericDialect{}, route.Dialect)

	// Catalog override wins, customParams joins the chat judgment.
	route, err = Resolve(workflow.RuntimeOperationGeneration, "openai", "dall-e-3", "custom/endpoint/v2", `{"type":"chat"}`)
	require.NoError(t, err)
	assert.Equal(t, "custom/endpoint/v2", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceOverride, route.Source)
	assert.True(t, route.IsChat)
	assert.IsType(t, &ChatDialect{}, route.Dialect)
}

func TestResolveNestedNativeChatRoute(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		params   string
		override string
		wantPath string
	}{
		{"explicit protocol", "custom-provider", "owner/native-model", `{"protocol":"chat"}`, "", "v1/chat/completions"},
		{"explicit type", "custom-provider", "owner/native-model", `{"type":"chat"}`, "", "v1/chat/completions"},
		{"model judgment", "custom-provider", "owner/gpt-5-image", "", "", "v1/chat/completions"},
		{"Fal chat judgment", "fal", "owner/native-model", `{"protocol":"chat"}`, "", "v1/chat/completions"},
		{"explicit path", "custom-provider", "owner/native-model", `{"protocol":"chat"}`, "custom/chat", "custom/chat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, err := Resolve(workflow.RuntimeOperationGeneration, tc.provider, tc.model, tc.override, tc.params)
			require.NoError(t, err)
			require.True(t, route.IsChat)
			payload, err := route.Dialect.Encode(Request{Model: tc.model, Prompt: "Draw a landscape"})
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(payload, &body))
			assert.Equal(t, tc.model, body["model"])
			assert.NotEmpty(t, body["messages"])
			assert.Equal(t, tc.wantPath, route.Path, "Chat payload must use its protocol endpoint unless a path override is supplied")
		})
	}
}

func TestResolvePipelineDefaultsAndOverrides(t *testing.T) {
	// Fal models default to their native app slug with auto-prefixing.
	route, err := Resolve(workflow.RuntimeOperationCutout, "fal", "birefnet", "", "")
	require.NoError(t, err)
	assert.Equal(t, "fal-ai/birefnet", route.Path)
	assert.Equal(t, workflow.GatewayProviderPath("fal"), route.Namespace)
	assert.Equal(t, workflow.GatewayPathSourceFal, route.Source)
	assert.IsType(t, &FalDialect{}, route.Dialect)

	route, err = Resolve(workflow.RuntimeOperationCutout, "fal", "fal-ai/birefnet", "", "")
	require.NoError(t, err)
	assert.Equal(t, "fal-ai/birefnet", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceFal, route.Source)

	route, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "", "")
	require.NoError(t, err)
	assert.Equal(t, "fal-ai/aura-sr", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceFal, route.Source)

	// Non-Fal providers without override resolve through unified RouteResolution.
	route, err = Resolve(workflow.RuntimeOperationCutout, "openai", "custom-cutout", "", "")
	require.NoError(t, err)
	assert.Equal(t, "v1/images/generations", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceGenerations, route.Source)

	route, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "custom/upscale/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "custom/upscale/v2", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceOverride, route.Source)

	// Pipeline rejects customParams fail-closed: no silent drop.
	_, err = Resolve(workflow.RuntimeOperationCutout, "openai", "dall-e-3", "", `{"type":"chat"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not accept custom params")
	_, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "", `{"type":"image"}`)
	require.Error(t, err)
}
