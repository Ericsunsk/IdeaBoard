import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { beforeEach, describe, expect, it, test, vi } from 'vitest'
import { computed, effectScope, nextTick, reactive, ref, type Ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { SearchResult, VisionReference } from '../core/models'
import { mockAppToast } from '../../../test/toast'
import { useSearchSession } from './useSearchSession'
import {
  clearPendingSearchAuthAction,
  setPendingSearchAuthAction,
} from './adapters/authContinuation'
import { createSearchLibraryCoordinator } from './adapters/libraryCoordinator'
import { getLibraryIslandState } from './adapters/libraryIslandPersistence'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string | string[]> },
  replace: vi.fn(async () => undefined),
  push: vi.fn(async () => undefined),
  load: vi.fn(async () => undefined),
  markSaved: vi.fn(),
  markSavedKey: vi.fn(),
  markUnsaved: vi.fn(),
  clearReferences: vi.fn(),
  resetExtraction: vi.fn(),
  refreshReferencePresentation: vi.fn(),
  submit: vi.fn(async () => undefined),
  resumeAfterAuthentication: vi.fn(async () => undefined),
  references: { value: [] as VisionReference[] },
  feedResults: null as Ref<SearchResult[]> | null,
  feedHasError: null as Ref<boolean> | null,
  state: { value: 'idle' as 'idle' | 'extracting' | 'ready' },
  error: { value: '' },
}))

vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace, push: mocks.push }),
}))

vi.mock('./feed', () => ({
  createSearchFeed: () => ({
    load: mocks.load,
    results: mocks.feedResults!,
    hasError: mocks.feedHasError,
    markSaved: mocks.markSaved,
    markSavedKey: mocks.markSavedKey,
    markUnsaved: mocks.markUnsaved,
  }),
}))

vi.mock('./useVisionQueryLifecycle', () => ({
  useVisionQueryLifecycle: () => ({
    references: mocks.references,
    state: mocks.state,
    error: ref(mocks.error.value),
    clearReferences: mocks.clearReferences,
    resetExtraction: mocks.resetExtraction,
    refreshReferencePresentation: mocks.refreshReferencePresentation,
    submit: mocks.submit,
    resumeAfterAuthentication: mocks.resumeAfterAuthentication,
  }),
}))

vi.mock('../../../core/composables/useAppToast')

