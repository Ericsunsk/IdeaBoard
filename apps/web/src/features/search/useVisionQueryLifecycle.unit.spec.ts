import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { clearCachedVisionReferences, setCachedVisionReferences } from './visionReferencePersistence'
import { useVisionQueryLifecycle } from './useVisionQueryLifecycle'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(async () => undefined),
  extractVisionSearch: vi.fn(),
  fetchPrice: vi.fn(),
  requireLogin: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('vue-router', () => ({ useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('../../shared/composables/useAppToast', () => ({ useAppToast: () => mocks.toast }))
vi.mock('./visionSearchBillingResource', () => ({ fetchVisionSearchPrice: mocks.fetchPrice }))
vi.mock('./searchResource', () => ({
  extractVisionSearch: mocks.extractVisionSearch,
  normalizeSearchQuery: (value: string) => value.trim(),
  isSearchQueryWithinLimit: (value: string) => value.length <= 200,
}))
vi.mock('../library/libraryResource', () => ({
  libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
}))

function createLifecycle(options: { authenticated?: boolean; scope?: string } = {}) {
  const authenticated = ref(options.authenticated ?? true)
  const submittedQuery = ref('')
  const draftQuery = ref('warm cafe')
  const queryError = ref('')
  const loadResults = vi.fn(async () => undefined)
  const lifecycle = useVisionQueryLifecycle({
    draftQuery,
    submittedQuery,
    queryError,
    keepResultsOnNextLoad: ref(false),
    loadResults,
    isAuthenticated: () => authenticated.value,
    requireLogin: mocks.requireLogin,
    cacheScope: options.scope,
  })
  return { lifecycle, authenticated, draftQuery, submittedQuery, queryError, loadResults }
}

const libraryAsset = {
  id: 7,
  provider: 'unsplash',
  source_id: 'photo-1',
  preview_url: 'https://images.example/photo-1.jpg',
}

describe('useVisionQueryLifecycle', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
    vi.clearAllMocks()
    mocks.fetchPrice.mockResolvedValue(12)
  })

  it('restores a cached Library draft for editing without submitting a paid extraction', () => {
    setCachedVisionReferences(
      [{ key: 'unsplash:cached', previewUrl: 'https://images.example/cached.jpg', libraryAssetID: 1 }],
      'user-a',
    )

    const { lifecycle } = createLifecycle({ scope: 'user-a' })

    expect(lifecycle.references.value.map((reference) => reference.key)).toEqual(['unsplash:cached'])
    expect(mocks.extractVisionSearch).not.toHaveBeenCalled()
    lifecycle.dispose()
    clearCachedVisionReferences('user-a')
  })

  it('accepts Library, File, and Search result references through one lifecycle', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        headers: { get: () => 'image/jpeg' },
        blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
      })),
    )
    const { lifecycle } = createLifecycle()

    lifecycle.addLibraryAsset(libraryAsset as never)
    lifecycle.handleFileInput({
      target: { files: [new File(['file'], 'local.jpg', { type: 'image/jpeg' })], value: 'selected' },
    } as unknown as Event)
    await lifecycle.addSearchResult({
      provider: 'pexels',
      id: 'photo-2',
      preview_url: 'https://images.example/photo-2.jpg',
      thumbnail_url: 'https://images.example/thumb-2.jpg',
    } as never)

    expect(lifecycle.references.value).toHaveLength(3)
    expect(lifecycle.references.value[2]?.file).toBeInstanceOf(File)
    expect(lifecycle.hasPendingReferences.value).toBe(false)
    lifecycle.dispose()
  })

  it('gates paid extraction behind explicit authentication and only resumes on command', async () => {
    const { lifecycle, authenticated } = createLifecycle({ authenticated: false })
    lifecycle.addLibraryAsset(libraryAsset as never)

    await lifecycle.submit()

    expect(mocks.requireLogin).toHaveBeenCalledWith({ kind: 'vision-search', query: 'warm cafe' })
    expect(mocks.extractVisionSearch).not.toHaveBeenCalled()
    authenticated.value = true
    mocks.extractVisionSearch.mockResolvedValue({ status: 'succeeded', query: 'warm cafe' })
    await lifecycle.resumeAfterAuthentication()
    expect(mocks.extractVisionSearch).toHaveBeenCalledOnce()
    lifecycle.dispose()
  })

  it('owns quote feedback, idempotent paid extraction, and feed handoff', async () => {
    const { lifecycle, submittedQuery, loadResults } = createLifecycle()
    lifecycle.addLibraryAsset(libraryAsset as never)
    mocks.extractVisionSearch.mockResolvedValue({ status: 'succeeded', query: 'warm editorial cafe' })

    await lifecycle.submit()

    expect(lifecycle.buttonTitle.value).toBe('Search · 12 Ideas')
    expect(mocks.extractVisionSearch).toHaveBeenCalledWith(
      expect.objectContaining({ userText: 'warm cafe', references: lifecycle.references.value }),
    )
    expect(submittedQuery.value).toBe('warm editorial cafe')
    expect(loadResults).toHaveBeenCalledOnce()
    expect(lifecycle.state.value).toBe('ready')
    lifecycle.dispose()
  })

  it('uses a new idempotency key after the text or reference request facts change', async () => {
    const { lifecycle, draftQuery } = createLifecycle()
    lifecycle.addLibraryAsset(libraryAsset as never)
    mocks.extractVisionSearch.mockResolvedValue({ status: 'succeeded', query: 'warm cafe' })

    await lifecycle.submit()
    const firstKey = mocks.extractVisionSearch.mock.calls[0]?.[0].idempotencyKey
    draftQuery.value = 'warm brutalist cafe'
    await lifecycle.submit()
    const textChangedKey = mocks.extractVisionSearch.mock.calls[1]?.[0].idempotencyKey
    expect(textChangedKey).not.toBe(firstKey)
    lifecycle.addLibraryAsset({ ...libraryAsset, id: 8, source_id: 'photo-2' } as never)
    await lifecycle.submit()
    expect(mocks.extractVisionSearch.mock.calls[2]?.[0].idempotencyKey).not.toBe(textChangedKey)
    lifecycle.dispose()
  })

  it('shares an in-flight logical request and reuses its idempotency key on retry', async () => {
    let resolveExtraction!: (value: { status: string; query: string }) => void
    mocks.extractVisionSearch.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExtraction = resolve
        }),
    )
    const { lifecycle } = createLifecycle()
    lifecycle.addLibraryAsset(libraryAsset as never)

    const first = lifecycle.submit()
    const second = lifecycle.submit()
    expect(second).toBe(first)
    expect(mocks.extractVisionSearch).toHaveBeenCalledOnce()
    resolveExtraction({ status: 'succeeded', query: 'warm cafe' })
    await first

    mocks.extractVisionSearch.mockResolvedValueOnce({ status: 'succeeded', query: 'warm cafe' })
    await lifecycle.submit()
    expect(mocks.extractVisionSearch).toHaveBeenCalledTimes(2)
    expect(mocks.extractVisionSearch.mock.calls[1]?.[0].idempotencyKey).toBe(
      mocks.extractVisionSearch.mock.calls[0]?.[0].idempotencyKey,
    )
    lifecycle.dispose()
  })

  it('cancels preparation and extraction when a reference is removed', async () => {
    let resolveFetch!: (value: { ok: boolean; headers: { get: () => string }; blob: () => Promise<Blob> }) => void
    let signal!: AbortSignal
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise((resolve) => {
            signal = init?.signal as AbortSignal
            resolveFetch = resolve
          }),
      ),
    )
    const { lifecycle } = createLifecycle()
    const pending = lifecycle.addSearchResult({
      provider: 'unsplash',
      id: 'photo-cancelled',
      preview_url: 'https://images.example/photo-cancelled.jpg',
    } as never)
    await vi.waitFor(() => expect(lifecycle.hasPendingReferences.value).toBe(true))

    lifecycle.removeReference('unsplash:photo-cancelled')
    expect(signal.aborted).toBe(true)
    resolveFetch({
      ok: true,
      headers: { get: () => 'image/jpeg' },
      blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
    })
    await pending

    expect(lifecycle.references.value).toHaveLength(0)
    lifecycle.dispose()
  })

  it('prepares batch references concurrently and aborts all preparations on clear', async () => {
    const pendingResponses: Array<(value: { ok: boolean; headers: { get: () => string }; blob: () => Promise<Blob> }) => void> = []
    const signals: AbortSignal[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise((resolve) => {
            signals.push(init?.signal as AbortSignal)
            pendingResponses.push(resolve)
          }),
      ),
    )
    const { lifecycle } = createLifecycle()
    const pending = lifecycle.addSearchResults([
      { provider: 'unsplash', id: 'first', preview_url: 'https://images.example/first.jpg' },
      { provider: 'pexels', id: 'second', preview_url: 'https://images.example/second.jpg' },
    ] as never)
    await vi.waitFor(() => expect(signals).toHaveLength(2))

    lifecycle.clearReferences()
    expect(signals.every((signal) => signal.aborted)).toBe(true)
    for (const resolve of pendingResponses) {
      resolve({
        ok: true,
        headers: { get: () => 'image/jpeg' },
        blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
      })
    }
    await pending

    expect(lifecycle.references.value).toEqual([])
    lifecycle.dispose()
  })
})
