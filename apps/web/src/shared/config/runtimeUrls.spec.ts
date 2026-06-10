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

    expect(resolveAssetUrl('/uploads/workspace/U123/board/content/a.png')).toBe(
      'https://assets.example.com/uploads/workspace/U123/board/content/a.png',
    )
    expect(
      canonicalizeRuntimeUploadRef(
        'https://assets.example.com/uploads/workspace/U123/board/content/a.png',
      ),
    ).toBe('/uploads/workspace/U123/board/content/a.png')
    expect(
      canonicalizeRuntimeUploadRef('https://cdn.example.com/uploads/workspace/U123/a.png'),
    ).toBe('https://cdn.example.com/uploads/workspace/U123/a.png')
  })
})
