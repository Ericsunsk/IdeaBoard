import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useSearchSession } from './useSearchSession'
import { setPendingSearchAuthAction } from './searchAuthContinuation'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string | string[]> },
  replace: vi.fn(async () => undefined),
  load: vi.fn(async () => undefined),
  markSaved: vi.fn(),
  markSavedKey: vi.fn(),
  markUnsaved: vi.fn(),
  clearAllReferences: vi.fn(),
  clearVisionExtraction: vi.fn(),
  runVisionSearch: vi.fn(async () => undefined),
  references: { value: [] as { key: string }[] },
  visionState: { value: 'idle' as 'idle' | 'extracting' | 'ready' },
  visionError: { value: '' },
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('./useSearchResults', () => ({
  useSearchResults: () => ({
    load: mocks.load,
    results: { value: [] },
    markSaved: mocks.markSaved,
    markSavedKey: mocks.markSavedKey,
    markUnsaved: mocks.markUnsaved,
  }),
}))

vi.mock('./useVisionSearchFlow', () => ({
  useVisionSearchFlow: () => ({
    references: mocks.references,
    visionState: mocks.visionState,
    visionError: mocks.visionError,
    clearAllReferences: mocks.clearAllReferences,
    clearVisionExtraction: mocks.clearVisionExtraction,
    runVisionSearch: mocks.runVisionSearch,
  }),
}))

describe('useSearchSession', () => {
  beforeEach(() => {
    mocks.route.query = {}
    mocks.references.value = []
    mocks.visionState.value = 'idle'
    vi.clearAllMocks()
  })

  it('loads the route query into the feed', async () => {
    mocks.route.query = { q: '  warm cafe  ' }
    const session = useSearchSession()
    await nextTick()

    expect(session.query.draftQuery.value).toBe('warm cafe')
    expect(session.query.submittedQuery.value).toBe('warm cafe')
    expect(mocks.load).toHaveBeenCalledWith(true)
  })

  it('routes a text submission and delegates reference submissions to Vision Search', async () => {
    const session = useSearchSession()
    await nextTick()
    session.query.draftQuery.value = 'product display'

    await session.submitQuery()

    expect(mocks.clearVisionExtraction).toHaveBeenCalledOnce()
    expect(mocks.replace).toHaveBeenCalledWith({
      path: '/user/search',
      query: { q: 'product display' },
    })

    mocks.references.value = [{ key: 'unsplash:1' }]
    await session.submitQuery()
    expect(mocks.runVisionSearch).toHaveBeenCalledOnce()
  })

  it('clears references and reloads the empty query when the route is already clean', async () => {
    const session = useSearchSession()
    await nextTick()
    session.query.draftQuery.value = 'cafe'
    session.query.submittedQuery.value = 'cafe'
    vi.clearAllMocks()

    session.clearSearch()

    expect(mocks.clearAllReferences).toHaveBeenCalledOnce()
    expect(session.query.draftQuery.value).toBe('')
    expect(session.query.submittedQuery.value).toBe('')
    expect(mocks.load).toHaveBeenCalledWith(true)
  })

  it('owns batch selection and Library island commands', async () => {
    const session = useSearchSession()
    await nextTick()
    const result = {
      provider: 'unsplash',
      id: 'photo-1',
      preview_url: 'https://images.example/photo-1.jpg',
      source_url: 'https://example.com/photo-1',
      original_url: 'https://images.example/photo-1-original.jpg',
      title: 'Warm light',
      author: 'IdeaBoard',
      width: 1200,
      height: 800,
      media_kind: 'image',
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
    const session = useSearchSession({ isAuthenticated: () => true })
    await nextTick()

    await session.resumePendingAction()

    expect(session.library.open.value).toBe(true)
  })
})
