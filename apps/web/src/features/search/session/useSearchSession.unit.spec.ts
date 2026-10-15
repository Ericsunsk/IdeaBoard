import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref, type Ref } from 'vue'
import type { SearchResult, VisionReference } from '../core/models'
import { useSearchSession } from './useSearchSession'
import {
  clearPendingSearchAuthAction,
  setPendingSearchAuthAction,
} from './adapters/authContinuation'
import { createSearchLibraryCoordinator } from './adapters/libraryCoordinator'

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
  state: { value: 'idle' as 'idle' | 'extracting' | 'ready' },
  error: { value: '' },
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace, push: mocks.push }),
}))

vi.mock('./feed', () => ({
  createSearchFeed: () => ({
    load: mocks.load,
    results: mocks.feedResults!,
    markSaved: mocks.markSaved,
    markSavedKey: mocks.markSavedKey,
    markUnsaved: mocks.markUnsaved,
  }),
}))

vi.mock('./useVisionQueryLifecycle', () => ({
  useVisionQueryLifecycle: () => ({
    references: mocks.references,
    state: mocks.state,
    error: mocks.error,
    clearReferences: mocks.clearReferences,
    resetExtraction: mocks.resetExtraction,
    refreshReferencePresentation: mocks.refreshReferencePresentation,
    submit: mocks.submit,
    resumeAfterAuthentication: mocks.resumeAfterAuthentication,
  }),
}))

describe('useSearchSession', () => {
  beforeEach(() => {
    mocks.route.query = {}
    mocks.references.value = []
    mocks.feedResults = ref([])
    mocks.state.value = 'idle'
    clearPendingSearchAuthAction()
    vi.clearAllMocks()
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
    session.library.close()
    expect(session.library.open.value).toBe(false)
    expect(session.library.expanded.value).toBe(false)
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
