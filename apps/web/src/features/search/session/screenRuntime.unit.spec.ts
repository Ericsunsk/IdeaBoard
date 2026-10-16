import { mediaDisplayFixture } from '../../../test/mediaDisplay'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { mockAppToast } from '../../../test/toast'
import { screenRuntime } from './screenRuntime'
import type { SearchResult } from '../core/models'

const mocks = vi.hoisted(() => ({
  authOpen: vi.fn(),
  workspaceId: null as ReturnType<typeof ref<string>> | null,
  setPendingSearchAuthAction: vi.fn(),
  peekPendingSearchAuthAction: vi.fn(),
  libraryCoordinator: {
    reset: vi.fn(),
    dispose: vi.fn(),
    folderFacts: {},
    subscribe: vi.fn(
      (
        _listener: (
          outcomes: Array<{
            kind: string
            provider?: string
            sourceID?: string
            assetKey?: string
          }>,
        ) => void,
      ) =>
        () => {},
    ),
  },
  nextSession: null as ReturnType<typeof createSession> | null,
  sessions: [] as ReturnType<typeof createSession>[],
  mutationListener: null as
    | ((
        outcomes: Array<{ kind: string; provider?: string; sourceID?: string; assetKey?: string }>,
      ) => void)
    | null,
}))

