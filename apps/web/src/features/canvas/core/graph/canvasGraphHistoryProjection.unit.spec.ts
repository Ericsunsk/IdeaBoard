import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import {
  createChangeSet,
  createCommitPatch,
  normalizeTransactionChanges,
} from './canvasGraphPatchAlgebra'
import { createSummary } from './canvasGraphHistoryProjection'
import { stageAddNode, stageSelectNode, stageUpdateNode } from './canvasGraphMutation'
import { createStateFromSnapshot } from './canvasGraphStoreState'
import { toCanvasGraphNodeId } from './canvasGraphStoreIds'
import type {
  CanvasGraphStoreState,
  CanvasGraphTransactionDraft,
  CanvasGraphTransactionOptions,
} from './canvasGraphStoreTypes'

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

function createSampleSnapshot(): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = [createImageNode(1, 40, 80)]
  snapshot.promptNodes = [createPromptNode(1, 420, 80)]
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

const RECORD_HISTORY: Required<CanvasGraphTransactionOptions> = {
  recordHistory: true,
  markDirty: true,
}

describe('canvasGraphHistoryProjection', () => {
  it('builds history, dirty, and projection facts for domain changes', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    stageUpdateNode(draft, promptId, { x: 560 })
    const changes = normalizeTransactionChanges(draft)
    // Mirror the real commit ordering: the commit patch is built from the
    // pre-commit draft, then the summary projects from post-commit state.
    const commitPatch = createCommitPatch(draft)

    // Simulate the post-commit read: the summary projects from current state.
    base.nodesById.get(promptId)!.snapshot.x = 560
    const summary = createSummary('node-update', RECORD_HISTORY, base, changes, draft, commitPatch)

    expect(summary.id).toMatch(/^canvas-graph-tx-\d+$/)
    expect(summary).toMatchObject({
      reason: 'node-update',
      didChange: true,
      hasDomainChanges: true,
      changedNodeIds: [promptId],
      shouldRecordHistory: true,
      shouldMarkDirty: true,
    })
    expect(summary.projectionPatch.nodes.updated[0]?.snapshot).toMatchObject({ x: 560 })
    expect(summary.commitPatch.nodes.records).toHaveLength(1)
  })

  it('issues unique summary ids per transaction', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const first = createSummary('node-update', RECORD_HISTORY, base, createChangeSet(), draft)
    const second = createSummary('node-update', RECORD_HISTORY, base, createChangeSet(), draft)
    expect(first.id).not.toBe(second.id)
  })

  it('tracks selection without history or dirty intent', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const promptId = toCanvasGraphNodeId('prompt', 1)
    stageSelectNode(draft, promptId, true)
    const changes = normalizeTransactionChanges(draft)
    const commitPatch = createCommitPatch(draft)
    base.nodesById.get(promptId)!.selected = true

    const summary = createSummary('node-update', RECORD_HISTORY, base, changes, draft, commitPatch)

    expect(summary).toMatchObject({
      didChange: true,
      hasDomainChanges: false,
      shouldRecordHistory: false,
      shouldMarkDirty: false,
      selectedNodeIds: [promptId],
    })
    expect(summary.projectionPatch.nodes.selectionChanged).toEqual([
      { id: promptId, selected: true },
    ])
  })

  it('reports empty summaries for no-op drafts', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const summary = createSummary(
      'node-update',
      RECORD_HISTORY,
      base,
      normalizeTransactionChanges(draft),
      draft,
    )

    expect(summary).toMatchObject({
      didChange: false,
      hasDomainChanges: false,
      shouldRecordHistory: false,
      shouldMarkDirty: false,
    })
    expect(summary.projectionPatch).toEqual({
      nodes: { added: [], updated: [], removedIds: [], selectionChanged: [] },
      edges: { added: [], removedIds: [], selectionChanged: [] },
      groups: { added: [], updated: [], removedIds: [] },
    })
  })

  it('honors recordHistory and markDirty options independently', () => {
    const base = createStateFromSnapshot(createSampleSnapshot())
    const draft = createDraft(base)
    const added = stageAddNode(draft, createPromptNode(9, 100, 100))
    const changes = normalizeTransactionChanges(draft)

    const materialized = createSummary(
      'result-materialize',
      { recordHistory: false, markDirty: true },
      base,
      changes,
      draft,
    )
    expect(materialized.shouldRecordHistory).toBe(false)
    expect(materialized.shouldMarkDirty).toBe(true)

    const silent = createSummary(
      'node-update',
      { recordHistory: false, markDirty: false },
      base,
      changes,
      draft,
    )
    expect(silent.shouldRecordHistory).toBe(false)
    expect(silent.shouldMarkDirty).toBe(false)
    expect(silent.changedNodeIds).toEqual([added])
  })
})
