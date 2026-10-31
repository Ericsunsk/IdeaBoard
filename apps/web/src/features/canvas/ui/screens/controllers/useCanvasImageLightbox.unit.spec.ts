import { afterEach, describe, expect, it, vi } from 'vitest'

describe('useCanvasImageLightbox', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('resolves runtime upload paths before opening the preview', async () => {
    const { normalizeImageUrl } = await import('./useCanvasImageLightbox')

    expect(normalizeImageUrl('/uploads/tasks/user-1/board-1/content/adjusted.png')).toBe(
      '/uploads/tasks/user-1/board-1/content/adjusted.png',
    )
  })
})
