import { shallowRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../../canvas-board/domain'
import { readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import type {
  CanvasVueFlowGraphCommandOptions,
  CanvasVueFlowGraphNodeUpdateOptions,
} from '../../composables/useCanvasVueFlowGraphAdapter'
import { CanvasGraphStore } from '../../graph-runtime'
import {
  usePendingImageResultExecution,
  type PendingImageResultGraphAdapter,
} from './usePendingImageResultNodes'

function imageNode(id: number, x: number, sourceImageUrl = 'source'): Node {
  return {
    id: `image-${id}`,
    position: { x, y: 20 },
    data: {
      kind: 'image',
      snapshotNode: {
        id,
        type: 'image-node',
        x,
        y: 20,
        width: 280,
        sourceImageUrl,
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
    },
  } as Node
}

function createHarness(activeNodeIds: readonly string[] = []) {
  const nodes = shallowRef<Node[]>([imageNode(1, 10)])
  const edges = shallowRef<Edge[]>([])
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes.push(nodes.value[0]!.data.snapshotNode)
  const store = CanvasGraphStore.fromSnapshot(snapshot)
  let currentStore = store
  const attachment = { boardId: 'board-a', epoch: 1 } as never
  let isAttachmentCurrent = true
  const replaceGraph = vi.fn(
    (
      graph: { nodes: Node[]; edges: Edge[] },
      _commandOptions?: CanvasVueFlowGraphCommandOptions,
    ) => {
      nodes.value = graph.nodes
      edges.value = graph.edges
      return { didChange: true }
    },
  )
  const addEdges = vi.fn(
    (next: Edge | Edge[], _commandOptions?: CanvasVueFlowGraphCommandOptions) => {
      edges.value = [...edges.value, ...(Array.isArray(next) ? next : [next])]
      return { didChange: true }
    },
  )
  const updateNode = vi.fn(
    (
      nodeId: string,
      update: Partial<Node> | ((node: Node) => Partial<Node>),
      _commandOptions?: CanvasVueFlowGraphNodeUpdateOptions,
    ) => {
      nodes.value = nodes.value.map((node) => {
        if (node.id !== nodeId) return node
        const patch = typeof update === 'function' ? update(node) : update
        return { ...node, ...patch, data: patch.data ?? node.data }
      })
      return { didChange: true }
    },
  )
  const graphAdapter = {
    replaceGraph,
    addEdges,
    updateNode,
    removeNodes: vi.fn(),
  } satisfies PendingImageResultGraphAdapter
  const actions = usePendingImageResultExecution({
    nodes,
    edges,
    graphAdapter,
    readCanvasGraphStore: () => currentStore,
    readCurrentAttachment: () => attachment,
    isAttachmentCurrent: (candidate) => candidate === attachment && isAttachmentCurrent,
    isNodeExecutionActive: (nodeId) => activeNodeIds.includes(nodeId),
  })
  return {
    actions,
    addEdges,
    edges,
    nodes,
    replaceGraph,
    store,
    invalidateAttachment: () => {
      isAttachmentCurrent = false
    },
    replaceCurrentStore: (nextStore: CanvasGraphStore) => {
      currentStore = nextStore
    },
    updateNode,
  }
}

const source = { url: 'source', ratio: 1 }

describe('usePendingImageResultExecution reservation projection', () => {
  it('reserves a nearby derived image slot and canonical connection before the deferred projection', () => {
    const harness = createHarness()
    const reservation = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })

    expect(reservation.resultNodeIds).toHaveLength(1)
    expect(harness.nodes.value).toHaveLength(2)
    expect(harness.nodes.value[1]!.position.x).toBeGreaterThan(
      harness.nodes.value[0]!.position.x,
    )
    expect(harness.store.exportSnapshot().connections).toEqual([
      expect.objectContaining({
        id: 1,
        fromNodeId: 1,
        toNodeId: Number(reservation.resultNodeIds[0]?.split('-')[1]),
      }),
    ])
    const commandOptions = harness.replaceGraph.mock.calls[0]?.[1]
    expect(readFlowGraphCommandSideEffects(commandOptions?.metadata)).toMatchObject({
      recordHistory: false,
    })
  })

  it('does not project a deferred result edge after its attachment becomes stale', async () => {
    const frameCallbacks: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback)
      return frameCallbacks.length
    })
    try {
      const harness = createHarness()
      harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })
      await Promise.resolve()
      harness.invalidateAttachment()
      frameCallbacks.shift()?.(0)
      await Promise.resolve()

      expect(harness.store.exportSnapshot().connections).toHaveLength(1)
      expect(harness.addEdges).not.toHaveBeenCalled()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('reuses the first inactive empty image child before appending a slot', () => {
    const harness = createHarness()
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1', source: 'image-1', sourceHandle: 'out:image', target: 'image-2', targetHandle: 'in:image',
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    expect(reservation.resultNodeIds).toEqual(['image-2'])
    expect(harness.nodes.value).toHaveLength(2)
    expect(harness.nodes.value.find((node) => node.id === 'image-2')?.data.snapshotNode.outputStatus).toBe('running')
  })

  it('does not reuse an active empty child slot', () => {
    const harness = createHarness(['image-2'])
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1', source: 'image-1', sourceHandle: 'out:image', target: 'image-2', targetHandle: 'in:image',
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    expect(reservation.resultNodeIds[0]).not.toBe('image-2')
    expect(harness.nodes.value).toHaveLength(3)
  })

  it('places a new result below an occupied preferred column instead of colliding', () => {
    const harness = createHarness()
    harness.nodes.value.push({
      id: 'note-2',
      position: { x: 390, y: 20 },
      width: 280,
      height: 180,
      data: { kind: 'note', snapshotNode: { width: 280, height: 180 } },
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })
    const resultNode = harness.nodes.value.find((node) => node.id === reservation.resultNodeIds[0])

    expect(resultNode).toBeDefined()
    if (!resultNode) throw new Error('Expected reserved result node')
    expect(resultNode.position).toMatchObject({ x: 390 })
    expect(resultNode.position.y).toBeGreaterThan(20)
  })

  it('attaches persisted task identity without adding graph history', () => {
    const harness = createHarness()
    const { resultNodeIds } = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    harness.actions.attachPendingTaskIds([
      {
        resultNodeId: resultNodeIds[0]!,
        resultIndex: 0,
        taskId: 'task-1',
        clientActionId: 'canvas:action-1',
      },
    ])

    const node = harness.nodes.value.find((item) => item.id === resultNodeIds[0])
    expect(node).toBeDefined()
    if (!node) throw new Error('Expected pending result node')
    expect(node.data.snapshotNode).toMatchObject({
      pendingTaskId: 'task-1',
      pendingResultIndex: 0,
      pendingClientActionId: 'canvas:action-1',
    })
    const commandOptions = harness.updateNode.mock.calls[0]?.[2]
    expect(readFlowGraphCommandSideEffects(commandOptions?.metadata)).toMatchObject({
      recordHistory: false,
    })
  })
})
