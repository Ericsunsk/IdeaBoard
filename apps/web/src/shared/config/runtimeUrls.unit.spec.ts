import { afterEach, describe, expect, it, vi } from 'vitest'

describe('runtime URL helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('resolves and canonicalizes configured public upload URLs', async () => {
    vi.stubEnv('VITE_PUBLIC_ASSET_BASE_URL', 'https://assets.example.com')
    vi.resetModules()

    const { canonicalizeRuntimeUploadRef, resolveAssetUrl } = await import('./runtimeUrls')

    expect(resolveAssetUrl('/uploads/canvas/U123/board/content/a.png')).toBe(
      'https://assets.example.com/uploads/canvas/U123/board/content/a.png',
    )
    expect(
      canonicalizeRuntimeUploadRef(
        'https://assets.example.com/uploads/canvas/U123/board/content/a.png',
      ),
    ).toBe('/uploads/canvas/U123/board/content/a.png')
    expect(canonicalizeRuntimeUploadRef('https://cdn.example.com/uploads/canvas/U123/a.png')).toBe(
      'https://cdn.example.com/uploads/canvas/U123/a.png',
    )
  })

  it('derives the Auth base URL from the configured API base URL', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()

    const { resolveAuthUrl } = await import('./runtimeUrls')

    expect(resolveAuthUrl('/api/auth/get-session')).toBe(
      'https://api.example.com/api/auth/get-session',
    )
  })

  it('canonicalizes local API upload URLs for the Vite proxy', async () => {
    vi.resetModules()

    const { canonicalizeRuntimeUploadRef, normalizeRuntimeMediaRefs } =
      await import('./runtimeUrls')

    expect(
      canonicalizeRuntimeUploadRef(
        'http://localhost:6000/uploads/canvas/U123/board/content/image.png',
      ),
    ).toBe('/uploads/canvas/U123/board/content/image.png')
    expect(
      normalizeRuntimeMediaRefs({
        sourceImageUrl: 'http://localhost:6000/uploads/canvas/U123/board/content/image.png',
      }),
    ).toEqual({ sourceImageUrl: '/uploads/canvas/U123/board/content/image.png' })
  })
})
