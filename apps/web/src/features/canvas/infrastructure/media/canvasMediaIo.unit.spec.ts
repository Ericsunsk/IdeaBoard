import { afterEach, describe, expect, it, vi } from 'vitest'
import { filenameFromImageUrl, inferImageExtension, resolveFetchUrl } from './canvasMediaIo'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe('flow media URL resolution', () => {
  it('uses the same-origin upload proxy for local API upload URLs', () => {
    expect(
      resolveFetchUrl('http://localhost:6000/uploads/canvas/U123/board/content/image.png'),
    ).toBe('http://localhost:3000/uploads/canvas/U123/board/content/image.png')
  })

  it('routes API media asset URLs through the authenticated download boundary', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()
    const { resolveFetchUrl: resolveConfiguredFetchUrl } = await import('./canvasMediaIo')
    const mediaAssetURL = 'https://api.example.com/api/user/media-assets/3/original/content'

    expect(resolveConfiguredFetchUrl(mediaAssetURL)).toBe(
      '/api/user/download?url=' + encodeURIComponent(mediaAssetURL),
    )
  })

  it.each([
    ['image/avif', 'avif'],
    ['image/heic', 'heic'],
    ['image/heif', 'heif'],
  ])('infers %s as .%s', (mime, extension) => {
    expect(inferImageExtension(mime)).toBe(extension)
  })

  it('keeps AVIF extension when creating a stabilization filename', () => {
    expect(filenameFromImageUrl('https://cdn.example.com/image.jpg', 'image/avif')).toBe(
      'image.avif',
    )
  })
})
