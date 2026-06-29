import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { CanvasBoardSession } from '../board-session'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'

const mocks = vi.hoisted(() => ({
  load: vi.fn(),
  save: vi.fn(),
  resolveSave: undefined as undefined | (() => void),
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
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => {
      const boardId = routeBoardId.value.trim()
      return boardId || null
    }),
    exists: vi.fn((boardId?: string | null) => !!boardId),
    load: mocks.load,
    save: mocks.save,
    refreshPreview: vi.fn(),
  }

  const persistence = useFlowBoardPersistence({
    nodes,
    edges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
    setViewport,
    onWarning: vi.fn(),
    onSuccess: vi.fn(),
    onError: vi.fn(),
    refreshSnapshot: vi.fn(),
    clearFlowHistoryBaseline: vi.fn(),
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
    boardSession,
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
    mocks.load.mockResolvedValue(createEmptyCanvasBoardSnapshot())
    mocks.save.mockImplementation(
      (snapshot: CanvasBoardSnapshot) =>
        new Promise<void>((resolve) => {
          mocks.resolveSave = resolve
        }).then(() => ({ snapshot })),
    )
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
    mocks.load.mockImplementation(
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