vi.mock('../../auth/overlay', () => ({
  useAuthOverlay: () => ({ commands: { open: mocks.authOpen } }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

vi.mock('../../../core/composables/useAppToast')

vi.mock('./adapters/authContinuation', () => ({
  setPendingSearchAuthAction: mocks.setPendingSearchAuthAction,
  peekPendingSearchAuthAction: mocks.peekPendingSearchAuthAction,
}))

vi.mock('./adapters/libraryCoordinator', () => ({
  createSearchLibraryCoordinator: () => mocks.libraryCoordinator,
  provideSearchLibraryCoordinator: vi.fn(),
}))

vi.mock('./useSearchSession', () => ({
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
  useWorkspaceSelection: () => ({ selectedId: (mocks.workspaceId ??= ref('ws_test123')) }),
}))

vi.mock('../../library', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../library')>()
  return {
    ...actual,
    libraryAssetKey: (provider: string, id: string) => `${provider}:${id}`,
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
  let runtime: ReturnType<typeof screenRuntime> | null = null
  const app = createApp(
    defineComponent({
      setup() {
        runtime = screenRuntime({
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

function dragResult(): SearchResult {
  return {
    id: 'photo-1',
    provider: 'unsplash',
    title: 'Warm light',
    author: 'IdeaBoard',
    author_url: 'https://example.com/author',
    source_url: 'https://example.com/photo-1',
    thumbnail_url: 'https://images.example/photo-1-thumb.jpg',
    preview_url: 'https://images.example/photo-1-preview.jpg',
    original_url: 'https://images.example/photo-1-original.jpg',
    width: 1200,
    height: 800,
    media_kind: 'image',
    display: mediaDisplayFixture(),
  }
}

function createDropTarget(runtime: ReturnType<typeof screenRuntime>) {
  const trigger = document.createElement('button')
  document.body.appendChild(trigger)
  vi.spyOn(trigger, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 100, 100))
  vi.spyOn(document, 'elementFromPoint').mockReturnValue(trigger)
  runtime.commands.library.registerTrigger(trigger)
  return trigger
}

describe('screenRuntime', () => {
  beforeEach(() => {
    mockAppToast()
    mocks.nextSession = null
    mocks.sessions = []
    mocks.authOpen.mockReset()
    mocks.setPendingSearchAuthAction.mockReset()
    mocks.peekPendingSearchAuthAction.mockReset()
    mocks.libraryCoordinator.reset.mockReset()
    mocks.libraryCoordinator.dispose.mockReset()
    mocks.libraryCoordinator.subscribe.mockImplementation((listener) => {
      mocks.mutationListener = listener
      return () => {
        mocks.mutationListener = null
      }
    })
    mocks.mutationListener = null
    mocks.workspaceId = null
  })

  afterEach(() => vi.restoreAllMocks())

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

  it('updates saved state from Library publications', () => {
    const { unmount } = mountRuntime(true)
    const session = mocks.sessions[0]!

    mocks.mutationListener?.([
      {
        kind: 'asset-saved',
        assetKey: 'unsplash:saved-image',
      },
      {
        kind: 'asset-removed',
        provider: 'unsplash',
        sourceID: 'removed-image',
        assetKey: 'unsplash:removed-image',
      },
    ])

    expect(session.library.markAssetSaved).toHaveBeenCalledExactlyOnceWith('unsplash:saved-image')
    expect(session.library.markAssetUnsaved).toHaveBeenCalledExactlyOnceWith(
      'unsplash:removed-image',
    )
    unmount()
  })

  it('writes the Search reference payload and cleans up the drag ghost', () => {
    const { runtime, unmount } = mountRuntime(true)
    const hitTest = vi.spyOn(document, 'elementFromPoint').mockReturnValue(null)
    try {
      const result = dragResult()
      runtime.commands.library.handleCardPointerDown(
        new PointerEvent('pointerdown', {
          pointerId: 1,
          isPrimary: true,
          clientX: 10,
          clientY: 10,
        }),
        result,
      )
      window.dispatchEvent(
        new PointerEvent('pointermove', { pointerId: 1, clientX: 30, clientY: 30 }),
      )

      expect(mocks.sessions[0]?.toLibraryReference).toHaveBeenCalledWith(result)
      expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(true)
      expect(document.body.querySelector('img')).not.toBeNull()
    } finally {
      unmount()
      hitTest.mockRestore()
    }
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
    expect(document.body.querySelector('img')).toBeNull()
  })

  it('anchors the save popover to the event currentTarget', () => {
    const { runtime, unmount } = mountRuntime(true)
    const toggle = vi.fn()
    const hide = vi.fn()
    runtime.commands.library.registerSavePopover({ toggle, hide })
    const anchor = document.createElement('button')
    document.body.appendChild(anchor)
    try {
      const result = dragResult()
      runtime.commands.library.openSavePopover(
        { currentTarget: anchor } as unknown as Event,
        result,
      )

      expect(runtime.savePopoverAnchor.value).toBe(anchor)
      expect(toggle).toHaveBeenCalledOnce()
      const request = toggle.mock.calls[0]?.[1] as { result: unknown; isAuthenticated: boolean }
      expect(request.result).toBe(result)
      expect(request.isAuthenticated).toBe(true)
    } finally {
      anchor.remove()
      unmount()
    }
  })

  it('auto-opens the Library on hover and cancels on drag leave', () => {
    vi.useFakeTimers()
    try {
      const { runtime, unmount } = mountRuntime(true)
      const session = mocks.sessions[0]!
      expect(session.library.open.value).toBe(false)
      const trigger = createDropTarget(runtime)
      const reference = dragResult()
      runtime.dropTargets.update(10, 10, { kind: 'reference', reference })
      expect(runtime.view.library.isDropTarget.value).toBe(true)

      runtime.dropTargets.clear()
      expect(runtime.view.library.isDropTarget.value).toBe(false)
      vi.advanceTimersByTime(280)
      expect(session.library.open.value).toBe(false)

      runtime.dropTargets.update(10, 10, { kind: 'reference', reference })
      runtime.commands.library.registerTrigger(trigger)
      vi.advanceTimersByTime(280)
      expect(session.library.open.value).toBe(true)

      runtime.dropTargets.clear()
      expect(runtime.view.library.isDropTarget.value).toBe(false)
      unmount()
      trigger.remove()
    } finally {
      vi.useRealTimers()
    }
  })

  it('drops the reference directly into Quick Saves', () => {
    const { runtime, unmount } = mountRuntime(true)
    const session = mocks.sessions[0]!
    const reference = dragResult()
    const trigger = createDropTarget(runtime)
    runtime.dropTargets.drop({ kind: 'reference', reference }, 10, 10)

    expect(session.library.saveReferenceToQuickSaves).toHaveBeenCalledWith(reference)
    expect(runtime.view.library.isDropTarget.value).toBe(false)
    unmount()
    trigger.remove()
  })

  it('retargets the automatically opened Library without another pointer move', () => {
    vi.useFakeTimers()
    const { runtime, unmount } = mountRuntime(true)
    const trigger = createDropTarget(runtime)
    try {
      runtime.commands.library.handleCardPointerDown(
        new PointerEvent('pointerdown', { pointerId: 1, isPrimary: true }),
        dragResult(),
      )
      window.dispatchEvent(
        new PointerEvent('pointermove', { pointerId: 1, clientX: 10, clientY: 10 }),
      )
      vi.advanceTimersByTime(280)
      expect(mocks.sessions[0]?.library.open.value).toBe(true)

      const folder = document.createElement('div')
      trigger.appendChild(folder)
      vi.spyOn(folder, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 100, 100))
      vi.mocked(document.elementFromPoint).mockReturnValue(folder)
      const onHoverChange = vi.fn()
      const onDrop = vi.fn()
      runtime.dropTargets.register({
        element: folder,
        canReceive: () => true,
        onHoverChange,
        onDrop,
      })
      vi.advanceTimersToNextFrame()
      expect(onHoverChange).toHaveBeenCalledWith(true)
      expect(runtime.view.library.isDropTarget.value).toBe(false)

      window.dispatchEvent(
        new PointerEvent('pointerup', { pointerId: 1, clientX: 10, clientY: 10 }),
      )
      expect(onDrop).toHaveBeenCalledOnce()
      expect(mocks.sessions[0]?.library.saveReferenceToQuickSaves).not.toHaveBeenCalled()
    } finally {
      unmount()
      trigger.remove()
      vi.runAllTimers()
      vi.useRealTimers()
    }
  })

  it('retains the save intent and login prompt for an anonymous trigger drop', () => {
    const { runtime, unmount } = mountRuntime(false)
    const trigger = createDropTarget(runtime)
    const reference = dragResult()
    runtime.dropTargets.update(10, 10, { kind: 'reference', reference })
    expect(runtime.view.library.isDropTarget.value).toBe(false)
    runtime.dropTargets.drop({ kind: 'reference', reference }, 10, 10)

    expect(mocks.setPendingSearchAuthAction).toHaveBeenCalledWith({
      kind: 'save-reference',
      reference,
    })
    expect(mocks.authOpen).toHaveBeenCalledWith('login')
    expect(mocks.sessions[0]?.library.saveReferenceToQuickSaves).not.toHaveBeenCalled()
    unmount()
    trigger.remove()
  })

  it('cancels the active drag synchronously when the workspace changes', () => {
    const { runtime, unmount } = mountRuntime(true)
    const trigger = createDropTarget(runtime)
    runtime.commands.library.handleCardPointerDown(
      new PointerEvent('pointerdown', { pointerId: 1, isPrimary: true }),
      dragResult(),
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { pointerId: 1, clientX: 10, clientY: 10 }),
    )
    expect(runtime.view.library.isDropTarget.value).toBe(true)

    mocks.workspaceId!.value = 'ws_other123'
    window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 1, clientX: 10, clientY: 10 }))

    expect(runtime.view.library.isDropTarget.value).toBe(false)
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
    expect(mocks.sessions[0]?.library.saveReferenceToQuickSaves).not.toHaveBeenCalled()
    unmount()
    trigger.remove()
  })
})
