import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '..'
import { useCanvasGroupRuntime } from './useCanvasGroupRuntime'
import type { CanvasVueFlowGraphAdapter } from '../../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'

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

function createSnapshot(): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
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

function createHarness(options: { isGraphSessionAttached?: () => boolean } = {}) {
  const nodes = shallowRef<SelectableHarnessNode[]>([
    createPromptNode(1, 40, 40, true),
    createPromptNode(2, 400, 40, true),
  ])
  const edges = shallowRef<Edge[]>([])
  const boardBaseSnapshot = ref(createSnapshot())
  const graphSession = CanvasGraphSession.hydrate('board-a', boardBaseSnapshot.value)
  graphSession.subscribe((event) => {
    if (event.patch.groups.records.length === 0) return
    boardBaseSnapshot.value = {
      ...boardBaseSnapshot.value,
      groups: graphSession.store.getGroupIds().flatMap((id) => {
        const group = graphSession.store.getGroup(id)
        return group ? [group] : []
      }),
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
    readCanvasGraphStore: () => graphSession.store,
    isGraphSessionAttached: options.isGraphSessionAttached ?? (() => true),
    onWarning: vi.fn(),
    captureActiveNodeExecutions: vi.fn(() => []),
    releaseNodeExecutions: vi.fn(),
  })

  return {
    runtime,
    nodes,
    boardBaseSnapshot,
  }
}

describe('useCanvasGroupRuntime', () => {
  it('keeps the selection action available while a board attachment is changing', () => {
    const harness = createHarness({ isGraphSessionAttached: () => false })

    expect(harness.runtime.view.selectionGroupAction.value).toEqual({
      kind: 'group',
      label: 'Group',
      nodeIds: ['prompt-1', 'prompt-2'],
    })
    expect(harness.runtime.commands.groupSelectedNodes()).toBe(false)
  })

  it('owns grouping commands through injected graph and side-effect adapters', () => {
    const harness = createHarness()

    expect(Object.keys(harness.runtime).sort()).toEqual(['commands', 'internal', 'view'])
    expect(Object.keys(harness.runtime.view).sort()).toEqual([
      'groupFrames',
      'selectionGroupAction',
    ])
    expect(harness.runtime.view.selectionGroupAction.value).toEqual({
      kind: 'group',
      label: 'Group',
      nodeIds: ['prompt-1', 'prompt-2'],
    })

    expect(harness.runtime.commands.groupSelectedNodes()).toBe(true)

    expect(harness.boardBaseSnapshot.value.groups).toEqual([
      {
        id: 'group-1',
        nodeIds: ['prompt-1', 'prompt-2'],
        x: 30,
        y: 30,
        width: 660,
        height: 322,
      },
    ])
    expect(harness.runtime.internal.selectedGroupId.value).toBe('group-1')
    expect(harness.runtime.view.groupFrames.value[0]).toMatchObject({
      id: 'group-1',
      selected: true,
    })
    expect(harness.nodes.value.map((node) => node.selected)).toEqual([false, false])
  })
})
