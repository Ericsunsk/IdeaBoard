import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '../../../core/graph'
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import type { CanvasVueFlowGraphAdapter } from '../../../session/flow-adapter'

type SelectableHarnessNode = Node & Partial<Pick<import('@vue-flow/core').GraphNode, 'selected'>>

function createPromptFlowNode(
  id: number,
  x: number,
  y: number,
  isSelected = false,
): SelectableHarnessNode {
  return {
    id: `prompt-${id}`,
    type: 'canvas',
    position: { x, y },
    selected: isSelected,
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      snapshotNode: {
        id,
        type: 'prompt',
        x,
        y,
        width: 280,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    },
  }
}

function createSnapshot(
  nodeDefs: Array<{ id: number; x: number; y: number }> = [
    { id: 1, x: 40, y: 40 },
    { id: 2, x: 400, y: 40 },
  ],
): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.promptNodes = nodeDefs.map(({ id, x, y }) => ({
    id,
    type: 'prompt',
    x,
    y,
    width: 280,
    text: '',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }))
  return snapshot
}

function createHarness(
  nodeDefs: Array<{ id: number; x: number; y: number; isSelected?: boolean }> = [
    { id: 1, x: 40, y: 40, isSelected: true },
    { id: 2, x: 400, y: 40, isSelected: true },
  ],
) {
  const nodes = shallowRef<SelectableHarnessNode[]>(
    nodeDefs.map(({ id, x, y, isSelected }) => createPromptFlowNode(id, x, y, isSelected ?? false)),
  )
  const edges = shallowRef<Edge[]>([])
  const boardBaseSnapshot = ref(createSnapshot(nodeDefs))
  const graphSession = CanvasGraphSession.hydrate('board-a', boardBaseSnapshot.value)
  // Simulate post-reconcile state: the harness presets Flow selection, and in
  // production reconcile would already have mirrored it into the store.
  {
    const initial = graphSession.store.beginTransaction('node-update', {
      shouldRecordHistory: false,
      shouldMarkDirty: false,
    })
    for (const def of nodeDefs) {
      if (def.isSelected) initial.selectNode(`prompt-${def.id}`, true)
    }
    initial.commit()
  }
  const commits: Array<{ reason: string; shouldRecordHistory: boolean; shouldMarkDirty: boolean }> =
    []
  let historyCount = 0
  let autosaveCount = 0
  graphSession.subscribe((event) => {
    commits.push({
      reason: event.summary.reason,
      shouldRecordHistory: event.summary.shouldRecordHistory,
      shouldMarkDirty: event.summary.shouldMarkDirty,
    })
    if (event.sideEffects.shouldRecordHistory) historyCount += 1
    if (event.sideEffects.shouldMarkDirty) autosaveCount += 1
  })
  const syncGroups = () => {
    boardBaseSnapshot.value = {
      ...boardBaseSnapshot.value,
      groups: graphSession.store.getGroupIds().flatMap((id) => {
        const group = graphSession.store.getGroup(id)
        return group ? [group] : []
      }),
    }
  }
  graphSession.subscribe((event) => {
    if (event.patch.groups.records.length > 0) syncGroups()
  })
  // Mirrors the production commit projection for selection: group intents now
  // write selection to the store, and Flow is updated from the commit patch.
  graphSession.subscribe((event) => {
    for (const record of event.patch.nodes.records) {
      if (!record.after) continue
      const isSelected = record.after.isSelected
      nodes.value = nodes.value.map((node) =>
        node.id === record.id ? { ...node, selected: isSelected } : node,
      ) as SelectableHarnessNode[]
    }
    for (const record of event.patch.edges.records) {
      if (!record.after) continue
      const isSelected = record.after.isSelected
      edges.value = edges.value.map((edge) =>
        edge.id === record.id
          ? ({ ...edge, selected: isSelected } satisfies Partial<
              import('@vue-flow/core').GraphEdge
            >)
          : edge,
      )
    }
  })
  const graphAdapter = {
    replaceGraph: vi.fn((graph: { nodes: Node[]; edges: Edge[] }) => {
      nodes.value = graph.nodes as SelectableHarnessNode[]
      edges.value = graph.edges
      return { didChange: true }
    }),
    applyNodeChanges: vi.fn(),
  } as unknown as CanvasVueFlowGraphAdapter

  const runtime = useCanvasGroupRuntime({
    nodes,
    edges,
    boardBaseSnapshot,
    graphAdapter,
    readCanvasGraphStore: () => graphSession.store,
    isGraphSessionAttached: () => true,
    onWarning: vi.fn(),
    captureActiveNodeExecutions: vi.fn(() => []),
    releaseNodeExecutions: vi.fn(),
  })

  return {
    runtime,
    nodes,
    edges,
    boardBaseSnapshot,
    graphSession,
    commits,
    historyCount: () => historyCount,
    autosaveCount: () => autosaveCount,
    resetCounts: () => {
      commits.length = 0
      historyCount = 0
      autosaveCount = 0
    },
    expectSelectionInSync: () => {
      const storeSelectedNodes = new Set(
        graphSession.store
          .getNodeIds()
          .filter((nodeId) => graphSession.store.getNode(nodeId)?.isSelected === true),
      )
      const flowSelectedNodes = new Set(
        nodes.value.filter((node) => node.selected === true).map((node) => node.id),
      )
      expect(flowSelectedNodes).toEqual(storeSelectedNodes)
      const storeSelectedEdges = new Set(
        graphSession.store
          .getEdgeIds()
          .filter((edgeId) => graphSession.store.getEdge(edgeId)?.isSelected === true),
      )
      const flowSelectedEdges = new Set(
        edges.value
          .filter(
            (edge) =>
              (edge as Edge & Partial<Pick<import('@vue-flow/core').GraphEdge, 'selected'>>)
                .selected === true,
          )
          .map((edge) => edge.id),
      )
      expect(flowSelectedEdges).toEqual(storeSelectedEdges)
    },
  }
}

