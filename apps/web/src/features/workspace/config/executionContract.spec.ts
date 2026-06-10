import { afterEach, describe, expect, it, vi } from 'vitest'

describe('buildExecutionTaskRequest', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('canonicalizes first-party upload URLs before task submission', async () => {
    vi.stubEnv('VITE_PUBLIC_ASSET_BASE_URL', 'https://assets.example.com')
    vi.resetModules()

    const { buildExecutionTaskRequest } = await import('./executionContract')

    const request = buildExecutionTaskRequest({
      prompt: 'demo',
      imageUrls: ['https://assets.example.com/uploads/workspace/U123/board/content/a.png'],
      params: {
        original_image_url: 'https://assets.example.com/uploads/workspace/U123/board/content/a.png',
        reference_image_urls: [
          'https://assets.example.com/uploads/workspace/U123/board/content/b.png',
          'https://cdn.example.com/uploads/workspace/U123/board/content/c.png',
        ],
      },
    })

    expect(request.image_urls).toEqual(['/uploads/workspace/U123/board/content/a.png'])
    expect(request.params).toEqual({
      original_image_url: '/uploads/workspace/U123/board/content/a.png',
      reference_image_urls: [
        '/uploads/workspace/U123/board/content/b.png',
        'https://cdn.example.com/uploads/workspace/U123/board/content/c.png',
      ],
    })
  })
})
