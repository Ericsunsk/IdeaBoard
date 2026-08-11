import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { useCanvasBoardRuntime } from './useCanvasBoardRuntime'
import { createCanvasExecutionRuntime } from '../execution/runtime/CanvasExecutionRuntime'

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

function createFlowRuntime(flushAutosave: () => Promise<void>) {
  return {
    persistence: {
      currentBoardId: computed(() => 'board-a'),
      loadInitialOrRouteBoard: vi.fn(),
      flushAutosave,
      history: {},
    },
    flowPane: {
      onNodesChange: vi.fn(),
      onEdgesChange: vi.fn(),
      nodeDragTransactions: {
        begin: vi.fn(),
        commit: vi.fn(),
      },
    },
    graphEditing: {},
  }
}

function createBoardHeaderRuntime() {
  return {
    isBoardMenuOpen: ref(false),
    isRenamingBoard: ref(false),
    boardNameDraft: ref(''),
    isRenamePending: ref(false),
    catalogRecovery: ref({ status: 'idle' as const }),
    currentBoardName: computed(() => 'Board A'),
    boardOptions: computed(() => []),
    setBoardNameInputRef: vi.fn(),
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
    mocks.useCanvasBoardSession.mockReturnValue({})
    mocks.useFlowBoardHeaderActions.mockReturnValue(createBoardHeaderRuntime())
  })

  it('flushes autosave before navigation and logout leave the board', async () => {
    const events: string[] = []
    const flushAutosave = vi.fn(async () => {
      events.push('flush')
    })
    mocks.useFlowCanvasRuntime.mockReturnValue(createFlowRuntime(flushAutosave))
    const router = {
      push: vi.fn(async (path: string) => {
        events.push(`navigate:${path}`)
      }),
    } as unknown as Router
    const logoutAndRedirect = vi.fn(async (path: string) => {
      events.push(`logout:${path}`)
    })
    const runtime = useCanvasBoardRuntime({
      route: { query: {} } as RouteLocationNormalizedLoaded,
      router,
      getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
      logoutAndRedirect,
      feedback: {
        warning: vi.fn(),
        saveSucceeded: vi.fn(),
        saveFailed: vi.fn(),
      },
      cancelTasks: vi.fn(),
      executionRuntime: createCanvasExecutionRuntime(),
    })

    await runtime.boardChrome.onNavigateRequest('/boards/next')
    await runtime.boardChrome.onLogoutRequest()

    expect(events).toEqual(['flush', 'navigate:/boards/next', 'flush', `logout:${USER_HOME_PATH}`])
    expect(flushAutosave).toHaveBeenCalledTimes(2)
  })

  it('keeps Board Session ownership explicit and viewport concerns outside Flow persistence', () => {
    const boardSession = { kind: 'board-session' }
    mocks.useCanvasBoardSession.mockReturnValue(boardSession)
    mocks.useFlowCanvasRuntime.mockReturnValue(createFlowRuntime(vi.fn(async () => {})))
    const feedback = {
      warning: vi.fn(),
      saveSucceeded: vi.fn(),
      saveFailed: vi.fn(),
    }

    const runtime = useCanvasBoardRuntime({
      route: { query: {} } as RouteLocationNormalizedLoaded,
      router: { push: vi.fn() } as unknown as Router,
      getUserIdentity: () => ({ userId: 7, userUid: 'user-seven' }),
      logoutAndRedirect: vi.fn(),
      feedback,
      cancelTasks: vi.fn(),
      executionRuntime: createCanvasExecutionRuntime(),
    })
    const flowOptions = mocks.useFlowCanvasRuntime.mock.calls[0]?.[0]

    expect(flowOptions).toEqual(
      expect.objectContaining({
        boardSession,
        boardBaseSnapshot: expect.objectContaining({ value: expect.any(Object) }),
        autosaveResumeSignal: expect.objectContaining({ value: '7:user-seven' }),
        onWarning: feedback.warning,
        onSuccess: feedback.saveSucceeded,
        onError: feedback.saveFailed,
      }),
    )
    expect(flowOptions).not.toHaveProperty('getViewport')
    expect(flowOptions).not.toHaveProperty('setViewport')
    expect(Object.keys(runtime).sort()).toEqual([
      'boardChrome',
      'boardSession',
      'flowPane',
      'graphEditing',
    ])
    expect(runtime).not.toHaveProperty('graphRuntime')
  })
})
