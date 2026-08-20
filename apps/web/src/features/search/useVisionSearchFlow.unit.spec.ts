import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useVisionSearchFlow } from './useVisionSearchFlow'

const mocks = vi.hoisted(() => ({
  route: { path: '/search', fullPath: '/search', query: {} as Record<string, unknown> },
  replace: vi.fn(async () => undefined),
  extractVisionSearch: vi.fn(),
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
    expect(mocks.replace).toHaveBeenCalledOnce()
  })

  it('keeps extraction state separate from Catalog loading', async () => {
    const loadCatalog = vi.fn(async () => undefined)
    const submittedQuery = ref('')
    const flow = useVisionSearchFlow({
      draftQuery: ref('warm cafe'),
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
      palette: ['#d97706'],
      atmosphere_tags: ['soft light'],
    })

    await flow.runVisionSearch()

    expect(submittedQuery.value).toBe('warm minimalist cafe')
    expect(flow.selectedPalette.value).toEqual(new Set(['#d97706']))
    expect(flow.selectedTags.value).toEqual(new Set(['soft light']))
    expect(loadCatalog).toHaveBeenCalledOnce()
  })

  it('ignores an extraction response after its references become stale', async () => {
    let resolveExtraction!: (value: {
      query: string
      palette: string[]
      atmosphere_tags: string[]
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
      palette: ['#ffffff'],
      atmosphere_tags: ['stale'],
    })
    await pending

    expect(flow.visionState.value).toBe('idle')
    expect(draftQuery.value).toBe('original query')
    expect(mocks.replace).toHaveBeenCalledOnce()
  })

  it('keeps palette and atmosphere changes made within one debounce window', () => {
    vi.useFakeTimers()
    const flow = useVisionSearchFlow({
      draftQuery: ref('cafe'),
      submittedQuery: ref('cafe'),
      queryError: ref(''),
      keepResultsOnNextLoad: ref(false),
      loadCatalog: vi.fn(async () => undefined),
    })

    flow.togglePalette('#d97706')
    flow.toggleTag('soft light')
    vi.advanceTimersByTime(180)

    expect(mocks.replace).toHaveBeenCalledWith({
      query: {
        palette: ['#d97706'],
        atmosphere_tag: ['soft light'],
      },
    })
  })
})
