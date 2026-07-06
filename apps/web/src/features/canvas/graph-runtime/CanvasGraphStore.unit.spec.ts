import { describe, expect, it } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../canvas-board/domain'
import {
  CanvasGraphStore,
  toCanvasGraphEdgeId,
  toCanvasGraphNodeId,
} from './CanvasGraphStore'
import {
  commitCanvasGraphConnectIntent,
  commitCanvasGraphDeleteIntent,
  commitCanvasGraphEdgeUpdateIntent,
  findCanvasGraphEdgeIdByEndpoint,
  isSameCanvasConnectionEndpoint,
  stageCanvasGraphNodeDragIntent,
} from '.'

function createImageNode(id: number, x: number, y: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x,
    y,
    width: 280,
    sourceImageUrl: `https://example.com/source-${id}.png`,
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'ready',
    outputImages: [{ url: `https://example.com/output-${id}.png`, ratio: 1 }],
  }
}

function createPromptNode(id: number, x: number, y: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x,
    y,
    width: 280,
    title: `Prompt ${id}`,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function createConnection(id: number, fromNodeId: number, toNodeId: number): CanvasConnection {
  return {
    id,
    fromNodeType: 'image',
    fromNodeId,
    fromPort: 'image',
    toNodeType: 'prompt',
    toNodeId,
    toPort: 'image',
  }
}

function createSampleSnapshot(): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.viewport = { canvasX: 120, canvasY: -40, zoomLevel: 75 }
  snapshot.imageNodes = [createImageNode(1, 40, 80)]
  snapshot.promptNodes = [createPromptNode(1, 420, 80), createPromptNode(2, 820, 80)]
  snapshot.connections = [createConnection(1, 1, 1)]
  return snapshot
}

function createLargeSnapshot(nodeCount: number): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  const pairCount = Math.floor(nodeCount / 2)
  for (let id = 1; id <= pairCount; id += 1) {
    snapshot.imageNodes.push(createImageNode(id, id * 8, 40))
    snapshot.promptNodes.push(createPromptNode(id, id * 8, 380))
    snapshot.connections.push(createConnection(id, id, id))
  }
  return snapshot
}

