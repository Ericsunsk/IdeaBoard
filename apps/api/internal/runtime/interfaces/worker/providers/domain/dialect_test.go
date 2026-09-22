package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"ideaboard/api/internal/workflow"
)

func TestAdapterForOperation(t *testing.T) {
	adapter, err := AdapterForOperation(workflow.RuntimeOperationInpaint)
	require.NoError(t, err)
	assert.Equal(t, ProviderRuntimeAdapterModelImage, adapter)

	adapter, err = AdapterForOperation(workflow.RuntimeOperationCutout)
	require.NoError(t, err)
	assert.Equal(t, ProviderRuntimeAdapterCutout, adapter)

	adapter, err = AdapterForOperation(workflow.RuntimeOperationRender)
	require.NoError(t, err)
	assert.Equal(t, ProviderRuntimeAdapterModelImage, adapter)

	_, err = AdapterForOperation("invalid_op")
	require.Error(t, err)
}