describe('useSearchSession', () => {
  beforeEach(() => {
    mocks.route.query = {}
    mocks.references.value = []
    mocks.feedResults = ref([])
    mocks.feedHasError = ref(false)
    mocks.state.value = 'idle'
    clearPendingSearchAuthAction()
    vi.clearAllMocks()
    mocks.load.mockReset().mockResolvedValue(undefined)
    mockAppToast()
  })

  it('loads the route query into the feed', async () => {
    mocks.route.query = { q: '  warm cafe  ' }
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()

    expect(session.query.draftQuery.value).toBe('warm cafe')
    expect(session.query.submittedQuery.value).toBe('warm cafe')
    expect(mocks.load).toHaveBeenCalledWith(true)
  })

  it('routes a text submission and delegates reference submissions to Vision Search', async () => {
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()
    session.query.draftQuery.value = 'product display'

    await session.submitQuery()

    expect(mocks.resetExtraction).toHaveBeenCalledOnce()
    expect(mocks.replace).toHaveBeenCalledWith({
      path: '/user/search',
      query: { q: 'product display' },
    })

    mocks.references.value = [{ key: 'unsplash:1', previewUrl: '' }]
    await session.submitQuery()
    expect(mocks.submit).toHaveBeenCalledOnce()
  })

  test.each(['warm cafe', ''])(
    'retries a failed query (%j) on resubmit and loads a changed query once',
    async (query) => {
      const router = createRouter({
        history: createMemoryHistory(),
        routes: [{ path: '/user/search', component: { render: () => null } }],
      })
      await router.push({ path: '/user/search', query: query ? { q: query } : {} })
      const route = reactive({
        ...router.currentRoute.value,
        query: computed(() => router.currentRoute.value.query),
      })
      const scope = effectScope()
      mocks.load
        .mockImplementationOnce(async () => {
          mocks.feedHasError!.value = true
        })
        .mockImplementationOnce(async () => {
          mocks.feedHasError!.value = false
        })

      try {
        await scope.run(async () => {
          const session = useSearchSession({
            libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
            route,
            router,
          })
          await nextTick()
          expect(mocks.load).toHaveBeenCalledTimes(1)
          expect(session.feed.hasError.value).toBe(true)

          await session.submitQuery()
          await nextTick()

          expect(mocks.load).toHaveBeenCalledTimes(2)
          expect(session.feed.hasError.value).toBe(false)
          expect(session.query.submittedQuery.value).toBe(query)

          session.query.draftQuery.value = 'new search'
          await session.submitQuery()
          await nextTick()

          expect(router.currentRoute.value.query.q).toBe('new search')
          expect(session.query.submittedQuery.value).toBe('new search')
          expect(mocks.load).toHaveBeenCalledTimes(3)
        })
      } finally {
        scope.stop()
      }
    },
  )

  it('clears references and reloads the empty query when the route is already clean', async () => {
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()
    session.query.draftQuery.value = 'cafe'
    session.query.submittedQuery.value = 'cafe'
    vi.clearAllMocks()

    session.clearSearch()

    expect(mocks.clearReferences).toHaveBeenCalledOnce()
    expect(session.query.draftQuery.value).toBe('')
    expect(session.query.submittedQuery.value).toBe('')
    expect(mocks.load).toHaveBeenCalledWith(true)
  })

  it('owns batch selection and Library island commands', async () => {
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()
    const result = {
      provider: 'unsplash',
      id: 'photo-1',
      preview_url: 'https://images.example/photo-1.jpg',
      source_url: 'https://example.com/photo-1',
      original_url: 'https://images.example/photo-1-original.jpg',
      title: 'Warm light',
      author: 'IdeaBoard',
      author_url: 'https://example.com/author',
      thumbnail_url: 'https://images.example/photo-1-thumb.jpg',
      width: 1200,
      height: 800,
      media_kind: 'image',
      display: mediaDisplayFixture(),
    } as const

    session.batch.toggle(result)
    expect(session.batch.selected.value).toEqual([result])
    expect(session.batch.isSelected(result)).toBe(true)
    session.batch.clear()
    expect(session.batch.selected.value).toEqual([])

    session.library.toggle()
    expect(session.library.open.value).toBe(true)
    session.library.expanded.value = true
    session.library.close()
    expect(session.library.open.value).toBe(false)
    expect(session.library.expanded.value).toBe(true)
    await nextTick()
    expect(getLibraryIslandState().expanded).toBe(false)
  })

  it('leaves feed saved-state updates to Library publications after saving', async () => {
    const coordinator = createSearchLibraryCoordinator({ workspaceId: 'ws_test123' })
    const result: SearchResult = {
      provider: 'unsplash',
      id: 'photo-1',
      title: 'Warm light',
      author: '',
      author_url: '',
      source_url: 'https://example.com/photo-1',
      preview_url: 'https://example.com/photo-1.jpg',
      thumbnail_url: '',
      original_url: 'https://example.com/photo-1.jpg',
      width: 1200,
      height: 800,
      media_kind: 'image',
    }
    const asset = {
      source: 'external' as const,
      id: 'lasset_photo0001',
      workspaceId: 'ws_test123',
      provider: result.provider,
      sourceId: result.id,
      sourceUrl: result.source_url,
      previewUrl: result.preview_url,
      originalUrl: result.original_url,
      width: result.width,
      height: result.height,
      createdAt: '2026-10-16T00:00:00Z',
      updatedAt: '2026-10-16T00:00:00Z',
    }
    vi.spyOn(coordinator, 'saveReference').mockResolvedValue({ asset })
    vi.spyOn(coordinator, 'saveReferences').mockResolvedValue({
      operationId: 'op-1',
      status: 'succeeded',
      items: [
        {
          id: 'item-1',
          kind: 'external',
          title: result.title,
          status: 'saved',
          attempts: 1,
          asset,
        },
      ],
    })
    const session = useSearchSession({ libraryCoordinator: coordinator })
    await nextTick()

    expect(
      await session.library.saveReferenceToQuickSaves(session.toLibraryReference(result)),
    ).toBe(true)
    session.batch.toggle(result)
    await session.batch.saveAllToLibrary()

    expect(coordinator.saveReference).toHaveBeenCalledOnce()
    expect(coordinator.saveReferences).toHaveBeenCalledOnce()
    expect(session.batch.selected.value).toEqual([])
    expect(mocks.markSavedKey).not.toHaveBeenCalled()
    session.library.markAssetSaved('unsplash:photo-1')
    expect(mocks.markSavedKey).toHaveBeenCalledExactlyOnceWith('unsplash:photo-1')
    coordinator.dispose()
  })

  it('opens Library after a gated anonymous action resumes', async () => {
    setPendingSearchAuthAction({ kind: 'open-library' })
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
      isAuthenticated: () => true,
    })
    await nextTick()

    await session.resumePendingAction()

    expect(session.library.open.value).toBe(true)
  })

  it('restores cached Vision references for editing without auto-submitting a billed extraction', async () => {
    mocks.references.value = [{ key: 'unsplash:cached-reference', previewUrl: '' }]

    useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
      isAuthenticated: () => true,
    })
    await nextTick()

    expect(mocks.submit).not.toHaveBeenCalled()
  })

  it('toasts empty-result feed failures without an inline channel', async () => {
    const toast = mockAppToast()
    useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()

    mocks.feedResults!.value = []
    mocks.feedHasError!.value = true
    await nextTick()

    expect(toast.error).toHaveBeenCalledWith('Unable to load inspiration feed. Please try again.')
  })

  it('skips the failure toast while cached results stay visible', async () => {
    const toast = mockAppToast()
    useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    await nextTick()

    mocks.feedResults!.value = [{ id: 'photo-1' } as SearchResult]
    mocks.feedHasError!.value = true
    await nextTick()

    // 有数据时的刷新失败保留行内次级提示，不进 toast。
    expect(toast.error).not.toHaveBeenCalled()
  })
})