describe('CanvasGraphStore', () => {
  it('hydrates and exports a CanvasBoardSnapshot without changing the schema shape', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    expect(store.readCounts()).toEqual({ nodes: 3, edges: 1 })
    expect(store.getNodeIds()).toEqual([
      toCanvasGraphNodeId('image', 1),
      toCanvasGraphNodeId('prompt', 1),
      toCanvasGraphNodeId('prompt', 2),
    ])
    expect(store.getNodeIdsByKind('prompt')).toEqual([
      toCanvasGraphNodeId('prompt', 1),
      toCanvasGraphNodeId('prompt', 2),
    ])
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('commits node updates with projection, history, and dirty summary data', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const promptId = toCanvasGraphNodeId('prompt', 1)

    const summary = store
      .beginTransaction('node-update')
      .updateNode(promptId, { x: 560, text: 'updated prompt' })
      .commit()

    expect(summary).toMatchObject({
      reason: 'node-update',
      didChange: true,
      hasDomainChanges: true,
      changedNodeIds: [promptId],
      removedNodeIds: [],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(summary.projectionPatch.nodes.updated[0]?.snapshot).toMatchObject({
      id: 1,
      x: 560,
      text: 'updated prompt',
    })
    expect(store.exportSnapshot().promptNodes[0]).toMatchObject({
      id: 1,
      x: 560,
      text: 'updated prompt',
    })
  })

  it('adds and removes edges while keeping endpoint indexes current', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 2)
    const edgeId = toCanvasGraphEdgeId(2)

    const addSummary = store
      .beginTransaction('connect')
      .addEdge(createConnection(2, 1, 2))
      .commit()

    expect(addSummary.changedEdgeIds).toEqual([edgeId])
    expect(store.getOutgoingEdgeIds(imageId)).toEqual([toCanvasGraphEdgeId(1), edgeId])
    expect(store.getIncomingEdgeIds(promptId)).toEqual([edgeId])

    const removeSummary = store.beginTransaction('disconnect').removeEdge(edgeId).commit()

    expect(removeSummary.removedEdgeIds).toEqual([edgeId])
    expect(store.getIncomingEdgeIds(promptId)).toEqual([])
    expect(store.exportSnapshot().connections.map((connection) => connection.id)).toEqual([1])
  })

  it('batch deletes nodes and connected edges as one transaction', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const edgeId = toCanvasGraphEdgeId(1)

    const summary = store
      .beginTransaction('batch-delete')
      .batchDelete({ nodeIds: [imageId, promptId], removeConnectedEdges: true })
      .commit()

    expect(summary.removedNodeIds).toEqual([imageId, promptId])
    expect(summary.removedEdgeIds).toEqual([edgeId])
    expect(summary.didChangeOrder).toBe(true)
    expect(summary.shouldRecordHistory).toBe(true)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(store.exportSnapshot()).toMatchObject({
      imageNodes: [],
      promptNodes: [expect.objectContaining({ id: 2 })],
      connections: [],
    })
  })

  it('supports transaction cancel without mutating committed graph state', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const transaction = store.beginTransaction('node-update')

    transaction.updateNode(toCanvasGraphNodeId('prompt', 1), { x: 999 })
    const summary = transaction.cancel()

    expect(summary.didChange).toBe(false)
    expect(summary.shouldMarkDirty).toBe(false)
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
    expect(() => transaction.commit()).toThrow('already cancelled')
  })

  it('tracks node and edge selection without marking history or dirty', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const nodeId = toCanvasGraphNodeId('prompt', 1)
    const edgeId = toCanvasGraphEdgeId(1)

    const summary = store
      .beginTransaction('node-update')
      .selectNode(nodeId, true)
      .selectEdge(edgeId, true)
      .commit()

    expect(summary).toMatchObject({
      didChange: true,
      hasDomainChanges: false,
      selectedNodeIds: [nodeId],
      selectedEdgeIds: [edgeId],
      shouldRecordHistory: false,
      shouldMarkDirty: false,
    })
    expect(summary.projectionPatch.nodes.selectionChanged).toEqual([{ id: nodeId, selected: true }])
    expect(summary.projectionPatch.edges.selectionChanged).toEqual([{ id: edgeId, selected: true }])
    expect(store.getNode(nodeId)?.selected).toBe(true)
    expect(store.getEdge(edgeId)?.selected).toBe(true)
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('matches canonical edges by Canvas connection endpoint', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const sameEndpoint = createConnection(99, 1, 1)

    expect(isSameCanvasConnectionEndpoint(createConnection(1, 1, 1), sameEndpoint)).toBe(true)
    expect(findCanvasGraphEdgeIdByEndpoint(store, sameEndpoint)).toBe(
      toCanvasGraphEdgeId(1),
    )
  })

  it('commits connect intents with single-input last-write-wins policy', () => {
    const snapshot = createSampleSnapshot()
    snapshot.imageNodes.push(createImageNode(2, 80, 120))
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const summary = commitCanvasGraphConnectIntent(store, {
      connection: createConnection(2, 2, 1),
      allowsMultipleTargetConnections: false,
    })

    expect(summary).toMatchObject({
      reason: 'connect',
      changedEdgeIds: [toCanvasGraphEdgeId(2)],
      removedEdgeIds: [toCanvasGraphEdgeId(1)],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(store.exportSnapshot().connections).toEqual([createConnection(2, 2, 1)])
  })

  it('commits edge update intents while replacing conflicting target inputs', () => {
    const snapshot = createSampleSnapshot()
    snapshot.imageNodes.push(createImageNode(2, 80, 120))
    snapshot.connections.push(createConnection(2, 2, 2))
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const summary = commitCanvasGraphEdgeUpdateIntent(store, {
      edgeId: toCanvasGraphEdgeId(2),
      connection: createConnection(2, 2, 1),
      allowsMultipleTargetConnections: false,
    })

    expect(summary.changedEdgeIds).toEqual([toCanvasGraphEdgeId(2)])
    expect(summary.removedEdgeIds).toEqual([toCanvasGraphEdgeId(1)])
    expect(store.exportSnapshot().connections).toEqual([createConnection(2, 2, 1)])
  })

  it('stages node drag position and selection as graph runtime intent', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const transaction = store.beginTransaction('node-drag')

    expect(
      stageCanvasGraphNodeDragIntent(transaction, {
        nodeId: promptId,
        position: { x: 640, y: 120 },
      }),
    ).toBe(true)
    expect(
      stageCanvasGraphNodeDragIntent(transaction, {
        nodeId: promptId,
        selected: true,
      }),
    ).toBe(true)
    expect(
      stageCanvasGraphNodeDragIntent(transaction, {
        nodeId: 'vue-flow-generated-node',
        position: { x: 1, y: 1 },
      }),
    ).toBe(false)

    const summary = transaction.commit()

    expect(summary.changedNodeIds).toEqual([promptId])
    expect(summary.selectedNodeIds).toEqual([promptId])
    expect(store.exportSnapshot().promptNodes[0]).toMatchObject({ x: 640, y: 120 })
    expect(store.getNode(promptId)?.selected).toBe(true)
  })

  it('commits delete and disconnect intents with canonical transaction reasons', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const edgeId = toCanvasGraphEdgeId(1)

    const deleteSummary = commitCanvasGraphDeleteIntent(store, {
      nodeIds: [imageId],
      removeConnectedEdges: true,
    })

    expect(deleteSummary.reason).toBe('delete')
    expect(deleteSummary.removedNodeIds).toEqual([imageId])
    expect(deleteSummary.removedEdgeIds).toEqual([edgeId])
    expect(store.exportSnapshot().imageNodes).toEqual([])
    expect(store.exportSnapshot().connections).toEqual([])

    const disconnectStore = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const disconnectSummary = commitCanvasGraphDeleteIntent(disconnectStore, {
      edgeIds: [edgeId],
    })

    expect(disconnectSummary.reason).toBe('disconnect')
    expect(disconnectSummary.removedNodeIds).toEqual([])
    expect(disconnectSummary.removedEdgeIds).toEqual([edgeId])
    expect(disconnectStore.exportSnapshot().connections).toEqual([])
  })

  it.each([5_000, 10_000])(
    'commits a store-level mutation for %i nodes without DOM',
    (nodeCount) => {
      const snapshot = createLargeSnapshot(nodeCount)
      const store = CanvasGraphStore.fromSnapshot(snapshot)
      const promptId = toCanvasGraphNodeId('prompt', Math.floor(nodeCount / 2))
      const startedAt = performance.now()

      const summary = store
        .beginTransaction('node-update')
        .updateNode(promptId, (node) => ({ ...node, x: node.x + 24 }))
        .commit()

      const durationMs = performance.now() - startedAt
      expect(summary.changedNodeIds).toEqual([promptId])
      expect(summary.shouldMarkDirty).toBe(true)
      expect(store.readCounts()).toEqual({
        nodes: nodeCount,
        edges: Math.floor(nodeCount / 2),
      })
      expect(durationMs).toBeLessThan(1_000)
    },
  )
})
