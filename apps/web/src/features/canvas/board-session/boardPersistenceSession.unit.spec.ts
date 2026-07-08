import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import type { FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import { createBoardPersistenceSession } from './boardPersistenceSession'

function makePromptSnapshot(id = 1) {
  return {
    id,
    type: 'prompt' as const,
    x: 40,
    y: 40,
    width: 280,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto' as const,
    inputImages: [],
  }
}

function makePendingImageFlowNode(): FlowAdapterNode {
  const snapshotNode: ImageNodeItem = {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'running',
    outputImages: [],
  }
  return {
    id: 'image-1',
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode,
    },
  }
}

function createSessionHarness(initialBoardId = 'board-a') {
  const routeBoardId = ref(initialBoardId)
  const fullNodes = ref<unknown[]>([])
  const fullEdges = ref<unknown[]>([])
  const boardBaseSnapshot = ref(createEmptyCanvasBoardSnapshot())
  const isDirty = ref(true)
  const saveStatus = ref<'idle' | 'saved' | 'failed'>('idle')
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const canSaveBoard = ref(true)
  const commitGraph = vi.fn((nextGraph: { nodes: unknown[]; edges: unknown[] }) => {
    fullNodes.value = nextGraph.nodes
    fullEdges.value = nextGraph.edges
  })
  const onWarning = vi.fn()
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const clearFlowHistoryBaseline = vi.fn()
  const onGraphCommitted = vi.fn()
  const boardSession: CanvasBoardSession = {
    currentBoardId: computed(() => {
      const boardId = routeBoardId.value.trim()
      return boardId || null
    }),
    exists: vi.fn((boardId?: string | null) => !!boardId),
    load: vi.fn(async () => createEmptyCanvasBoardSnapshot()),
    save: vi.fn(async (snapshot: CanvasBoardSnapshot) => ({
      snapshot,
      remoteSync: { ok: true as const },
    })),
    refreshPreview: vi.fn(),
  }
  const session = createBoardPersistenceSession({
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    canSaveBoard: () => canSaveBoard.value,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
    commitGraph,
    onGraphCommitted,
  })

  return {
    session,
    routeBoardId,
    fullNodes,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    canSaveBoard,
    commitGraph,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
    onGraphCommitted,
    boardSession,
  }
}

describe('boardPersistenceSession', () => {
  it('loads a board snapshot, applies the graph, and clears dirty state', async () => {
    const harness = createSessionHarness()
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.promptNodes = [makePromptSnapshot()]
    vi.mocked(harness.boardSession.load).mockResolvedValueOnce(snapshot)

    await harness.session.loadInitialOrRouteBoard()
    await Promise.resolve()

    expect(harness.boardSession.load).toHaveBeenCalledWith('board-a')
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { preserveSelection: false },
    )
    expect(harness.boardBaseSnapshot.value).toMatchObject({
      promptNodes: snapshot.promptNodes,
    })
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
    expect(harness.clearFlowHistoryBaseline).toHaveBeenCalledOnce()
    expect(harness.onGraphCommitted).toHaveBeenCalledOnce()
    expect(harness.isLoadingBoardSnapshot.value).toBe(false)
  })

  it('loads an empty snapshot without calling the repository when no board is selected', async () => {
    const harness = createSessionHarness('')
    harness.canSaveBoard.value = false

    await harness.session.loadInitialOrRouteBoard()

    expect(harness.boardSession.load).not.toHaveBeenCalled()
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { preserveSelection: false },
    )
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('does not replace the graph baseline when saving while unresolved pending result nodes exist', async () => {
    const harness = createSessionHarness()
    harness.fullNodes.value = [makePendingImageFlowNode()]

    const saved = await harness.session.saveCurrentBoardSnapshot()

    expect(saved).toBe(true)
    expect(harness.boardSession.save).toHaveBeenCalledOnce()
    expect(harness.commitGraph).not.toHaveBeenCalled()
    expect(harness.clearFlowHistoryBaseline).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
    expect(harness.onSuccess).toHaveBeenCalledWith('已保存')
    expect(harness.onGraphCommitted).toHaveBeenCalledOnce()
    expect(harness.isSavingBoard.value).toBe(false)
  })

  it('keeps the board dirty and failed when remote sync fails after local save', async () => {
    const harness = createSessionHarness()
    const savedSnapshot = createEmptyCanvasBoardSnapshot()
    savedSnapshot.promptNodes = [makePromptSnapshot(2)]
    vi.mocked(harness.boardSession.save).mockResolvedValueOnce({
      snapshot: savedSnapshot,
      remoteSync: { ok: false, error: new Error('auth unavailable') },
    })

    const saved = await harness.session.saveCurrentBoardSnapshot()

    expect(saved).toBe(false)
    expect(harness.boardBaseSnapshot.value).toEqual(savedSnapshot)
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('failed')
    expect(harness.clearFlowHistoryBaseline).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledWith('已保存到本地，远端同步失败，将在网络恢复后重试')
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onGraphCommitted).toHaveBeenCalledOnce()
    expect(harness.isSavingBoard.value).toBe(false)
  })
})
