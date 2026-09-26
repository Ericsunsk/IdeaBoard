import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { useSearchScreenRuntime } from '../application/useSearchScreenRuntime'

const mocks = vi.hoisted(() => ({
  authOpen: vi.fn(),
  setPendingSearchAuthAction: vi.fn(),
  peekPendingSearchAuthAction: vi.fn(),
  libraryCoordinator: {
    reset: vi.fn(),
    dispose: vi.fn(),
    folderFacts: {},
    subscribe: vi.fn(
      (
        _listener: (outcome: {
          kind: string
          provider?: string
          sourceID?: string
          assetKey?: string
        }) => void,
      ) =>
        () => {},
    ),
  },
  nextSession: null as ReturnType<typeof createSession> | null,
  sessions: [] as ReturnType<typeof createSession>[],
  mutationListener: null as
    | ((outcome: { kind: string; provider?: string; sourceID?: string; assetKey?: string }) => void)
    | null,
}))

vi.mock('../../auth/overlay', () => ({
  useAuthOverlay: () => ({ commands: { open: mocks.authOpen } }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }),
}))

vi.mock('../infrastructure/authContinuation', () => ({
  setPendingSearchAuthAction: mocks.setPendingSearchAuthAction,
  peekPendingSearchAuthAction: mocks.peekPendingSearchAuthAction,
}))

vi.mock('../infrastructure/libraryCoordinator', () => ({
  createSearchLibraryCoordinator: () => mocks.libraryCoordinator,
  provideSearchLibraryCoordinator: vi.fn(),
}))

vi.mock('../application/useSearchSession', () => ({
  useSearchSession: (options: { initialVisionReferences?: { key: string }[] }) => {
    const session = mocks.nextSession ?? createSession()
    mocks.nextSession = null
    if (options.initialVisionReferences?.length) {
      session.vision.references.value = options.initialVisionReferences
    }
    mocks.sessions.push(session)
    return session
  },
}))

vi.mock('../../workspace', () => ({
  useWorkspaceSelection: () => ({ selectedId: ref('ws_test123') }),
}))

vi.mock('../../library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../library')>()
  return {
    ...actual,
    libraryAssetKey: (provider: string, id: string) => `${provider}:${id}`,
    readSearchReferenceTransfer: vi.fn(),
    writeSearchReferenceTransfer: vi.fn(),
  }
})

function createSession() {
  const references = ref<{ key: string }[]>([])
  const libraryOpen = ref(false)
  return {
    query: {
      draftQuery: ref(''),
      submittedQuery: ref(''),
      queryError: ref(''),
      heading: ref('Inspiration'),
    },
    feed: {
      savedKeys: ref(new Set<string>()),
    },
    vision: {
      references,
      state: ref<'idle' | 'extracting' | 'ready'>('idle'),
      buttonTitle: ref('Search'),
      transferReferences: vi.fn(() => {
        const current = references.value
        references.value = []
        return current
      }),
    },
    library: {
      open: libraryOpen,
      expanded: ref(false),
      markAssetSaved: vi.fn(),
      markAssetUnsaved: vi.fn(),
      saveReferenceToQuickSaves: vi.fn(async () => true),
      toggle: vi.fn(() => {
        libraryOpen.value = !libraryOpen.value
      }),
    },
    batch: {
      clear: vi.fn(),
    },
    lightbox: {
      visible: ref(false),
      activeResult: ref(null),
      open: vi.fn(),
      close: vi.fn(),
    },
    clearSearch: vi.fn(),
    submitQuery: vi.fn(async () => undefined),
    resumePendingAction: vi.fn(async () => undefined),
    toLibraryReference: vi.fn((result) => result),
    activate: vi.fn(),
    dispose: vi.fn(),
  }
}

