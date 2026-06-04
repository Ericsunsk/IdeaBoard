import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createEmptyCanvasBoardSnapshot } from '../utils/flowSnapshotAdapter'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'

const mocks = vi.hoisted(() => ({
  loadBoardSnapshotForFlow: vi.fn(),
  saveBoardSnapshotFromFlow: vi.fn(),
  syncCanvasBoardToServer: vi.fn(),
  resolveSave: undefined as undefined | (() => void),
}))

vi.mock('../services/flowBoardSnapshotLoader', () => ({
  resolveBoardIdFromQueryValue: (raw: string | string[] | null | undefined) => {
    const value = Array.isArray(raw) ? raw[0] : raw
    return typeof value === 'string' && value.trim() ? value.trim() : null
  },
  loadBoardSnapshotForFlow: mocks.loadBoardSnapshotForFlow,
}))

vi.mock('../services/flowBoardSnapshotSaver', () => ({
  saveBoardSnapshotFromFlow: mocks.saveBoardSnapshotFromFlow,
}))

vi.mock('../services/flowWorkspaceUploadLifecycle', () => ({
  createFlowWorkspaceUploadLifecycle: vi.fn(() => ({
    readPreviousSnapshot: vi.fn(() => null),
    stabilizeSnapshotForSave: vi.fn(async (snapshot) => snapshot),
    recycleRemovedUploadsAfterSave: vi.fn(async () => undefined),
  })),
}))

vi.mock('../../canvas/lib/boardPreviewSerializer', () => ({
  renderBoardPreviewDataUrl: vi.fn(async () => ''),
}))

vi.mock('../../boards/lib/boardSync', () => ({
  syncCanvasBoardToServer: mocks.syncCanvasBoardToServer,
}))

vi.mock('../../boards/lib/canvasBoards', () => ({
  getCanvasBoardById: vi.fn((boardId: string) => ({
    id: boardId,
    name: 'Board',
    createdAt: 1,
    updatedAt: 1,
  })),
}))

function createPersistenceHarness(initialBoardId = 'board-a') {
  const routeBoardId = ref(initialBoardId)
  const nodes = ref<unknown[]>([])
  const edges = ref<unknown[]>([])
  const boardBaseSnapshot = ref(createEmptyCanvasBoardSnapshot())
  const isDirty = ref(true)
  const saveStatus = ref<'idle' | 'saved' | 'failed'>('idle')
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const setViewport = vi.fn()
  const commitNodes = vi.fn((nextNodes: unknown[]) => {
    nodes.value = nextNodes
  })
  const commitEdges = vi.fn((nextEdges: unknown[]) => {
    edges.value = nextEdges
  })

  const persistence = useFlowBoardPersistence({
    nodes,
    edges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    getBoardIdQuery: () => routeBoardId.value,
    getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
    setViewport,
    onWarning: vi.fn(),
    onSuccess: vi.fn(),
    onError: vi.fn(),
    refreshSnapshot: vi.fn(),
    clearFlowHistoryBaseline: vi.fn(),
    getUserIdentity: () => ({ userId: 1, userUid: 'ABC123' }),
    commitNodes,
    commitEdges,
  })

  return {
    persistence,
    routeBoardId,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    commitNodes,
    commitEdges,
    setViewport,
  }
}

async function waitForPendingSave() {
  for (let index = 0; index < 10; index += 1) {
    if (mocks.resolveSave) return
    await Promise.resolve()
  }
  throw new Error('save did not start')
}

describe('useFlowBoardPersistence', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.resolveSave = undefined
    mocks.loadBoardSnapshotForFlow.mockResolvedValue(createEmptyCanvasBoardSnapshot())
    mocks.saveBoardSnapshotFromFlow.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          mocks.resolveSave = resolve
        }),
    )
    mocks.syncCanvasBoardToServer.mockResolvedValue(undefined)
  })

  it('does not commit graph or status when the current board changes during save', async () => {
    const harness = createPersistenceHarness()

    const savePromise = harness.persistence.saveCurrentBoardSnapshot()
    await waitForPendingSave()
    harness.routeBoardId.value = 'board-b'
    mocks.resolveSave?.()
    await savePromise

    expect(harness.commitNodes).not.toHaveBeenCalled()
    expect(harness.commitEdges).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('commits graph and saved status when the board remains current', async () => {
    const harness = createPersistenceHarness()

    const savePromise = harness.persistence.saveCurrentBoardSnapshot()
    await waitForPendingSave()
    mocks.resolveSave?.()
    await savePromise

    expect(harness.commitNodes).toHaveBeenCalledOnce()
    expect(harness.commitEdges).toHaveBeenCalledOnce()
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
  })

  it('ignores stale board snapshot loads when the current board changes', async () => {
    const harness = createPersistenceHarness('board-a')
    const resolvers = new Map<
      string,
      (snapshot: ReturnType<typeof createEmptyCanvasBoardSnapshot>) => void
    >()
    mocks.loadBoardSnapshotForFlow.mockImplementation(
      (boardId: string) =>
        new Promise<ReturnType<typeof createEmptyCanvasBoardSnapshot>>((resolve) => {
          resolvers.set(boardId, resolve)
        }),
    )

    const firstLoad = harness.persistence.loadInitialOrRouteBoard()
    harness.routeBoardId.value = 'board-b'
    const secondLoad = harness.persistence.loadInitialOrRouteBoard()

    const boardB = createEmptyCanvasBoardSnapshot()
    boardB.viewport = { canvasX: 20, canvasY: 20, zoomLevel: 222 }
    resolvers.get('board-b')?.(boardB)
    await secondLoad

    const boardA = createEmptyCanvasBoardSnapshot()
    boardA.viewport = { canvasX: 10, canvasY: 10, zoomLevel: 111 }
    resolvers.get('board-a')?.(boardA)
    await firstLoad
    await Promise.resolve()

    expect(harness.boardBaseSnapshot.value.viewport?.zoomLevel).toBe(222)
    expect(harness.setViewport).toHaveBeenLastCalledWith({ x: 20, y: 20, zoom: 2.22 })
    expect(harness.isLoadingBoardSnapshot.value).toBe(false)
  })
})
