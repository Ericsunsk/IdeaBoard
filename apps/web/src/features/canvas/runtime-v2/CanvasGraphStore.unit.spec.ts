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
  createCanvasGraphConnectionId,
  createCanvasGraphImageNodeId,
  createCanvasGraphPromptNodeId,
} from './CanvasGraphStore'

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

function createConnection(
  id: number,
  fromNodeId: number,
  toNodeId: number,
): CanvasConnection {
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
      createCanvasGraphImageNodeId(1),
      createCanvasGraphPromptNodeId(1),
      createCanvasGraphPromptNodeId(2),
    ])
    expect(store.getNodeIdsByKind('prompt')).toEqual([
      createCanvasGraphPromptNodeId(1),
      createCanvasGraphPromptNodeId(2),
    ])
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('commits node updates with projection, history, and dirty summary data', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const promptId = createCanvasGraphPromptNodeId(1)

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
    const imageId = createCanvasGraphImageNodeId(1)
    const promptId = createCanvasGraphPromptNodeId(2)
    const edgeId = createCanvasGraphConnectionId(2)

    const addSummary = store
      .beginTransaction('connect')
      .addEdge(createConnection(2, 1, 2))
      .commit()

    expect(addSummary.changedEdgeIds).toEqual([edgeId])
    expect(store.getOutgoingEdgeIds(imageId)).toEqual([
      createCanvasGraphConnectionId(1),
      edgeId,
    ])
    expect(store.getIncomingEdgeIds(promptId)).toEqual([edgeId])

    const removeSummary = store.beginTransaction('disconnect').removeEdge(edgeId).commit()

    expect(removeSummary.removedEdgeIds).toEqual([edgeId])
    expect(store.getIncomingEdgeIds(promptId)).toEqual([])
    expect(store.exportSnapshot().connections.map((connection) => connection.id)).toEqual([1])
  })

  it('batch deletes nodes and connected edges as one transaction', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = createCanvasGraphImageNodeId(1)
    const promptId = createCanvasGraphPromptNodeId(1)
    const edgeId = createCanvasGraphConnectionId(1)

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

    transaction.updateNode(createCanvasGraphPromptNodeId(1), { x: 999 })
    const summary = transaction.cancel()

    expect(summary.didChange).toBe(false)
    expect(summary.shouldMarkDirty).toBe(false)
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
    expect(() => transaction.commit()).toThrow('already cancelled')
  })

  it('tracks node and edge selection without marking history or dirty', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const nodeId = createCanvasGraphPromptNodeId(1)
    const edgeId = createCanvasGraphConnectionId(1)

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

  it.each([5_000, 10_000])(
    'commits a store-level mutation for %i nodes without DOM',
    (nodeCount) => {
      const snapshot = createLargeSnapshot(nodeCount)
      const store = CanvasGraphStore.fromSnapshot(snapshot)
      const promptId = createCanvasGraphPromptNodeId(Math.floor(nodeCount / 2))
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
