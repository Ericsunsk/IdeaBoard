import { afterEach, describe, expect, it, vi } from 'vitest'

describe('runtime URL helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('keeps upload refs same-origin and canonicalizes known upload hosts', async () => {
    const { canonicalizeRuntimeUploadRef, resolveAssetUrl } = await import('./runtimeUrls')

    expect(resolveAssetUrl('/uploads/canvas/U123/board/content/a.png')).toBe('/uploads/canvas/U123/board/content/a.png')
    expect(
      canonicalizeRuntimeUploadRef(
        'https://assets.example.com/uploads/canvas/U123/board/content/a.png',
      ),
    ).toBe('https://assets.example.com/uploads/canvas/U123/board/content/a.png')
    expect(canonicalizeRuntimeUploadRef('https://cdn.example.com/uploads/canvas/U123/a.png')).toBe(
      'https://cdn.example.com/uploads/canvas/U123/a.png',
    )
  })

  it('derives the Auth base URL from the configured API base URL', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()

    const { resolveAuthUrl } = await import('./runtimeUrls')

    expect(resolveAuthUrl('/api/auth/get-session')).toBe('/api/auth/get-session')
  })

  it('forces same-origin API and asset bases for production builds', async () => {
    vi.stubEnv('MODE', 'production')
    vi.stubEnv('PROD', true)
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.example.com/api')
    vi.resetModules()

    const { API_BASE_URL, resolveApiUrl, resolveAssetUrl } = await import('./runtimeUrls')

    expect(API_BASE_URL).toBe('/api')
    expect(resolveApiUrl('/user/tasks')).toBe('/api/user/tasks')
    expect(resolveAssetUrl('/uploads/image.png')).toBe('/uploads/image.png')
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

  it('ignores malformed development API and asset overrides', async () => {
    vi.stubEnv('VITE_API_BASE_URL', 'https://user:pass@example.com/api?token=leak')
    vi.resetModules()

    const { API_BASE_URL } = await import('./runtimeUrls')

    expect(API_BASE_URL).toBe('/api')
  })
})
