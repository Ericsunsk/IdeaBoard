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

  it('keeps v1 workspace media asset URLs on the authenticated API origin', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()
    const { resolveFetchUrl: resolveConfiguredFetchUrl } = await import('./canvasMediaIo')
    const mediaAssetURL =
      'https://api.example.com/api/v1/workspaces/ws_test123/media-assets/asset_3/content'

    expect(resolveConfiguredFetchUrl(mediaAssetURL)).toBe(mediaAssetURL)
  })

  it('resolves external image URLs directly without a legacy download proxy', () => {
    expect(resolveFetchUrl('https://cdn.example.com/image.jpg')).toBe(
      'https://cdn.example.com/image.jpg',
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
