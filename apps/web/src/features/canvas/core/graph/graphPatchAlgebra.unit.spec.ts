import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasConnection,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import {
  applyCommitPatchOrder,
  canApplyCommitPatch,
  createChangeSet,
  createCommitPatch,
  hasVisibleChanges,
  isSameJsonValue,
  normalizeTransactionChanges,
  recordAppliedOrderPatch,
  selectCommitPatchRecords,
} from './graphPatchAlgebra'
import { cloneNodeRecord, createStateFromSnapshot } from './graphStoreState'
import { toCanvasGraphEdgeId, toCanvasGraphNodeId } from './graphStoreIds'
import {
  stageAddEdge,
  stageAddNode,
  stageRemoveEdge,
  stageRemoveNode,
  stageSelectNode,
  stageUpdateNode,
} from './graphMutation'
import type { CanvasGraphStoreState, CanvasGraphTransactionDraft } from './graphStoreTypes'

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

describe('canvasGraphPatchAlgebra', () => {
  it('compares snapshots structurally', () => {
    expect(isSameJsonValue({ x: 1 }, { x: 1 })).toBe(true)
    expect(isSameJsonValue({ x: 1 }, { x: 2 })).toBe(false)
    expect(isSameJsonValue(undefined, undefined)).toBe(true)
    expect(isSameJsonValue({ x: 1 }, undefined)).toBe(false)
  })

  it('normalizes staged writes into added/updated/removed/isSelected sets', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    const removedId = toCanvasGraphNodeId('prompt', 2)

    stageAddNode(draft, createPromptNode(9, 100, 100))
    stageUpdateNode(draft, promptId, { x: 560 })
    stageRemoveNode(draft, removedId)
    stageSelectNode(draft, promptId, true)

    const changes = normalizeTransactionChanges(draft)
    expect(changes.addedNodeIds.has(toCanvasGraphNodeId('prompt', 9))).toBe(true)
    expect(changes.updatedNodeIds).toEqual(new Set([promptId]))
    expect(changes.removedNodeIds).toEqual(new Set([removedId]))
    expect(changes.selectedNodeIds).toEqual(new Set([promptId]))
    expect(changes.hasDomainChanges).toBe(true)
    expect(changes.didChangeNodeOrder).toBe(true)
    expect(hasVisibleChanges(changes)).toBe(true)
  })

  it('treats restored records as a no-op change set', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)

    stageUpdateNode(draft, promptId, { x: 500 })
    stageUpdateNode(draft, promptId, { x: 420 })
    stageSelectNode(draft, promptId, true)
    stageSelectNode(draft, promptId, false)

    const changes = normalizeTransactionChanges(draft)
    expect(changes.updatedNodeIds.size).toBe(0)
    expect(changes.selectedNodeIds.size).toBe(0)
    expect(changes.hasDomainChanges).toBe(false)
    expect(hasVisibleChanges(changes)).toBe(false)
  })

  it('reports selection-only change sets as visible without domain changes', () => {
    const changes = createChangeSet()
    changes.selectedNodeIds.add(toCanvasGraphNodeId('prompt', 1))
    expect(changes.hasDomainChanges).toBe(false)
    expect(hasVisibleChanges(changes)).toBe(true)
    expect(hasVisibleChanges(createChangeSet())).toBe(false)
  })

  it('round trips commit patches through select in both directions', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    stageUpdateNode(draft, promptId, { x: 560 })

    const patch = createCommitPatch(draft)
    expect(patch.nodes.records).toHaveLength(1)

    const forwardWrites: typeof draft.nodeWrites = new Map()
    selectCommitPatchRecords(patch.nodes, forwardWrites, cloneNodeRecord, 'forward')
    expect(forwardWrites.get(promptId)?.snapshot).toMatchObject({ x: 560 })

    const backwardWrites: typeof draft.nodeWrites = new Map()
    selectCommitPatchRecords(patch.nodes, backwardWrites, cloneNodeRecord, 'backward')
    expect(backwardWrites.get(promptId)?.snapshot).toMatchObject({ x: 420 })

    const removalDraft = createDraft(base)
    stageRemoveNode(removalDraft, promptId)
    const removalPatch = createCommitPatch(removalDraft)
    const removalWrites: typeof draft.nodeWrites = new Map()
    selectCommitPatchRecords(removalPatch.nodes, removalWrites, cloneNodeRecord, 'forward')
    expect(removalWrites.get(promptId)).toBeNull()
  })

  it('applies order patches forward and backward', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const added = stageAddNode(draft, createPromptNode(9, 100, 100))
    const patch = createCommitPatch(draft)

    const forward = applyCommitPatchOrder(base.nodeOrder, patch.nodes.order, 'forward')
    expect(forward).toEqual([...base.nodeOrder, added])

    const backward = applyCommitPatchOrder(forward!, patch.nodes.order, 'backward')
    expect(backward).toEqual(base.nodeOrder)

    expect(applyCommitPatchOrder(base.nodeOrder, [], 'forward')).toBeUndefined()
  })

  it('records order patches only for changed placements', () => {
    const before = [toCanvasGraphNodeId('prompt', 1), toCanvasGraphNodeId('prompt', 2)]
    const unchanged = new Map()
    recordAppliedOrderPatch(before, [...before], unchanged)
    expect(unchanged.size).toBe(0)

    const added = new Map()
    recordAppliedOrderPatch(before, [...before, toCanvasGraphNodeId('prompt', 9)], added)
    // Appending shifts the previous tail's placement, so both the new id and
    // its predecessor carry order patches (needed to restore order on undo).
    expect([...added.keys()]).toEqual([
      toCanvasGraphNodeId('prompt', 2),
      toCanvasGraphNodeId('prompt', 9),
    ])
  })

  it('accepts matching patches and rejects diverged state', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    stageUpdateNode(draft, promptId, { x: 560 })
    const patch = createCommitPatch(draft)

    expect(canApplyCommitPatch(base, patch, 'forward')).toBe(true)

    const diverged = createStateFromSnapshot(createSampleSnapshot())
    diverged.nodesById.get(promptId)!.snapshot.x = 999
    expect(canApplyCommitPatch(diverged, patch, 'forward')).toBe(false)
  })

  it('detects edge topology changes as remove plus add', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const edgeId = toCanvasGraphEdgeId(1)
    stageRemoveEdge(draft, edgeId)
    stageAddEdge(draft, createConnection(2, 1, 2))

    const changes = normalizeTransactionChanges(draft)
    expect(changes.removedEdgeIds.has(edgeId)).toBe(true)
    expect(changes.addedEdgeIds.has(toCanvasGraphEdgeId(2))).toBe(true)
    expect(changes.didChangeEdgeOrder).toBe(true)
  })
})