function mountRuntime(isAuthenticated: boolean, references: { key: string }[] = []) {
  mocks.nextSession = createSession()
  mocks.nextSession.vision.references.value = references
  const userPublicID = ref(isAuthenticated ? 'user-1' : undefined)
  let runtime: ReturnType<typeof useSearchScreenRuntime> | null = null
  const app = createApp(
    defineComponent({
      setup() {
        runtime = useSearchScreenRuntime({
          userPublicID,
          isAuthenticated: () => Boolean(userPublicID.value),
          scrollContainer: ref(null),
        })
        return () => null
      },
    }),
  )
  const host = document.createElement('div')
  app.mount(host)
  return {
    runtime: runtime!,
    userPublicID,
    unmount: () => app.unmount(),
  }
}

describe('useSearchScreenRuntime', () => {
  beforeEach(() => {
    mocks.nextSession = null
    mocks.sessions = []
    mocks.authOpen.mockReset()
    mocks.setPendingSearchAuthAction.mockReset()
    mocks.libraryCoordinator.reset.mockReset()
    mocks.libraryCoordinator.dispose.mockReset()
    mocks.libraryCoordinator.subscribe.mockImplementation((listener) => {
      mocks.mutationListener = listener
      return () => {
        mocks.mutationListener = null
      }
    })
    mocks.mutationListener = null
  })

  it('owns the Auth gate for protected Library actions', async () => {
    const { runtime, unmount } = mountRuntime(false)

    runtime.commands.library.toggleForUser()
    await nextTick()

    expect(mocks.setPendingSearchAuthAction).toHaveBeenCalledWith({ kind: 'open-library' })
    expect(mocks.authOpen).toHaveBeenCalledWith('login')
    expect(mocks.sessions[0]?.library.toggle).not.toHaveBeenCalled()
    unmount()
  })

  it('disposes screen resources on unmount', () => {
    const { unmount } = mountRuntime(true, [{ key: 'unsplash:1' }])
    unmount()

    expect(mocks.libraryCoordinator.dispose).toHaveBeenCalledOnce()
  })

  it('replaces the anonymous session when a user signs in without carrying its facts forward', async () => {
    const { runtime, userPublicID, unmount } = mountRuntime(false, [{ key: 'local:anonymous' }])
    const anonymousSession = mocks.sessions[0]!

    userPublicID.value = 'user-a'
    await nextTick()

    expect(anonymousSession.dispose).toHaveBeenCalledOnce()
    expect(mocks.libraryCoordinator.reset).toHaveBeenCalledOnce()
    expect(runtime.view.search.references.value).toEqual([])
    unmount()
  })

  it('replaces user sessions on logout and account switches', async () => {
    const { runtime, userPublicID, unmount } = mountRuntime(true, [{ key: 'user-a:reference' }])
    const userASession = mocks.sessions[0]!

    userPublicID.value = undefined
    await nextTick()
    const anonymousSession = mocks.sessions[1]!
    userPublicID.value = 'user-b'
    await nextTick()

    expect(userASession.dispose).toHaveBeenCalledOnce()
    expect(anonymousSession.dispose).toHaveBeenCalledOnce()
    expect(runtime.view.search.references.value).toEqual([])
    unmount()
  })

  it('transfers Vision references only across the pending anonymous login continuation', async () => {
    mocks.peekPendingSearchAuthAction.mockReturnValue({ kind: 'vision-search', query: 'lamp' })
    const references = [{ key: 'local:vision-reference' }]
    const { userPublicID, unmount } = mountRuntime(false, references)
    const anonymousSession = mocks.sessions[0]!

    userPublicID.value = 'user-a'
    await nextTick()

    const userSession = mocks.sessions[1]!
    expect(anonymousSession.vision.transferReferences).toHaveBeenCalledOnce()
    expect(userSession.vision.references.value).toEqual(references)
    expect(userSession.resumePendingAction).toHaveBeenCalledOnce()
    unmount()
  })

  it('marks an Island-removed asset unsaved through the coordination outcome', () => {
    const { unmount } = mountRuntime(true)
    const session = mocks.sessions[0]!

    mocks.mutationListener?.({
      kind: 'asset-removed',
      provider: 'unsplash',
      sourceID: 'removed-image',
      assetKey: 'unsplash:removed-image',
    })

    expect(session.library.markAssetUnsaved).toHaveBeenCalledWith('unsplash:removed-image')
    unmount()
  })
})
