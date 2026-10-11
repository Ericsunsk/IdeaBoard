import { describe, expect, it, vi } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasConnection,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import {
  CanvasGraphSession,
  CanvasGraphStore,
  commitCanvasGraphConnectIntent,
  commitCanvasGraphDeleteGroupIntent,
  commitCanvasGraphDeleteIntent,
  stageCanvasGraphNodeDragIntent,
  toCanvasGraphEdgeId,
  toCanvasGraphGroupId,
  toCanvasGraphNodeId,
  type CanvasGraphTransactionReason,
} from './index'

const GROUP_GEOMETRY = { x: 20, y: 20, width: 700, height: 360 }

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

function createSampleSnapshot(): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = [createImageNode(1, 40, 80), createImageNode(2, 80, 120)]
  snapshot.promptNodes = [createPromptNode(1, 420, 80), createPromptNode(2, 820, 80)]
  snapshot.connections = [createConnection(1, 1, 1)]
  return snapshot
}

function subscribeOnce(store: CanvasGraphStore) {
  const summaries: Array<{
    reason: CanvasGraphTransactionReason
    shouldRecordHistory: boolean
    shouldMarkDirty: boolean
  }> = []
  store.subscribe((event) => {
    summaries.push({
      reason: event.summary.reason,
      shouldRecordHistory: event.summary.shouldRecordHistory,
      shouldMarkDirty: event.summary.shouldMarkDirty,
    })
  })
  return { summaries }
}

