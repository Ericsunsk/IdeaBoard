import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { useCanvasBoardRuntime } from './useCanvasBoardRuntime'

const mocks = vi.hoisted(() => ({
  useCanvasBoardSession: vi.fn(),
  useFlowBoardHeaderActions: vi.fn(),
  useFlowCanvasRuntime: vi.fn(),
}))

vi.mock('../board-session', () => ({
  useCanvasBoardSession: mocks.useCanvasBoardSession,
}))

vi.mock('./useFlowBoardHeaderActions', () => ({
  useFlowBoardHeaderActions: mocks.useFlowBoardHeaderActions,
}))

vi.mock('./useFlowCanvasRuntime', () => ({
  useFlowCanvasRuntime: mocks.useFlowCanvasRuntime,
}))

function createBoardRuntimeOptions() {
  return {
    route: { query: {} } as RouteLocationNormalizedLoaded,
    router: {
      push: vi.fn(async () => undefined),
    } as unknown as Router,
    getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
    logoutAndRedirect: vi.fn(async (_path: string) => undefined),
    feedback: {
      warning: vi.fn(),
      saveSucceeded: vi.fn(),
      saveFailed: vi.fn(),
    },
  }
}

function createFlowRuntime() {
  return {
    currentBoardId: computed(() => 'board-a'),
    loadInitialOrRouteBoard: vi.fn(),
    flushAutosave: vi.fn(async () => undefined),
    flowHistory: {},
    graphRuntime: {
      captureHistory: vi.fn(),
      markDirty: vi.fn(),
      nodeDrag: {
        begin: vi.fn(),
        commit: vi.fn(),
        cancel: vi.fn(),
      },
    },
    graphAdapter: {},
    onNodesChange: vi.fn(),
    onEdgesChange: vi.fn(),
  }
}

function createHeaderRuntime() {
  return {
    isBoardMenuOpen: ref(false),
    isRenamingBoard: ref(false),
    boardNameDraft: ref(''),
    isRenamePending: ref(false),
    catalogRecovery: ref({ status: 'idle' as const }),
    currentBoardName: computed(() => 'Board A'),
    boardOptions: computed(() => []),
    setBoardNameInputRef: vi.fn(),
    refreshBoardMetas: vi.fn(),
    toggleBoardMenu: vi.fn(),
    startBoardRename: vi.fn(),
    cancelBoardRename: vi.fn(),
    closeBoardHeaderEditing: vi.fn(),
    onConfirmBoardRename: vi.fn(),
    reloadCatalog: vi.fn(),
    onSelectBoard: vi.fn(),
  }
}

describe('useCanvasBoardRuntime', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0)
      return 1
    })
    mocks.useCanvasBoardSession.mockReturnValue({
      currentBoardId: computed(() => 'board-a'),
    })
    mocks.useFlowBoardHeaderActions.mockReturnValue(createHeaderRuntime())
    mocks.useFlowCanvasRuntime.mockReturnValue(createFlowRuntime())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('flushes autosave before navigation and logout requests leave the board', async () => {
    const options = createBoardRuntimeOptions()
    const events: string[] = []
    const flowRuntime = createFlowRuntime()
    flowRuntime.flushAutosave.mockImplementation(async () => {
      events.push('flush')
    })
    ;(options.router.push as ReturnType<typeof vi.fn>).mockImplementation(async (...args) => {
      const [path] = args as [string]
      events.push(`navigate:${path}`)
    })
    options.logoutAndRedirect.mockImplementation(async (path: string) => {
      events.push(`logout:${path}`)
    })
    mocks.useFlowCanvasRuntime.mockReturnValue(flowRuntime)

    const runtime = useCanvasBoardRuntime(options)

    await runtime.onNavigateRequest('/boards/next')
    await runtime.onLogoutRequest()

    expect(events).toEqual(['flush', 'navigate:/boards/next', 'flush', `logout:${USER_HOME_PATH}`])
    expect(flowRuntime.flushAutosave).toHaveBeenCalledTimes(2)
  })

  it('passes board session state into the Flow Pane runtime seam', () => {
    const flowRuntime = createFlowRuntime()
    mocks.useFlowCanvasRuntime.mockReturnValue(flowRuntime)
    const options = createBoardRuntimeOptions()

    useCanvasBoardRuntime(options)

    expect(mocks.useFlowCanvasRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        boardBaseSnapshot: expect.objectContaining({
          value: createEmptyCanvasBoardSnapshot(),
        }),
        onWarning: options.feedback.warning,
        onSuccess: options.feedback.saveSucceeded,
        onError: options.feedback.saveFailed,
        autosaveResumeSignal: expect.objectContaining({
          value: '1:user-a',
        }),
      }),
    )
  })

  it('keeps viewport persistence outside board dirty and autosave inputs', () => {
    const flowRuntime = createFlowRuntime()
    mocks.useFlowCanvasRuntime.mockReturnValue(flowRuntime)

    const runtime = useCanvasBoardRuntime(createBoardRuntimeOptions())
    const flowRuntimeOptions = mocks.useFlowCanvasRuntime.mock.calls[0]?.[0] as Record<
      string,
      unknown
    >

    expect(runtime).not.toHaveProperty('markBoardViewportChanged')
    expect(runtime).not.toHaveProperty('commitPendingBoardViewportChange')
    expect(flowRuntimeOptions).not.toHaveProperty('getViewport')
    expect(flowRuntimeOptions).not.toHaveProperty('setViewport')
    expect(flowRuntime.graphRuntime.markDirty).not.toHaveBeenCalled()
  })
})
