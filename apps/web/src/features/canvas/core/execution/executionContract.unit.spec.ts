import { afterEach, describe, expect, it, vi } from 'vitest'

describe('buildExecutionTaskRequest', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('keeps only the typed media fields at the task boundary', async () => {
    const { buildExecutionTaskRequest } = await import('./executionContract')

    const request = buildExecutionTaskRequest({
      prompt: 'demo',
      mediaAssetIds: [' asset_reference ', 'asset_reference'],
      params: {
        mask_asset_id: 'asset_mask',
      },
    })

    expect(request.media_asset_ids).toEqual(['asset_reference'])
    expect(request).not.toHaveProperty('image_urls')
    expect(request.params).toEqual({
      mask_asset_id: 'asset_mask',
    })
  })
})
