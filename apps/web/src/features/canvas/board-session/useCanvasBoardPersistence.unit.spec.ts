import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type PromptNodeItem,
} from '../../canvas-board/domain'
import type { FlowAdapterEdge, FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import { useCanvasBoardPersistence } from './useCanvasBoardPersistence'

const mocks = vi.hoisted(() => ({
  load: vi.fn(),
  save: vi.fn(),
  resolveSave: undefined as undefined | (() => void),
}))

function createPersistenceHarness(initialBoardId = 'board-a') {
  const routeBoardId = ref(initialBoardId)
  const fullNodes = ref<unknown[]>([])
  const fullEdges = ref<unknown[]>([])
  const boardBaseSnapshot = ref(createEmptyCanvasBoardSnapshot())
  const isDirty = ref(true)
  const saveStatus = ref<'idle' | 'saved' | 'failed'>('idle')
  const isLoadingBoardSnapshot = ref(false)
  const isSavingBoard = ref(false)
  const commitGraph = vi.fn((nextGraph: { nodes: unknown[]; edges: unknown[] }) => {
    fullNodes.value = nextGraph.nodes
    fullEdges.value = nextGraph.edges
  })
  const onWarning = vi.fn()
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const clearFlowHistoryBaseline = vi.fn()
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

  const persistence = useCanvasBoardPersistence({
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
    commitGraph,
  })

  return {
    persistence,
    routeBoardId,
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    commitGraph,
    boardSession,
    onWarning,
    onSuccess,
    onError,
    clearFlowHistoryBaseline,
  }
}

function makePromptFlowNode(flowId: string, id: number, x: number): FlowAdapterNode {
  const snapshotNode = makePromptSnapshot(id, x)
  return {
    id: flowId,
    type: 'canvas',
    position: { x, y: 40 },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      title: `Prompt #${id}`,
      summary: 'Canvas prompt node',
      snapshotNode,
    },
  }
}

