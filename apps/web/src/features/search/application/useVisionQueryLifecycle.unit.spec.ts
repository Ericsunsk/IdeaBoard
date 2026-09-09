import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import {
  clearCachedVisionReferences,
  setCachedVisionReferences,
} from '../infrastructure/visionReferencePersistence'
import { useVisionQueryLifecycle } from './useVisionQueryLifecycle'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(async () => undefined),
  extractVisionSearch: vi.fn(),
  fetchPrice: vi.fn(),
  requireLogin: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('vue-router', () => ({ useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('../../../shared/composables/useAppToast', () => ({ useAppToast: () => mocks.toast }))
vi.mock('../infrastructure/visionBillingResource', () => ({
  fetchVisionSearchPrice: mocks.fetchPrice,
}))
vi.mock('../infrastructure/searchResource', () => ({
  extractVisionSearch: mocks.extractVisionSearch,
}))
vi.mock('../../library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../library')>()
  return {
    ...actual,
    libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
  }
})

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
      [
        {
          key: 'unsplash:cached',
          previewUrl: 'https://images.example/cached.jpg',
          libraryAssetID: 1,
        },
      ],
      'user-a',
    )

    const { lifecycle } = createLifecycle({ scope: 'user-a' })

    expect(lifecycle.references.value.map((reference) => reference.key)).toEqual([
      'unsplash:cached',
    ])
    expect(mocks.extractVisionSearch).not.toHaveBeenCalled()
    lifecycle.dispose()
    clearCachedVisionReferences('user-a')
  })

  it('accepts Library, File, and Search result references without browser fetching external image', async () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    const { lifecycle } = createLifecycle()

    lifecycle.addLibraryAsset(libraryAsset as never)
    lifecycle.handleFileInput({
      target: {
        files: [new File(['file'], 'local.jpg', { type: 'image/jpeg' })],
        value: 'selected',
      },
    } as unknown as Event)
    lifecycle.addSearchResult({
      provider: 'pexels',
      id: 'photo-2',
      preview_url: 'https://images.example/photo-2.jpg',
      thumbnail_url: 'https://images.example/thumb-2.jpg',
    } as never)

    expect(lifecycle.references.value).toHaveLength(3)
    expect(lifecycle.references.value[0]?.libraryAssetID).toBe(7)
    expect(lifecycle.references.value[1]?.file).toBeInstanceOf(File)
    expect(lifecycle.references.value[2]?.externalReference).toEqual({
      provider: 'pexels',
      id: 'photo-2',
      preview_url: 'https://images.example/photo-2.jpg',
    })
    expect(fetchSpy).not.toHaveBeenCalled()
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
    mocks.extractVisionSearch.mockResolvedValue({
      status: 'succeeded',
      query: 'warm editorial cafe',
    })

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

  it('removes a reference and updates route when last reference is removed', () => {
    const { lifecycle } = createLifecycle()
    lifecycle.addSearchResult({
      provider: 'unsplash',
      id: 'photo-cancelled',
      preview_url: 'https://images.example/photo-cancelled.jpg',
    } as never)
    expect(lifecycle.references.value).toHaveLength(1)

    lifecycle.removeReference('unsplash:photo-cancelled')
    expect(lifecycle.references.value).toHaveLength(0)
    expect(mocks.replace).toHaveBeenCalledWith(
      expect.objectContaining({ path: '/user/search', query: { q: 'warm cafe' } }),
    )
    lifecycle.dispose()
  })

  it('adds batch references directly and clears them on clear', () => {
    const { lifecycle } = createLifecycle()
    lifecycle.addSearchResults([
      { provider: 'unsplash', id: 'first', preview_url: 'https://images.example/first.jpg' },
      { provider: 'pexels', id: 'second', preview_url: 'https://images.example/second.jpg' },
    ] as never)

    expect(lifecycle.references.value).toHaveLength(2)
    lifecycle.clearReferences()
    expect(lifecycle.references.value).toEqual([])
    lifecycle.dispose()
  })
})
