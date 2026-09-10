import { shallowRef } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../board/domain'
import { CanvasGraphStore, CanvasGraphSession } from '../../graph-runtime'
import { bindCanvasGraphSessionProjection } from '../../vue-flow-adapter/bindCanvasGraphSessionProjection'
import { boardSnapshotToFlow } from '../../utils/flowSnapshotAdapter'
import { usePendingImageResultExecution } from './useResultNodes'

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
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes.push(nodes.value[0]!.data.snapshotNode)
  const session = CanvasGraphSession.hydrate('board-a', snapshot)
  const store = session.store
  let currentStore = store
  const attachment = { boardId: 'board-a', epoch: 1 } as never
  let isAttachmentCurrent = true
  bindCanvasGraphSessionProjection({
    session,
    graphAdapter: {
      applyIncrementalProjection: (projection) => {
        if (projection.nodes) nodes.value = projection.nodes
        if (projection.edges) edges.value = projection.edges
      },
      replaceFullProjection: (graph) => {
        nodes.value = graph.nodes
        edges.value = graph.edges
      },
    },
    readCurrent: () => ({ nodes: nodes.value, edges: edges.value }),
    projectFull: () => boardSnapshotToFlow(session.exportSnapshot(snapshot)),
    isCurrentAttachment: () => isAttachmentCurrent,
  })
  const actions = usePendingImageResultExecution({
    nodes,
    edges,
    readCanvasGraphStore: () => currentStore,
    readCurrentAttachment: () => attachment,
    isAttachmentCurrent: (candidate) => candidate === attachment && isAttachmentCurrent,
    isNodeExecutionActive: (nodeId) => activeNodeIds.includes(nodeId),
  })
  return {
    actions,
    edges,
    nodes,
    store,
    invalidateAttachment: () => {
      isAttachmentCurrent = false
    },
    replaceCurrentStore: (nextStore: CanvasGraphStore) => {
      currentStore = nextStore
    },
    syncGraph: () => {
      currentStore.hydrate(createSnapshotFromFlow(nodes.value, edges.value))
    },
  }
}

function createSnapshotFromFlow(nodes: readonly Node[], edges: readonly Edge[]) {
  const snapshot = createEmptyBoardSnapshot()
  nodes.forEach((node) => {
    const snapshotNode = node.data?.snapshotNode
    if (!snapshotNode) return
    if (snapshotNode.type === 'image-node') snapshot.imageNodes.push(snapshotNode)
    else if (snapshotNode.type === 'prompt') snapshot.promptNodes.push(snapshotNode)
    else snapshot.fusionNodes.push(snapshotNode)
  })
  edges.forEach((edge) => {
    const match = /^edge-(\d+)$/.exec(edge.id)
    if (!match) return
    const connection = {
      id: Number(match[1]),
      fromNodeType: 'image',
      fromNodeId: Number(edge.source.split('-')[1]),
      fromPort: 'image',
      toNodeType: 'image',
      toNodeId: Number(edge.target.split('-')[1]),
      toPort: 'image',
    } as const
    snapshot.connections.push(connection)
  })
  return snapshot
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
    expect(harness.nodes.value[1]!.position.x).toBeGreaterThan(harness.nodes.value[0]!.position.x)
    expect(harness.store.exportSnapshot().connections).toEqual([
      expect.objectContaining({
        id: 1,
        fromNodeId: 1,
        toNodeId: Number(reservation.resultNodeIds[0]?.split('-')[1]),
      }),
    ])
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
      expect(harness.edges.value).toHaveLength(1)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('reuses the first inactive empty image child before appending a slot', () => {
    const harness = createHarness()
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'image-2',
      targetHandle: 'in:image',
    })
    harness.syncGraph()

    const reservation = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })

    expect(reservation.resultNodeIds).toEqual(['image-2'])
    expect(harness.nodes.value).toHaveLength(2)
    expect(
      harness.nodes.value.find((node) => node.id === 'image-2')?.data.snapshotNode.outputStatus,
    ).toBe('running')
  })

  it('does not reuse an active empty child slot', () => {
    const harness = createHarness(['image-2'])
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'image-2',
      targetHandle: 'in:image',
    })
    harness.syncGraph()

    const reservation = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })

    expect(reservation.resultNodeIds[0]).not.toBe('image-2')
    expect(harness.nodes.value).toHaveLength(3)
  })

  it('places a new result below an occupied preferred column instead of colliding', () => {
    const harness = createHarness()
    harness.nodes.value.push({
      id: 'prompt-2',
      position: { x: 390, y: 20 },
      width: 280,
      height: 180,
      data: {
        kind: 'prompt',
        snapshotNode: {
          id: 2,
          type: 'prompt',
          x: 390,
          y: 20,
          width: 280,
          text: 'occupied',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [],
        },
      },
    })
    harness.syncGraph()

    const reservation = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })
    const resultNode = harness.nodes.value.find((node) => node.id === reservation.resultNodeIds[0])

    expect(resultNode).toBeDefined()
    if (!resultNode) throw new Error('Expected reserved result node')
    expect(resultNode.position).toMatchObject({ x: 390 })
    expect(resultNode.position.y).toBeGreaterThan(20)
  })

  it('attaches persisted task identity without adding graph history', () => {
    const harness = createHarness()
    const { resultNodeIds } = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })

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
  })
})
