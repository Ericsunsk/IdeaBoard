import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import { createCanvasGraphStore } from '../graph-runtime'
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import type { FlowHistorySnapshot } from './useFlowHistory'

type SelectableHarnessNode = Node & {
  selected?: boolean
}

function createPromptNode(
  id: number,
  x: number,
  y: number,
  selected = false,
): SelectableHarnessNode {
  return {
    id: `prompt-${id}`,
    type: 'canvas',
    position: { x, y },
    selected,
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

function createSnapshot(): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.promptNodes = [
    {
      id: 1,
      type: 'prompt',
      x: 40,
      y: 40,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
    {
      id: 2,
      type: 'prompt',
      x: 400,
      y: 40,
      width: 280,
      text: '',
      outputCount: 1,
      aspectRatio: 'auto',
      inputImages: [],
    },
  ]
  return snapshot
}

function createHarness() {
  const nodes = shallowRef<SelectableHarnessNode[]>([
    createPromptNode(1, 40, 40, true),
    createPromptNode(2, 400, 40, true),
  ])
  const edges = shallowRef<Edge[]>([])
  const boardBaseSnapshot = ref(createSnapshot())
  const historyBefore = {} as FlowHistorySnapshot
  const markDirty = vi.fn()
  const commitGroups = vi.fn((groups: CanvasBoardSnapshot['groups']) => {
    boardBaseSnapshot.value = {
      ...boardBaseSnapshot.value,
      groups,
    }
  })
  const graphAdapter = {
    replaceGraph: vi.fn((graph: { nodes: Node[]; edges: Edge[] }) => {
      nodes.value = graph.nodes as SelectableHarnessNode[]
      edges.value = graph.edges
      return { didChange: true }
    }),
    replaceNodes: vi.fn((nextNodes: Node[]) => {
      nodes.value = nextNodes as SelectableHarnessNode[]
      return { didChange: true }
    }),
    applyNodeChanges: vi.fn(),
    applyGraphChanges: vi.fn(),
  } as unknown as CanvasVueFlowGraphAdapter

  const runtime = useCanvasGroupRuntime({
    nodes,
    edges,
    boardBaseSnapshot,
    graphAdapter,
    flowHistory: {
      capture: vi.fn(() => historyBefore),
      push: vi.fn(() => true),
    },
    createCurrentCanvasGraphStore: () => createCanvasGraphStore(boardBaseSnapshot.value),
    commitGroups,
    markDirty,
    onWarning: vi.fn(),
  })

  return {
    runtime,
    nodes,
    boardBaseSnapshot,
    commitGroups,
    markDirty,
  }
}

describe('useCanvasGroupRuntime', () => {
  it('owns grouping commands through injected graph and side-effect adapters', () => {
    const harness = createHarness()

    expect(Object.keys(harness.runtime).sort()).toEqual(['commands', 'internal', 'view'])
    expect(Object.keys(harness.runtime.view).sort()).toEqual([
      'groupFrames',
      'selectionGroupAction',
    ])
    expect(harness.runtime.view.selectionGroupAction.value).toEqual({
      kind: 'group',
      label: '组合',
      nodeIds: ['prompt-1', 'prompt-2'],
    })

    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)

    expect(harness.commitGroups).toHaveBeenCalledOnce()
    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      {
        id: 'group-1',
        nodeIds: ['prompt-1', 'prompt-2'],
        x: 30,
        y: 30,
        width: 660,
        height: 300,
      },
    ])
    expect(harness.runtime.internal.selectedGroupId.value).toBe('group-1')
    expect(harness.runtime.view.groupFrames.value[0]).toMatchObject({
      id: 'group-1',
      selected: true,
    })
    expect(harness.nodes.value.map((node) => node.selected)).toEqual([false, false])
    expect(harness.markDirty).toHaveBeenCalledOnce()
  })
})
