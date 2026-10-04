import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasConnection,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import {
  applyNodeUpdate,
  commitEdgeWrites,
  commitGroupWrites,
  commitNodeWrites,
  hasSameGroupRecord,
  isSameGroupGeometry,
  normalizeGroupGeometry,
  stageAddEdge,
  stageCreateGroup,
  stageMoveNodeToGroup,
  stageRemoveNode,
  stageUngroup,
  stageUpdateGroupGeometry,
  stageUpdateNode,
  getDraftConnectedEdgeIds,
  getDraftGroupIdForNode,
} from './canvasGraphMutation'
import { createStateFromSnapshot } from './canvasGraphStoreState'
import {
  toCanvasGraphEdgeId,
  toCanvasGraphGroupId,
  toCanvasGraphNodeId,
} from './canvasGraphStoreIds'
import type { CanvasGraphStoreState, CanvasGraphTransactionDraft } from './canvasGraphStoreTypes'

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
  snapshot.imageNodes = [createImageNode(1, 40, 80)]
  snapshot.promptNodes = [createPromptNode(1, 420, 80), createPromptNode(2, 820, 80)]
  snapshot.connections = [createConnection(1, 1, 1)]
  return snapshot
}

function createDraft(base: CanvasGraphStoreState): CanvasGraphTransactionDraft {
  return {
    base,
    baseRevision: 0,
    nodeWrites: new Map(),
    edgeWrites: new Map(),
    groupWrites: new Map(),
    nodeOrderPatches: new Map(),
    edgeOrderPatches: new Map(),
    groupOrderPatches: new Map(),
  }
}

