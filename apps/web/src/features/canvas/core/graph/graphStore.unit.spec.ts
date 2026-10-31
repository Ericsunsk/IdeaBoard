import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasConnection,
  type CanvasGroup,
  type CanvasNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import {
  CanvasGraphStore,
  toCanvasGraphEdgeId,
  toCanvasGraphGroupId,
  toCanvasGraphNodeId,
} from './graphStore'
import { CanvasGraphSession } from '../../session/board-session'
import type { CanvasGraphStoreTelemetry } from './graphStoreTelemetry'
import {
  commitCanvasGraphConnectIntent,
  commitCanvasGraphDeleteGroupIntent,
  commitCanvasGraphDeleteIntent,
  findCanvasGraphEdgeIdByEndpoint,
  isSameCanvasConnectionEndpoint,
  stageCanvasGraphNodeDragIntent,
} from '.'

const GROUP_GEOMETRY = {
  x: 20,
  y: 20,
  width: 700,
  height: 360,
}

function createGroup(
  id: CanvasGroup['id'],
  nodeIds: CanvasGroup['nodeIds'],
  geometry: Pick<CanvasGroup, 'x' | 'y' | 'width' | 'height'> = GROUP_GEOMETRY,
): CanvasGroup {
  return {
    id,
    nodeIds,
    ...geometry,
  }
}

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
  snapshot.imageNodes = [createImageNode(1, 40, 80)]
  snapshot.promptNodes = [createPromptNode(1, 420, 80), createPromptNode(2, 820, 80)]
  snapshot.connections = [createConnection(1, 1, 1)]
  snapshot.mediaRefs = [
    { mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' },
  ]
  return snapshot
}

function createLargeSnapshot(nodeCount: number): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  const pairCount = Math.floor(nodeCount / 2)
  for (let id = 1; id <= pairCount; id += 1) {
    snapshot.imageNodes.push(createImageNode(id, id * 8, 40))
    snapshot.promptNodes.push(createPromptNode(id, id * 8, 380))
    snapshot.connections.push(createConnection(id, id, id))
  }
  return snapshot
}

