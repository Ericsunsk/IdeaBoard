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
      imageUrls: ['https://assets.example.com/reference.png'],
      params: {
        mask_image_url: 'https://assets.example.com/mask.png',
      },
    })

    expect(request.image_urls).toEqual(['https://assets.example.com/reference.png'])
    expect(request.params).toEqual({
      mask_image_url: 'https://assets.example.com/mask.png',
    })
  })
})
