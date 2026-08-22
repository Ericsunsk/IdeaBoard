import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useVisionSearchFlow } from './useVisionSearchFlow'

const mocks = vi.hoisted(() => ({
  route: { path: '/search', fullPath: '/search', query: {} as Record<string, unknown> },
  replace: vi.fn(async () => undefined),
  extractVisionSearch: vi.fn(),
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

vi.mock('../../shared/composables/useAppToast', () => ({
  useAppToast: () => mocks.toast,
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('./searchResource', () => ({
  extractVisionSearch: mocks.extractVisionSearch,
  normalizeSearchQuery: (value: string) => value.trim(),
}))

vi.mock('../library/libraryResource', () => ({
  libraryAssetKey: (provider: string, sourceID: string) => `${provider}:${sourceID}`,
}))

describe('useVisionSearchFlow', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    mocks.route.query = {}
    mocks.route.fullPath = '/search'
    vi.clearAllMocks()
  })

  it('dedupes Library references and marks extraction stale', () => {
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })
    const asset = {
      provider: 'unsplash',
      source_id: 'photo-1',
      original_url: 'https://images.unsplash.com/original',
      preview_url: 'https://images.unsplash.com/preview',
    }

    flow.handleLibraryReference(asset as never)
    flow.handleLibraryReference(asset as never)

    expect(flow.references.value).toHaveLength(1)
    expect(flow.visionState.value).toBe('idle')
    expect(mocks.replace).not.toHaveBeenCalled()
  })

  it('keeps extraction state separate from Catalog loading', async () => {
    const loadCatalog = vi.fn(async () => undefined)
    const submittedQuery = ref('')
    const draftQuery = ref('warm cafe')
    const flow = useVisionSearchFlow({
      draftQuery,
      submittedQuery,
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog,
    })
    flow.references.value = [
      {
        libraryAssetID: 1,
        previewUrl: 'https://images.unsplash.com/preview',
        key: 'unsplash:photo-1',
      },
    ]
    mocks.extractVisionSearch.mockResolvedValue({
      query: 'warm minimalist cafe',
    })

    await flow.runVisionSearch()

    expect(submittedQuery.value).toBe('warm minimalist cafe')
    expect(draftQuery.value).toBe('warm cafe')
    expect(flow.references.value).toHaveLength(1)
    expect(loadCatalog).toHaveBeenCalledOnce()
  })

  it('ignores an extraction response after its references become stale', async () => {
    let resolveExtraction!: (value: {
      query: string
    }) => void
    mocks.extractVisionSearch.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExtraction = resolve
        }),
    )
    const draftQuery = ref('original query')
    const flow = useVisionSearchFlow({
      draftQuery,
      submittedQuery: ref(''),
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })
    flow.references.value = [
      {
        libraryAssetID: 1,
        previewUrl: 'https://images.unsplash.com/preview',
        key: 'unsplash:photo-1',
      },
    ]

    const pending = flow.runVisionSearch()
    flow.removeReference('unsplash:photo-1')
    resolveExtraction({
      query: 'stale query',
    })
    await pending

    expect(flow.visionState.value).toBe('idle')
    expect(draftQuery.value).toBe('original query')
    expect(mocks.replace).toHaveBeenCalledOnce()
  })

  it('clears all references and resets vision extraction', () => {
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref('cafe'),
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })
    flow.references.value = [
      { previewUrl: 'blob:mock-1', key: 'local:1' },
      { previewUrl: 'blob:mock-2', key: 'local:2' },
    ]
    flow.clearAllReferences()

    expect(flow.references.value).toHaveLength(0)
    expect(flow.visionState.value).toBe('idle')
  })

  it('handles search result reference selection and deduplication', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        headers: { get: () => 'image/jpeg' },
        blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
      })),
    )
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })
    const searchResult = {
      provider: 'unsplash',
      id: 'photo-100',
      preview_url: 'https://images.unsplash.com/preview-100',
      source_url: 'https://unsplash.com/photos/photo-100',
      original_url: 'https://images.unsplash.com/original-100',
      title: 'Coffee in cafe',
      author: 'Photographer',
      width: 800,
      height: 600,
      media_kind: 'image',
    }

    await flow.handleSearchResultReference(searchResult as never)
    await flow.handleSearchResultReference(searchResult as never)

    expect(flow.references.value).toHaveLength(1)
    expect(flow.references.value[0].key).toBe('unsplash:photo-100')
    expect(flow.references.value[0].previewUrl).toBe('https://images.unsplash.com/preview-100')
    expect(flow.references.value[0].file).toBeInstanceOf(File)
    expect(flow.hasPendingReferences.value).toBe(false)
  })

  it('removes a search result reference when its preview cannot be fetched', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        headers: { get: () => 'text/html' },
        blob: async () => new Blob(['not an image'], { type: 'text/html' }),
      })),
    )
    const queryError = ref('')
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError,
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })

    await flow.handleSearchResultReference({
      provider: 'pexels',
      id: 'photo-200',
      preview_url: 'https://images.pexels.com/preview-200',
    } as never)

    expect(flow.references.value).toHaveLength(0)
    expect(queryError.value).toBe('该图片无法作为搜索参考，请重试')
    expect(flow.hasPendingReferences.value).toBe(false)
  })

  it('blocks Vision extraction while a search result reference is preparing', async () => {
    let resolveFetch!: (value: {
      ok: boolean
      headers: { get: () => string }
      blob: () => Promise<Blob>
    }) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            resolveFetch = resolve
          }),
      ),
    )
    const queryError = ref('')
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError,
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })

    const pendingReference = flow.handleSearchResultReference({
      provider: 'unsplash',
      id: 'photo-300',
      preview_url: 'https://images.unsplash.com/preview-300',
    } as never)

    expect(flow.hasPendingReferences.value).toBe(true)
    await flow.runVisionSearch()
    expect(mocks.extractVisionSearch).not.toHaveBeenCalled()
    expect(queryError.value).toBe('正在准备参考图，请稍候')

    resolveFetch({
      ok: true,
      headers: { get: () => 'image/jpeg' },
      blob: async () => new Blob(['image'], { type: 'image/jpeg' }),
    })
    await pendingReference
    expect(flow.hasPendingReferences.value).toBe(false)
    expect(queryError.value).toBe('')
  })

  it('counts pending and visible references as the same slots', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => undefined)))
    const queryError = ref('')
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError,
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })

    for (let index = 0; index < 4; index += 1) {
      void flow.handleSearchResultReference({
        provider: 'unsplash',
        id: `photo-${index}`,
        preview_url: `https://images.unsplash.com/preview-${index}`,
      } as never)
    }
    await vi.waitFor(() => expect(flow.references.value).toHaveLength(4))

    expect(flow.referenceSlotCount.value).toBe(4)
    expect(flow.canAddReference.value).toBe(false)

    flow.handleFileInput({
      target: {
        files: [new File(['image'], 'extra.jpg', { type: 'image/jpeg' })],
        value: 'selected',
      },
    } as unknown as Event)

    expect(flow.references.value).toHaveLength(4)
    expect(mocks.toast.warning).toHaveBeenCalledWith('You can add up to 4 reference images')
    flow.clearAllReferences()
  })

  it('enforces the total byte limit across concurrent remote references', async () => {
    const sixMegabytes = new Uint8Array(6 * 1024 * 1024)
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        headers: { get: () => 'image/jpeg' },
        blob: async () => new Blob([sixMegabytes], { type: 'image/jpeg' }),
      })),
    )
    const queryError = ref('')
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref(''),
      queryError,
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })

    await Promise.all([
      flow.handleSearchResultReference({
        provider: 'unsplash',
        id: 'large-1',
        preview_url: 'https://images.unsplash.com/large-1',
      } as never),
      flow.handleSearchResultReference({
        provider: 'pexels',
        id: 'large-2',
        preview_url: 'https://images.pexels.com/large-2',
      } as never),
    ])

    expect(flow.references.value).toHaveLength(1)
    expect(flow.references.value[0].file?.size).toBe(6 * 1024 * 1024)
    expect(queryError.value).toBe('该图片无法作为搜索参考，请重试')
    expect(flow.hasPendingReferences.value).toBe(false)
  })
})
