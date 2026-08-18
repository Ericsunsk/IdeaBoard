import { afterEach, describe, expect, it, vi } from 'vitest'

describe('useFlowImagePreviewLightbox', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('resolves runtime upload paths before opening the preview', async () => {
    vi.stubEnv('VITE_PUBLIC_ASSET_BASE_URL', 'https://assets.example.com')
    vi.resetModules()

    const { normalizeImageUrl } = await import('./useFlowImagePreviewLightbox')

    expect(normalizeImageUrl('/uploads/canvas/user-1/board-1/content/adjusted.png')).toBe(
      'https://assets.example.com/uploads/canvas/user-1/board-1/content/adjusted.png',
    )
  })
})
