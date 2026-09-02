import { afterEach, describe, expect, it, vi } from 'vitest'

describe('useFlowImagePreviewLightbox', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('resolves runtime upload paths before opening the preview', async () => {
    const { normalizeImageUrl } = await import('./useFlowImagePreviewLightbox')

    expect(normalizeImageUrl('/uploads/canvas/user-1/board-1/content/adjusted.png')).toBe(
      '/uploads/canvas/user-1/board-1/content/adjusted.png',
    )
  })
})