describe('Search session display refresh', () => {
  it('coordinates fresh results with Vision and refreshes the active lightbox', async () => {
    const result: SearchResult = {
      provider: 'unsplash',
      id: 'photo-refresh',
      title: 'Photo',
      author: 'Author',
      author_url: '',
      source_url: '',
      original_url: 'https://images.unsplash.com/original',
      preview_url: 'https://images.unsplash.com/preview',
      thumbnail_url: '',
      width: 100,
      height: 200,
      media_kind: 'image',
      display: mediaDisplayFixture('old'),
    }
    const externalReference = {
      provider: result.provider,
      id: result.id,
      preview_url: result.preview_url,
    }
    const reference = {
      key: 'unsplash:photo-refresh',
      externalReference,
      previewUrl: '/api/media/previews/old/thumb',
      previewExpiresAt: new Date(Date.now() - 1).toISOString(),
    }
    mocks.feedResults = ref([])
    mocks.references.value = [reference]
    const session = useSearchSession({
      libraryCoordinator: createSearchLibraryCoordinator({ workspaceId: 'ws_test123' }),
    })
    session.lightbox.open(result)
    mocks.feedResults.value = [{ ...result, display: mediaDisplayFixture('fresh') }]
    await nextTick()
    expect(mocks.refreshReferencePresentation).toHaveBeenCalledWith(mocks.feedResults.value)
    expect(session.lightbox.activeResult.value?.display).toEqual(mediaDisplayFixture('fresh'))
    expect(mocks.submit).not.toHaveBeenCalled()
  })
})