describe('canvasGraphMutation', () => {
  it('applies node updates without changing identity', () => {
    const node = createPromptNode(1, 420, 80)
    expect(applyNodeUpdate(node, { x: 560 })).toMatchObject({ id: 1, type: 'prompt', x: 560 })
    expect(applyNodeUpdate(node, (current) => ({ ...current, x: 1 }))).toMatchObject({ x: 1 })
    // Identity fields survive even when the update tries to overwrite them.
    expect(applyNodeUpdate(node, { id: 99, type: 'fusion', x: 1 } as never)).toMatchObject({
      id: 1,
      type: 'prompt',
    })
  })

  it('rejects node updates that change identity', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    expect(() =>
      stageUpdateNode(draft, promptId, ((node: PromptNodeItem) => ({ ...node, id: 99 })) as never),
    ).toThrow('cannot change node identity')
  })

  it('validates group geometry', () => {
    expect(normalizeGroupGeometry(GROUP_GEOMETRY)).toEqual(GROUP_GEOMETRY)
    expect(() => normalizeGroupGeometry({ x: 0, y: 0, width: 0, height: 10 })).toThrow(
      'finite and positive',
    )
    expect(() => normalizeGroupGeometry({ x: NaN, y: 0, width: 10, height: 10 })).toThrow(
      'finite and positive',
    )
    expect(isSameGroupGeometry(GROUP_GEOMETRY, { ...GROUP_GEOMETRY })).toBe(true)
    expect(isSameGroupGeometry(GROUP_GEOMETRY, { ...GROUP_GEOMETRY, x: 1 })).toBe(false)
    expect(
      hasSameGroupRecord(
        { id: toCanvasGraphGroupId(1), nodeIds: [], ...GROUP_GEOMETRY },
        { id: toCanvasGraphGroupId(1), nodeIds: [], ...GROUP_GEOMETRY },
      ),
    ).toBe(true)
  })

  it('rejects invalid group staging', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)

    const missingNode = createDraft(base)
    expect(() =>
      stageCreateGroup(missingNode, [imageId, toCanvasGraphNodeId('image', 999)], GROUP_GEOMETRY),
    ).toThrow('group node does not exist')

    const tooFew = createDraft(base)
    expect(() => stageCreateGroup(tooFew, [imageId], GROUP_GEOMETRY)).toThrow('at least two nodes')

    const grouped = createDraft(base)
    stageCreateGroup(grouped, [imageId, promptId], GROUP_GEOMETRY, toCanvasGraphGroupId(1))
    expect(() =>
      stageCreateGroup(grouped, [imageId, toCanvasGraphNodeId('prompt', 2)], GROUP_GEOMETRY),
    ).toThrow('already belongs to group')
  })

  it('rejects edges with missing endpoints', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    expect(() => stageAddEdge(draft, createConnection(9, 1, 999))).toThrow('missing endpoint')
  })

  it('removes connected edges and group membership when staging node removal', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    snapshot.groups = [{ id: groupId, nodeIds: [imageId, promptId], ...GROUP_GEOMETRY }]
    const base = createStateFromSnapshot(snapshot)
    const draft = createDraft(base)

    stageRemoveNode(draft, imageId, { removeConnectedEdges: true })

    expect(draft.nodeWrites.get(imageId)).toBeNull()
    expect(draft.edgeWrites.get(toCanvasGraphEdgeId(1))).toBeNull()
    expect(draft.groupWrites.get(groupId)?.nodeIds).toEqual([promptId])
  })

  it('overlays draft writes on group and topology reads', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const secondPromptId = toCanvasGraphNodeId('prompt', 2)
    snapshot.groups = [{ id: groupId, nodeIds: [imageId, promptId], ...GROUP_GEOMETRY }]
    const base = createStateFromSnapshot(snapshot)
    const draft = createDraft(base)

    stageMoveNodeToGroup(draft, secondPromptId, groupId)
    expect(getDraftGroupIdForNode(draft, secondPromptId)).toBe(groupId)

    stageRemoveNode(draft, imageId, { removeConnectedEdges: true })
    expect(getDraftConnectedEdgeIds(draft, imageId).size).toBe(0)
    expect(getDraftConnectedEdgeIds(draft, promptId).size).toBe(0)
  })

  it('ignores geometry no-ops and unknown ungroups', () => {
    const snapshot = createSampleSnapshot()
    const groupId = toCanvasGraphGroupId(1)
    snapshot.groups = [
      {
        id: groupId,
        nodeIds: [toCanvasGraphNodeId('image', 1), toCanvasGraphNodeId('prompt', 1)],
        ...GROUP_GEOMETRY,
      },
    ]
    const base = createStateFromSnapshot(snapshot)
    const draft = createDraft(base)

    stageUpdateGroupGeometry(draft, groupId, { ...GROUP_GEOMETRY })
    expect(draft.groupWrites.size).toBe(0)

    stageUngroup(draft, toCanvasGraphGroupId(999))
    expect(draft.groupWrites.size).toBe(0)
  })

  it('commits writes while keeping derived indexes current', () => {
    const state = createStateFromSnapshot(createSampleSnapshot())
    const imageId = toCanvasGraphNodeId('image', 1)
    const promptId = toCanvasGraphNodeId('prompt', 2)
    const edgeId = toCanvasGraphEdgeId(2)

    commitNodeWrites(state, new Map([[toCanvasGraphNodeId('prompt', 9), null]]))
    expect(state.nodesById.size).toBe(3)

    commitEdgeWrites(
      state,
      new Map([[edgeId, { id: edgeId, connection: createConnection(2, 1, 2), selected: false }]]),
    )
    expect(state.indexes.outgoingEdgesByNodeId.get(imageId)).toContain(edgeId)
    expect(state.indexes.incomingEdgesByNodeId.get(promptId)).toContain(edgeId)

    commitEdgeWrites(state, new Map([[edgeId, null]]))
    expect(state.indexes.incomingEdgesByNodeId.get(promptId)).toBeUndefined()

    const groupId = toCanvasGraphGroupId(1)
    commitGroupWrites(
      state,
      new Map([[groupId, { id: groupId, nodeIds: [imageId], ...GROUP_GEOMETRY }]]),
    )
    expect(state.indexes.groupIdByNodeId.get(imageId)).toBe(groupId)
    commitGroupWrites(state, new Map([[groupId, null]]))
    expect(state.indexes.groupIdByNodeId.get(imageId)).toBeUndefined()
  })
})
