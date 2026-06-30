import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { CanvasBoardSession } from '../board-session'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { FlowAdapterEdge, FlowAdapterNode } from '../utils/flowSnapshotAdapter'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'

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
  const setViewport = vi.fn()
  const commitGraph = vi.fn((nextGraph: { nodes: unknown[]; edges: unknown[] }) => {
    fullNodes.value = nextGraph.nodes
    fullEdges.value = nextGraph.edges
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
    fullNodes,
    fullEdges,
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
    setViewport,
    boardSession,
  }
}

function makePromptFlowNode(flowId: string, id: number, x: number): FlowAdapterNode {
  return {
    id: flowId,
    type: 'canvas',
    position: { x, y: 40 },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      title: `Prompt #${id}`,
      summary: 'Canvas prompt node',
      snapshotNode: {
        id,
        type: 'prompt',
        x,
        y: 40,
        width: 280,
        text: `prompt ${id}`,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    },
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