function makePromptSnapshot(id: number, x: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x,
    y: 40,
    width: 280,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function makeImageFlowNode(flowId: string, id: number, x: number): FlowAdapterNode {
  return {
    id: flowId,
    type: 'canvas',
    position: { x, y: 40 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: `Image #${id}`,
      summary: 'Canvas image node',
      snapshotNode: {
        id,
        type: 'image-node',
        x,
        y: 40,
        width: 280,
        sourceImageUrl: `https://example.com/source-${id}.png`,
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [{ url: `https://example.com/output-${id}.png`, ratio: 1 }],
      },
    },
  }
}

async function waitForPendingSave() {
  for (let index = 0; index < 10; index += 1) {
    if (mocks.resolveSave) return
    await Promise.resolve()
  }
  throw new Error('save did not start')
}

describe('useCanvasBoardPersistence', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.resolveSave = undefined
    mocks.load.mockResolvedValue(createEmptyCanvasBoardSnapshot())
    mocks.save.mockImplementation((snapshot: CanvasBoardSnapshot) =>
      new Promise<void>((resolve) => {
        mocks.resolveSave = resolve
      }).then(() => ({ snapshot, remoteSync: { ok: true } })),
    )
  })

  it('does not commit graph or status when the current board changes during save', async () => {
    const harness = createPersistenceHarness()

    const savePromise = harness.persistence.saveCurrentBoardSnapshot()
    await waitForPendingSave()
    harness.routeBoardId.value = 'board-b'
    mocks.resolveSave?.()
    await savePromise

    expect(harness.commitGraph).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('idle')
  })

  it('commits graph and saved status when the board remains current', async () => {
    const harness = createPersistenceHarness()

    const savePromise = harness.persistence.saveCurrentBoardSnapshot()
    await waitForPendingSave()
    mocks.resolveSave?.()
    await savePromise

    expect(harness.commitGraph).toHaveBeenCalledOnce()
    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { preserveSelection: true },
    )
    expect(harness.isDirty.value).toBe(false)
    expect(harness.saveStatus.value).toBe('saved')
  })

  it('keeps the board dirty and exposes retry state when remote sync fails after local save', async () => {
    const harness = createPersistenceHarness()
    const remoteError = new Error('network down')
    mocks.save.mockResolvedValueOnce({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: { ok: false, error: remoteError },
    })

    const saved = await harness.persistence.saveCurrentBoardSnapshot({ silent: false })

    expect(saved).toBe(false)
    expect(harness.commitGraph).toHaveBeenCalledOnce()
    expect(harness.clearFlowHistoryBaseline).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('failed')
    expect(harness.onWarning).toHaveBeenCalledWith('已保存到本地，远端同步失败，将在网络恢复后重试')
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onError).not.toHaveBeenCalled()
  })

  it('surfaces board name conflicts without retry copy when remote sync fails after local save', async () => {
    const harness = createPersistenceHarness()
    mocks.save.mockResolvedValueOnce({
      snapshot: createEmptyCanvasBoardSnapshot(),
      remoteSync: { ok: false, error: { status: 409, msg: '画布名称已存在' } },
    })

    const saved = await harness.persistence.saveCurrentBoardSnapshot({ silent: false })

    expect(saved).toBe(false)
    expect(harness.commitGraph).toHaveBeenCalledOnce()
    expect(harness.clearFlowHistoryBaseline).not.toHaveBeenCalled()
    expect(harness.isDirty.value).toBe(true)
    expect(harness.saveStatus.value).toBe('failed')
    expect(harness.onError).toHaveBeenCalledWith('已保存到本地，但画布名称已存在，请更换后再试')
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('saves from the full graph without dropping hidden nodes or edges', async () => {
    const harness = createPersistenceHarness()
    const firstNode = makePromptFlowNode('prompt-1', 1, 40)
    const hiddenImageNode = makeImageFlowNode('image-2', 2, 700)
    const hiddenPromptNode = makePromptFlowNode('prompt-2', 2, 900)
    const hiddenEdge: FlowAdapterEdge = {
      id: 'connection-9',
      source: 'image-2',
      sourceHandle: 'out:image',
      target: 'prompt-2',
      targetHandle: 'in:image',
      data: {
        connectionSnapshot: {
          id: 9,
          fromNodeType: 'image',
          fromNodeId: 2,
          fromPort: 'image',
          toNodeType: 'prompt',
          toNodeId: 2,
          toPort: 'image',
        },
      },
    }
    harness.fullNodes.value = [firstNode, hiddenImageNode, hiddenPromptNode]
    harness.fullEdges.value = [hiddenEdge]

    const savePromise = harness.persistence.saveCurrentBoardSnapshot()
    await waitForPendingSave()
    mocks.resolveSave?.()
    await savePromise

    const savedSnapshot = mocks.save.mock.calls[0]?.[0] as CanvasBoardSnapshot
    expect(savedSnapshot.imageNodes.map((node) => node.id)).toEqual([2])
    expect(savedSnapshot.promptNodes.map((node) => node.id)).toEqual([1, 2])
    expect(savedSnapshot.connections.map((connection) => connection.id)).toEqual([9])
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
    boardB.promptNodes = [makePromptSnapshot(2, 20)]
    resolvers.get('board-b')?.(boardB)
    await secondLoad

    const boardA = createEmptyCanvasBoardSnapshot()
    boardA.promptNodes = [makePromptSnapshot(1, 10)]
    resolvers.get('board-a')?.(boardA)
    await firstLoad
    await Promise.resolve()

    expect(harness.boardBaseSnapshot.value.promptNodes[0]?.id).toBe(2)
    expect(harness.isLoadingBoardSnapshot.value).toBe(false)
  })

  it('treats loaded board snapshots as graph replacements without carrying selection', async () => {
    const harness = createPersistenceHarness('board-a')
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.promptNodes = [
      {
        id: 1,
        type: 'prompt',
        x: 40,
        y: 40,
        width: 280,
        text: 'loaded',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    mocks.load.mockResolvedValue(snapshot)

    await harness.persistence.loadInitialOrRouteBoard()

    expect(harness.commitGraph).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: expect.any(Array),
        edges: expect.any(Array),
      }),
      { preserveSelection: false },
    )
  })
})