describe('canvas graph transaction ownership', () => {
  it('commits node-drag as a single transaction with history and autosave intent', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)
    const nodeId = toCanvasGraphNodeId('prompt', 1)

    const transaction = store.beginTransaction('node-drag')
    expect(
      stageCanvasGraphNodeDragIntent(transaction, { nodeId, position: { x: 640, y: 120 } }),
    ).toBe(true)
    const summary = transaction.commit()

    expect(summary.reason).toBe('node-drag')
    expect(summary.shouldRecordHistory).toBe(true)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(summaries).toHaveLength(1)
    expect(summaries[0]).toMatchObject({ reason: 'node-drag' })
  })

  it('commits connect as a single transaction with history and autosave intent', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)

    const summary = commitCanvasGraphConnectIntent(store, {
      connection: createConnection(2, 2, 1),
      allowsMultipleTargetConnections: false,
    })

    expect(summary.reason).toBe('connect')
    expect(summary.shouldRecordHistory).toBe(true)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(summaries).toHaveLength(1)
  })

  it('commits disconnect and delete intents with canonical reasons', () => {
    const disconnectStore = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const disconnectNotifications = subscribeOnce(disconnectStore)
    const disconnectSummary = commitCanvasGraphDeleteIntent(disconnectStore, {
      edgeIds: [toCanvasGraphEdgeId(1)],
    })

    expect(disconnectSummary.reason).toBe('disconnect')
    expect(disconnectSummary.shouldRecordHistory).toBe(true)
    expect(disconnectSummary.shouldMarkDirty).toBe(true)
    expect(disconnectNotifications.summaries).toHaveLength(1)

    const deleteStore = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const deleteNotifications = subscribeOnce(deleteStore)
    const deleteSummary = commitCanvasGraphDeleteIntent(deleteStore, {
      nodeIds: [toCanvasGraphNodeId('image', 1)],
      removeConnectedEdges: true,
    })

    expect(deleteSummary.reason).toBe('delete')
    expect(deleteSummary.removedEdgeIds).toEqual([toCanvasGraphEdgeId(1)])
    expect(deleteNotifications.summaries).toHaveLength(1)
  })

  it('commits batch delete as a single transaction', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)

    const summary = commitCanvasGraphDeleteIntent(store, {
      nodeIds: [toCanvasGraphNodeId('image', 1), toCanvasGraphNodeId('prompt', 1)],
      removeConnectedEdges: true,
    })

    expect(summary.reason).toBe('batch-delete')
    expect(summary.shouldRecordHistory).toBe(true)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(summaries).toHaveLength(1)
  })

  it('commits node add and update as single node-update transactions', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)

    const added = store
      .beginTransaction('node-update')
      .addNode(createPromptNode(9, 100, 100))
      .commit()
    expect(added.reason).toBe('node-update')
    expect(added.shouldRecordHistory).toBe(true)
    expect(added.shouldMarkDirty).toBe(true)

    const updated = store
      .beginTransaction('node-update')
      .updateNode(toCanvasGraphNodeId('prompt', 1), { x: 560 })
      .commit()
    expect(updated.changedNodeIds).toEqual([toCanvasGraphNodeId('prompt', 1)])
    expect(summaries).toHaveLength(2)
  })

  it('commits group lifecycle intents as single transactions', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)

    const grouped = store
      .beginTransaction('group')
      .createGroup([imageId, promptId], GROUP_GEOMETRY)
      .commit()
    expect(grouped.reason).toBe('group')
    expect(grouped.shouldRecordHistory).toBe(true)

    const moved = store
      .beginTransaction('group-membership')
      .moveNodeToGroup(secondPromptId, grouped.changedGroupIds[0]!)
      .commit()
    expect(moved.reason).toBe('group-membership')
    expect(moved.shouldRecordHistory).toBe(true)

    const groupId = grouped.changedGroupIds[0]!
    const dragMoved = store
      .beginTransaction('group-move')
      .updateNode(imageId, { x: 111, y: 222 })
      .updateNode(promptId, { x: 333, y: 444 })
      .commit()
    expect(dragMoved.reason).toBe('group-move')
    expect(dragMoved.shouldRecordHistory).toBe(true)

    const resized = store
      .beginTransaction('group-resize')
      .updateGroupGeometry(groupId, { x: 10, y: 12, width: 540, height: 260 })
      .commit()
    expect(resized.reason).toBe('group-resize')
    expect(resized.shouldRecordHistory).toBe(true)

    const ungrouped = store.beginTransaction('ungroup').ungroup(groupId).commit()
    expect(ungrouped.reason).toBe('ungroup')
    expect(ungrouped.shouldRecordHistory).toBe(true)
    expect(summaries).toHaveLength(5)
  })

  it('commits group-delete as a single transaction', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    snapshot.groups = [
      {
        id: groupId,
        nodeIds: [toCanvasGraphNodeId('image', 1)],
        x: 20,
        y: 20,
        width: 700,
        height: 360,
      },
    ]
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const { summaries } = subscribeOnce(store)

    const summary = commitCanvasGraphDeleteGroupIntent(store, { groupId })
    expect(summary.reason).toBe('group-delete')
    expect(summary.shouldRecordHistory).toBe(true)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(summaries).toHaveLength(1)
  })

  it('commits result materialization without history but with autosave intent', () => {
    const snapshot = createSampleSnapshot()
    const pending: ImageNodeItem = {
      ...createImageNode(3, 200, 200),
      outputStatus: 'running',
      pendingTaskId: 'task-1',
    }
    snapshot.imageNodes.push(pending)
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const { summaries } = subscribeOnce(store)
    const nodeId = toCanvasGraphNodeId('image', 3)

    const summary = store
      .beginTransaction('result-materialize', { recordHistory: false, markDirty: true })
      .updateNode(nodeId, { outputStatus: 'ready', pendingTaskId: undefined })
      .commit()

    expect(summary.reason).toBe('result-materialize')
    expect(summary.shouldRecordHistory).toBe(false)
    expect(summary.shouldMarkDirty).toBe(true)
    expect(summaries).toHaveLength(1)
  })

  it('tracks selection without history or autosave intent', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const { summaries } = subscribeOnce(store)

    const summary = store
      .beginTransaction('node-update', { recordHistory: false, markDirty: false })
      .selectNode(toCanvasGraphNodeId('prompt', 1), true)
      .commit()

    expect(summary.hasDomainChanges).toBe(false)
    expect(summary.shouldRecordHistory).toBe(false)
    expect(summary.shouldMarkDirty).toBe(false)
    expect(summaries).toHaveLength(1)
  })

  it('emits no commit event for cancelled transactions', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const listener = vi.fn()
    store.subscribe(listener)

    const transaction = store.beginTransaction('node-update')
    transaction.updateNode(toCanvasGraphNodeId('prompt', 1), { x: 999 })
    const summary = transaction.cancel()

    expect(summary.didChange).toBe(false)
    expect(listener).not.toHaveBeenCalled()
  })

  it('routes history and autosave through a single session owner', () => {
    const session = CanvasGraphSession.hydrate('board-1', createSampleSnapshot())
    const historyRecord = vi.fn()
    const markDirty = vi.fn()
    session.subscribe((event) => {
      if (event.sideEffects.recordHistory) historyRecord(event.patch)
      if (event.sideEffects.markDirty) markDirty()
    })

    commitCanvasGraphConnectIntent(session.store, {
      connection: createConnection(2, 2, 2),
      allowsMultipleTargetConnections: true,
    })
    expect(historyRecord).toHaveBeenCalledTimes(1)
    expect(markDirty).toHaveBeenCalledTimes(1)

    session.store
      .beginTransaction('result-materialize', { recordHistory: false, markDirty: true })
      .updateNode(toCanvasGraphNodeId('image', 1), { x: 41 })
      .commit()
    expect(historyRecord).toHaveBeenCalledTimes(1)
    expect(markDirty).toHaveBeenCalledTimes(2)

    session.store
      .beginTransaction('node-update', { recordHistory: false, markDirty: false })
      .selectNode(toCanvasGraphNodeId('prompt', 1), true)
      .commit()
    expect(historyRecord).toHaveBeenCalledTimes(1)
    expect(markDirty).toHaveBeenCalledTimes(2)
    session.dispose()
  })
})
