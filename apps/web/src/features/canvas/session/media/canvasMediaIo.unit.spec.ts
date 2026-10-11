import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  fetchImageBlobFromUrl,
  filenameFromImageUrl,
  ImageBlobFetchError,
  inferImageExtension,
  isStableBoardUploadUrl,
  parseImageUrlPathname,
  resolveFetchUrl,
} from './canvasMediaIo'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe('flow media URL resolution', () => {
  it('uses the same-origin upload proxy for local API upload URLs', () => {
    expect(
      resolveFetchUrl('http://localhost:6000/uploads/tasks/U123/board/content/image.png'),
    ).toBe('http://localhost:3000/uploads/tasks/U123/board/content/image.png')
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

describe('canvas upload URL stability', () => {
  it('parses bare paths and absolute URLs to pathnames', () => {
    expect(parseImageUrlPathname('/uploads/tasks/u1/b1/content/a.png')).toBe(
      '/uploads/tasks/u1/b1/content/a.png',
    )
    expect(parseImageUrlPathname('https://cdn.example.com/a/b.png?x=1')).toBe('/a/b.png')
    expect(parseImageUrlPathname('   ')).toBeNull()
    expect(parseImageUrlPathname('http://')).toBeNull()
  })

  it('treats board-owned task uploads as stable without re-upload', () => {
    expect(
      isStableBoardUploadUrl(
        '/uploads/tasks/user-uid/board-a/content/a.png',
        'user-uid',
        'board-a',
      ),
    ).toBe(true)
    expect(isStableBoardUploadUrl('  ', 'user-uid', 'board-a')).toBe(true)
    expect(
      isStableBoardUploadUrl('https://provider.example/image.png', 'user-uid', 'board-a'),
    ).toBe(false)
    expect(
      isStableBoardUploadUrl(
        '/uploads/tasks/other-user/board-a/content/a.png',
        'user-uid',
        'board-a',
      ),
    ).toBe(false)
  })

  it('matches encoded user and board segments in stable paths', () => {
    expect(
      isStableBoardUploadUrl(
        '/uploads/tasks/user%20uid/board%20a/content/a.png',
        'user uid',
        'board a',
      ),
    ).toBe(true)
  })
})

describe('shared image blob fetch', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the blob for ok responses with credentials', async () => {
    const blob = new Blob(['image'], { type: 'image/png' })
    const fetchMock = vi.fn(async () => new Response(blob, { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(fetchImageBlobFromUrl('https://cdn.example.com/a.png')).resolves.toBeInstanceOf(
      Blob,
    )
    expect(fetchMock).toHaveBeenCalledWith(
      'https://cdn.example.com/a.png',
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('throws a status-carrying error for non-ok responses', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('nope', { status: 404 })),
    )

    const error = await fetchImageBlobFromUrl('https://cdn.example.com/a.png').catch(
      (reason: unknown) => reason,
    )
    expect(error).toBeInstanceOf(ImageBlobFetchError)
    expect((error as ImageBlobFetchError).status).toBe(404)
  })

  it('throws a null-status error when the network fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('offline')
      }),
    )

    const error = await fetchImageBlobFromUrl('https://cdn.example.com/a.png').catch(
      (reason: unknown) => reason,
    )
    expect(error).toBeInstanceOf(ImageBlobFetchError)
    expect((error as ImageBlobFetchError).status).toBeNull()
  })
})