describe('CanvasGraphStore', () => {
  it('rejects domain commits and undo when capability is revoked while preserving selection', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes = [createImageNode(1, 10, 20)]
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const edit = store.beginTransaction('node-update').updateNode('image-1', { x: 50 }).commit()
    let canWrite = false
    store.setDomainWritePolicy(() => canWrite)
    const commits = vi.fn()
    store.subscribe(commits)
    const blocked = store.beginTransaction('node-update').updateNode('image-1', { x: 99 }).commit()
    expect(blocked.didChange).toBe(false)
    expect(store.getNode('image-1')?.snapshot.x).toBe(50)
    expect(store.applyCommitPatch(edit.commitPatch, 'backward').didChange).toBe(false)
    expect(commits).not.toHaveBeenCalled()
    expect(
      store.beginTransaction('node-update').selectNode('image-1', true).commit().didChange,
    ).toBe(true)
    canWrite = true
    expect(
      store.beginTransaction('node-update').updateNode('image-1', { x: 70 }).commit().didChange,
    ).toBe(true)
    expect(store.getNode('image-1')?.snapshot.x).toBe(70)
  })

  afterEach(() => {
    delete window.__canvasInteractionBenchmarkCounters
  })

  it('leaves benchmark counters untouched without injected telemetry', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())

    store.hydrate(createEmptyBoardSnapshot())

    expect(window.__canvasInteractionBenchmarkCounters.runtimeTimingMs).toBeUndefined()
  })

  it('routes constructor and explicit hydrate measurements through injected telemetry', () => {
    const isMeasured: string[] = []
    const telemetry: CanvasGraphStoreTelemetry = {
      measureHydrate: (counts, task) => {
        isMeasured.push(`hydrate:${counts.nodes}:${counts.edges}:${counts.groups}`)
        return task()
      },
      measureDraft: (_counts, task) => task(),
      recordCommit: () => {},
    }
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot(), telemetry)

    store.hydrate(createEmptyBoardSnapshot())

    expect(isMeasured).toEqual(['hydrate:3:1:0', 'hydrate:0:0:0'])
  })

  it('hydrates and exports a BoardSnapshot without changing the schema shape', () => {
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
    const exported = store.exportSnapshot(snapshot)
    expect(exported).toEqual(snapshot)
    expect(exported.mediaRefs).not.toBe(snapshot.mediaRefs)
  })

  it('round trips persistent groups as graph state', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]

    const store = CanvasGraphStore.fromSnapshot(snapshot)

    expect(store.getGroupIds()).toEqual([groupId])
    expect(store.getGroup(groupId)).toEqual(createGroup(groupId, [imageId, promptId]))
    expect(store.getGroupIdForNode(imageId)).toBe(groupId)
    expect(store.exportSnapshot()).toMatchObject({
      groups: [createGroup(groupId, [imageId, promptId])],
    })
  })

  it('round trips persistent groups with zero or one member as graph frames', () => {
    const snapshot = createSampleSnapshot()
    const firstGroupId = toCanvasGraphGroupId(1)
    const secondGroupId = toCanvasGraphGroupId(2)
    const imageId = toCanvasGraphNodeId('image', 1)
    snapshot.groups = [createGroup(firstGroupId, [imageId]), createGroup(secondGroupId, [])]

    const store = CanvasGraphStore.fromSnapshot(snapshot)

    expect(store.getGroupIds()).toEqual([firstGroupId, secondGroupId])
    expect(store.getGroup(firstGroupId)).toEqual(createGroup(firstGroupId, [imageId]))
    expect(store.getGroup(secondGroupId)).toEqual(createGroup(secondGroupId, []))
    expect(store.exportSnapshot()).toMatchObject({
      groups: [createGroup(firstGroupId, [imageId]), createGroup(secondGroupId, [])],
    })
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

  it('publishes a compact reversible commit patch and isolates listener failures', () => {
    const store = CanvasGraphStore.fromSnapshot(createLargeSnapshot(2_500))
    const received = vi.fn()
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    store.subscribe(() => {
      throw new Error('renderer unavailable')
    })
    store.subscribe(received)

    const added = createPromptNode(1_251, 999, 999)
    const summary = store.beginTransaction('node-update').addNode(added).commit()

    expect(summary.commitPatch.nodes.records).toHaveLength(1)
    expect(summary.commitPatch.nodes.order).toEqual([
      expect.objectContaining({
        id: toCanvasGraphNodeId('prompt', 1_251),
        after: expect.any(Object),
      }),
    ])
    expect(JSON.stringify(summary.commitPatch).length).toBeLessThan(2_000)
    expect(store.getNode(toCanvasGraphNodeId('prompt', 1_251))).toBeDefined()
    expect(received).toHaveBeenCalledWith(expect.objectContaining({ patch: summary.commitPatch }))
    expect(error).toHaveBeenCalledTimes(1)

    const removed = store
      .beginTransaction('delete')
      .removeNode(toCanvasGraphNodeId('prompt', 1_251))
      .commit()
    expect(removed.commitPatch.nodes.records).toHaveLength(1)
    expect(removed.commitPatch.nodes.order).toHaveLength(1)
    expect(JSON.stringify(removed.commitPatch).length).toBeLessThan(2_000)
    error.mockRestore()
  })

  it('restores nodes, edges, groups, order, and selection from a commit patch', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const groupId = toCanvasGraphGroupId(1)
    const summary = store
      .beginTransaction('group')
      .selectNode(promptId, true)
      .removeNode(imageId, { shouldRemoveConnectedEdges: true })
      .createGroup([promptId, toCanvasGraphNodeId('prompt', 2)], GROUP_GEOMETRY, groupId)
      .commit()

    const undo = store.applyCommitPatch(summary.commitPatch, 'backward')
    expect(undo.reason).toBe('history-undo')
    expect(store.getNode(imageId)).toBeDefined()
    expect(store.getNode(promptId)?.isSelected).toBe(false)
    expect(store.getGroup(groupId)).toBeUndefined()
    expect(store.getNodeIds()).toEqual([imageId, promptId, toCanvasGraphNodeId('prompt', 2)])

    store.applyCommitPatch(summary.commitPatch, 'forward')
    expect(store.getNode(imageId)).toBeUndefined()
    expect(store.getNode(promptId)?.isSelected).toBe(true)
    expect(store.getGroup(groupId)).toBeDefined()
  })

  it('detaches session observers before a stale store can publish', () => {
    const session = CanvasGraphSession.hydrate('board-1', createSampleSnapshot())
    const staleStore = session.store
    const listener = vi.fn()
    session.subscribe(listener)

    session.dispose()
    staleStore.beginTransaction('node-update').updateNode('prompt-1', { x: 640 }).commit()

    expect(listener).not.toHaveBeenCalled()
    expect(staleStore.getNode('prompt-1')?.snapshot.x).toBe(640)
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
      .batchDelete({ nodeIds: [imageId, promptId], shouldRemoveConnectedEdges: true })
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

  it('creates and ungroups persistent groups without touching member nodes', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const groupId = toCanvasGraphGroupId(1)

    const groupSummary = store
      .beginTransaction('group')
      .createGroup([imageId, promptId], GROUP_GEOMETRY)
      .commit()

    expect(groupSummary).toMatchObject({
      reason: 'group',
      didChange: true,
      hasDomainChanges: true,
      changedGroupIds: [groupId],
      removedGroupIds: [],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(groupSummary.projectionPatch.groups.added).toEqual([
      createGroup(groupId, [imageId, promptId]),
    ])
    expect(store.exportSnapshot().groups).toEqual([createGroup(groupId, [imageId, promptId])])

    const ungroupSummary = store.beginTransaction('ungroup').ungroup(groupId).commit()

    expect(ungroupSummary).toMatchObject({
      reason: 'ungroup',
      changedGroupIds: [],
      removedGroupIds: [groupId],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(ungroupSummary.projectionPatch.groups.removedIds).toEqual([groupId])
    expect(store.getNode(imageId)).toBeTruthy()
    expect(store.getNode(promptId)).toBeTruthy()
    expect(store.exportSnapshot().groups).toEqual([])
  })

  it('commits group-delete intents for empty frames without touching nodes or edges', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    snapshot.groups = [createGroup(groupId, [])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const summary = commitCanvasGraphDeleteGroupIntent(store, { groupId })

    expect(summary).toMatchObject({
      reason: 'group-delete',
      didChange: true,
      hasDomainChanges: true,
      changedNodeIds: [],
      changedEdgeIds: [],
      removedNodeIds: [],
      removedEdgeIds: [],
      changedGroupIds: [],
      removedGroupIds: [groupId],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(summary.projectionPatch.groups.removedIds).toEqual([groupId])
    expect(store.exportSnapshot()).toMatchObject({
      imageNodes: [expect.objectContaining({ id: 1 })],
      promptNodes: [expect.objectContaining({ id: 1 }), expect.objectContaining({ id: 2 })],
      connections: [expect.objectContaining({ id: 1 })],
      groups: [],
    })
  })

  it('commits group-delete intents for member nodes and connected edges', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const edgeId = toCanvasGraphEdgeId(1)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const summary = commitCanvasGraphDeleteGroupIntent(store, {
      groupId,
      shouldDeleteMembers: true,
    })

    expect(summary).toMatchObject({
      reason: 'group-delete',
      removedNodeIds: [imageId, promptId],
      removedEdgeIds: [edgeId],
      removedGroupIds: [groupId],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(store.exportSnapshot()).toMatchObject({
      imageNodes: [],
      promptNodes: [expect.objectContaining({ id: 2 })],
      connections: [],
      groups: [],
    })
  })

  it('rejects missing or already grouped nodes when creating a group', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)

    store.beginTransaction('group').createGroup([imageId, promptId], GROUP_GEOMETRY).commit()

    expect(() =>
      store
        .beginTransaction('group')
        .createGroup([imageId, secondPromptId], GROUP_GEOMETRY)
        .commit(),
    ).toThrow('node already belongs to group')
    expect(() =>
      store
        .beginTransaction('group')
        .createGroup([secondPromptId, toCanvasGraphNodeId('image', 999)], GROUP_GEOMETRY)
        .commit(),
    ).toThrow('group node does not exist')
    expect(() =>
      store.beginTransaction('group').createGroup([secondPromptId], GROUP_GEOMETRY).commit(),
    ).toThrow('requires at least two nodes')
  })

  it('removes stale group membership when deleting nodes', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)
    snapshot.groups = [createGroup(groupId, [imageId, promptId, secondPromptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const updateSummary = store
      .beginTransaction('delete')
      .removeNode(imageId, { shouldRemoveConnectedEdges: true })
      .commit()

    expect(updateSummary.changedGroupIds).toEqual([groupId])
    expect(updateSummary.projectionPatch.groups.updated).toEqual([
      createGroup(groupId, [promptId, secondPromptId]),
    ])
    expect(store.exportSnapshot().groups).toEqual([
      createGroup(groupId, [promptId, secondPromptId]),
    ])

    const removeSummary = store.beginTransaction('delete').removeNode(promptId).commit()

    expect(removeSummary.changedGroupIds).toEqual([groupId])
    expect(removeSummary.removedGroupIds).toEqual([])
    expect(store.exportSnapshot().groups).toEqual([createGroup(groupId, [secondPromptId])])

    const emptySummary = store.beginTransaction('delete').removeNode(secondPromptId).commit()

    expect(emptySummary.changedGroupIds).toEqual([groupId])
    expect(emptySummary.removedGroupIds).toEqual([])
    expect(store.exportSnapshot().groups).toEqual([createGroup(groupId, [])])
  })

  it('moves nodes in and out of groups through membership transactions', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const addSummary = store
      .beginTransaction('group-membership')
      .moveNodeToGroup(secondPromptId, groupId)
      .commit()

    expect(addSummary.changedGroupIds).toEqual([groupId])
    expect(addSummary.projectionPatch.groups.updated).toEqual([
      createGroup(groupId, [imageId, promptId, secondPromptId]),
    ])
    expect(store.exportSnapshot().groups).toEqual([
      createGroup(groupId, [imageId, promptId, secondPromptId]),
    ])

    const removeSummary = store
      .beginTransaction('group-membership')
      .moveNodeToGroup(secondPromptId, null)
      .commit()

    expect(removeSummary.changedGroupIds).toEqual([groupId])
    expect(store.exportSnapshot().groups).toEqual([createGroup(groupId, [imageId, promptId])])
  })

  it('keeps groups when moveNodeToGroup removes their final members', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const singleMemberSummary = store
      .beginTransaction('group-membership')
      .moveNodeToGroup(imageId, null)
      .commit()

    expect(singleMemberSummary.changedGroupIds).toEqual([groupId])
    expect(singleMemberSummary.removedGroupIds).toEqual([])
    expect(store.getGroupIdForNode(imageId)).toBeUndefined()
    expect(store.getGroup(groupId)).toEqual(createGroup(groupId, [promptId]))

    const emptySummary = store
      .beginTransaction('group-membership')
      .moveNodeToGroup(promptId, null)
      .commit()

    expect(emptySummary.changedGroupIds).toEqual([groupId])
    expect(emptySummary.removedGroupIds).toEqual([])
    expect(store.getGroupIdForNode(promptId)).toBeUndefined()
    expect(store.getGroupIds()).toEqual([groupId])
    expect(store.exportSnapshot().groups).toEqual([createGroup(groupId, [])])
  })

  it('updates group geometry without changing membership', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const summary = store
      .beginTransaction('group-resize')
      .updateGroupGeometry(groupId, { x: 10, y: 12, width: 540, height: 260 })
      .commit()

    expect(summary.changedGroupIds).toEqual([groupId])
    expect(summary.projectionPatch.groups.updated).toEqual([
      createGroup(groupId, [imageId, promptId], { x: 10, y: 12, width: 540, height: 260 }),
    ])
    expect(store.exportSnapshot().groups).toEqual([
      createGroup(groupId, [imageId, promptId], { x: 10, y: 12, width: 540, height: 260 }),
    ])
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

  it('keeps staged topology and group-index writes isolated until commit', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)
    snapshot.groups = [createGroup(groupId, [imageId, promptId])]
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const transaction = store.beginTransaction('group-membership')

    transaction
      .moveNodeToGroup(secondPromptId, groupId)
      .addEdge(createConnection(2, 1, 2))
      .removeEdge(toCanvasGraphEdgeId(1))
      .cancel()

    expect(store.getIncomingEdgeIds(promptId)).toEqual([toCanvasGraphEdgeId(1)])
    expect(store.getIncomingEdgeIds(secondPromptId)).toEqual([])
    expect(store.getGroup(groupId)?.nodeIds).toEqual([imageId, promptId])
    expect(store.getGroupIdForNode(secondPromptId)).toBeUndefined()
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('rejects an overlapping transaction after another transaction commits', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const first = store.beginTransaction('node-update')
    const stale = store.beginTransaction('node-update')

    stale.updateNode(promptId, { x: 900 })
    first.updateNode(promptId, { x: 500 }).commit()

    expect(() => stale.commit()).toThrow('transaction is stale')
    expect(store.getNode(promptId)?.snapshot.x).toBe(500)
  })

  it('rejects stale mutations before they can read a newer committed record', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const first = store.beginTransaction('node-update')
    const stale = store.beginTransaction('node-update')
    const staleUpdate = vi.fn((node: CanvasNodeItem) => ({ ...node, x: node.x + 1 }))

    first.updateNode(promptId, { x: 500 }).commit()

    expect(() => stale.updateNode(promptId, staleUpdate)).toThrow('transaction is stale')
    expect(staleUpdate).not.toHaveBeenCalled()
    expect(stale.cancel().didChange).toBe(false)
  })

  it('normalizes add-then-remove journals to a no-op commit', () => {
    const snapshot = createSampleSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const transaction = store.beginTransaction('batch-delete')
    const transientNode = createPromptNode(99, 1200, 80)
    const transientNodeId = toCanvasGraphNodeId('prompt', transientNode.id)
    const transientEdge = createConnection(99, 1, 2)
    const transientEdgeId = toCanvasGraphEdgeId(transientEdge.id)
    const transientGroupId = toCanvasGraphGroupId(99)

    transaction.addNode(transientNode).removeNode(transientNodeId)
    transaction.addEdge(transientEdge).removeEdge(transientEdgeId)
    transaction.createGroup(
      [toCanvasGraphNodeId('image', 1), toCanvasGraphNodeId('prompt', 1)],
      GROUP_GEOMETRY,
      transientGroupId,
    )
    transaction.ungroup(transientGroupId)
    const summary = transaction.commit()

    expect(summary).toMatchObject({
      didChange: false,
      hasDomainChanges: false,
      didChangeOrder: false,
      shouldRecordHistory: false,
      shouldMarkDirty: false,
    })
    expect(summary.projectionPatch).toEqual({
      nodes: { added: [], updated: [], removedIds: [], selectionChanged: [] },
      edges: { added: [], removedIds: [], selectionChanged: [] },
      groups: { added: [], updated: [], removedIds: [] },
    })
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('normalizes restored records and selection to a no-op commit', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    snapshot.groups = [
      createGroup(groupId, [toCanvasGraphNodeId('image', 1), toCanvasGraphNodeId('prompt', 1)]),
    ]
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const edgeId = toCanvasGraphEdgeId(1)
    const transaction = store.beginTransaction('node-update')

    transaction.updateNode(promptId, { x: 500 }).updateNode(promptId, { x: 420 })
    transaction.selectNode(promptId, true).selectNode(promptId, false)
    transaction.selectEdge(edgeId, true).selectEdge(edgeId, false)
    transaction
      .updateGroupGeometry(groupId, { ...GROUP_GEOMETRY, x: 100 })
      .updateGroupGeometry(groupId, GROUP_GEOMETRY)
    const summary = transaction.commit()

    expect(summary.didChange).toBe(false)
    expect(summary.shouldRecordHistory).toBe(false)
    expect(summary.shouldMarkDirty).toBe(false)
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
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
    expect(summary.projectionPatch.nodes.selectionChanged).toEqual([
      { id: nodeId, isSelected: true },
    ])
    expect(summary.projectionPatch.edges.selectionChanged).toEqual([
      { id: edgeId, isSelected: true },
    ])
    expect(store.getNode(nodeId)?.isSelected).toBe(true)
    expect(store.getEdge(edgeId)?.isSelected).toBe(true)
    expect(store.exportSnapshot(snapshot)).toEqual(snapshot)
  })

  it('matches canonical edges by Canvas connection endpoint', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const sameEndpoint = createConnection(99, 1, 1)

    expect(isSameCanvasConnectionEndpoint(createConnection(1, 1, 1), sameEndpoint)).toBe(true)
    expect(findCanvasGraphEdgeIdByEndpoint(store, sameEndpoint)).toBe(toCanvasGraphEdgeId(1))
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
        isSelected: true,
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
    expect(store.getNode(promptId)?.isSelected).toBe(true)
  })

  it('commits delete and disconnect intents with canonical transaction reasons', () => {
    const store = CanvasGraphStore.fromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const edgeId = toCanvasGraphEdgeId(1)

    const deleteSummary = commitCanvasGraphDeleteIntent(store, {
      nodeIds: [imageId],
      shouldRemoveConnectedEdges: true,
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
