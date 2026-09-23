package dialects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	"ideaboard/api/internal/workflow"
)

func TestResolveAdapterDispatch(t *testing.T) {
	route, err := Resolve(workflow.RuntimeOperationInpaint, "openai", "gpt-image-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderRuntimeAdapterModelImage, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationCutout, "fal", "birefnet", "", "")
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderRuntimeAdapterCutout, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "", "")
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderRuntimeAdapterUpscale, route.Adapter)

	route, err = Resolve(workflow.RuntimeOperationRender, "openai", "gpt-image-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderRuntimeAdapterModelImage, route.Adapter)

	_, err = Resolve("invalid_op", "openai", "m", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "网关不支持操作")
}

func TestResolveModelImageSharesResolveRoute(t *testing.T) {
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

func TestResolvePipelineDefaultsAndOverrides(t *testing.T) {
	route, err := Resolve(workflow.RuntimeOperationCutout, "fal", "birefnet", "", "")
	require.NoError(t, err)
	assert.Equal(t, "v1/images/cutout", route.Path)
	assert.Equal(t, workflow.GatewayProviderPath("fal"), route.Namespace)
	assert.IsType(t, &FalDialect{}, route.Dialect)

	route, err = Resolve(workflow.RuntimeOperationUpscale, "fal", "aura-sr", "custom/upscale/v2", "")
	require.NoError(t, err)
	assert.Equal(t, "custom/upscale/v2", route.Path)
	assert.Equal(t, workflow.GatewayPathSourceOverride, route.Source)

	// Pipeline dialect keeps the legacy empty-params judgment: a chat
	// customParams flips model-image shaping but never pipeline decoding.
	modelImage, err := Resolve(workflow.RuntimeOperationGeneration, "openai", "dall-e-3", "", `{"type":"chat"}`)
	require.NoError(t, err)
	assert.IsType(t, &ChatDialect{}, modelImage.Dialect)
	pipeline, err := Resolve(workflow.RuntimeOperationCutout, "openai", "dall-e-3", "", `{"type":"chat"}`)
	require.NoError(t, err)
	assert.IsType(t, &GenericDialect{}, pipeline.Dialect)
}