describe('canvas group runtime ownership', () => {
  it('selects a group frame without a transaction, history, or autosave', () => {
    const harness = createHarness()
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    expect(harness.runtime.commands.selectGroupAtPoint({ x: 100, y: 100 })).toBe(true)

    expect(harness.commits).toHaveLength(0)
    expect(harness.historyCount()).toBe(0)
    expect(harness.autosaveCount()).toBe(0)
    expect(harness.runtime.internal.selectedGroupId.value).toBe('group-1')
    harness.expectSelectionInSync()
  })

  it('groups selected nodes with one transaction, history entry, and autosave', () => {
    const harness = createHarness()

    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'group', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    expect(harness.boardBaseSnapshot.value.groups).toHaveLength(1)
    harness.expectSelectionInSync()
  })

  it('ungroups the selected group with one transaction', () => {
    const harness = createHarness()
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    expect(harness.runtime.commands.ungroupSelectedGroup()).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'ungroup', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    expect(harness.boardBaseSnapshot.value.groups).toEqual([])
    harness.expectSelectionInSync()
  })

  it('deletes the selected group and its members with one transaction', () => {
    const harness = createHarness()
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    expect(harness.runtime.commands.deleteSelectedGroup()).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'group-delete', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    expect(harness.graphSession.store.exportSnapshot().promptNodes).toEqual([])
    harness.expectSelectionInSync()
  })

  it('drags a group frame with one group-move transaction', () => {
    const harness = createHarness()
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    expect(harness.runtime.commands.groupDrag.begin({ x: 100, y: 100 })).toBe(true)
    expect(harness.runtime.commands.groupDrag.move({ x: 150, y: 120 })).toBe(true)
    expect(harness.commits).toHaveLength(0)
    expect(harness.runtime.commands.groupDrag.commit()).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'group-move', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    expect(harness.graphSession.store.getNode('prompt-1')?.snapshot).toMatchObject({
      x: 90,
      y: 60,
    })
    harness.expectSelectionInSync()
  })

  it('resizes a group frame with one group-resize transaction', () => {
    const harness = createHarness()
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    expect(harness.runtime.commands.groupResize.begin('group-1', 'se', { x: 690, y: 362 })).toBe(
      true,
    )
    expect(harness.runtime.commands.groupResize.move({ x: 790, y: 402 })).toBe(true)
    expect(harness.commits).toHaveLength(0)
    expect(harness.runtime.commands.groupResize.commit()).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'group-resize', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    harness.expectSelectionInSync()
  })

  it('joins a pending group node with one group-membership transaction', () => {
    const harness = createHarness([
      { id: 1, x: 40, y: 40, isSelected: true },
      { id: 2, x: 400, y: 40, isSelected: true },
      { id: 3, x: 200, y: 120, isSelected: false },
    ])
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    harness.resetCounts()

    harness.runtime.commands.handlePaneClick()
    expect(harness.runtime.commands.handleNodeClick('prompt-3')).toBe(true)

    expect(harness.commits).toEqual([
      { reason: 'group-membership', shouldRecordHistory: true, shouldMarkDirty: true },
    ])
    expect(harness.historyCount()).toBe(1)
    expect(harness.autosaveCount()).toBe(1)
    expect(harness.graphSession.store.getGroupIdForNode('prompt-3')).toBe('group-1')
    harness.expectSelectionInSync()
  })

  it('restores group operations through undo and redo', () => {
    const harness = createHarness()
    const patches: Array<{
      patch: Parameters<Parameters<typeof harness.graphSession.subscribe>[0]>[0]['patch']
    }> = []
    const unsubscribe = harness.graphSession.subscribe((event) => {
      if (event.summary.shouldRecordHistory) patches.push({ patch: event.patch })
    })

    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)
    expect(harness.boardBaseSnapshot.value.groups).toHaveLength(1)
    expect(patches).toHaveLength(1)

    harness.graphSession.store.applyCommitPatch(patches[0]!.patch, 'backward')
    expect(harness.graphSession.store.getGroupIds()).toEqual([])

    harness.graphSession.store.applyCommitPatch(patches[0]!.patch, 'forward')
    expect(harness.graphSession.store.getGroupIds()).toEqual(['group-1'])
    unsubscribe()
  })
})
