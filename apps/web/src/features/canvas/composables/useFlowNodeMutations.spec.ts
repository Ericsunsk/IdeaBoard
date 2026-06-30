import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import type {
  CanvasCreatableNodeKind,
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { useFlowNodeMutations } from './useFlowNodeMutations'
import type { FlowHistorySnapshot } from './useFlowTransactions'

function makePromptSnapshot(id: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    text: '',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function makeImageSnapshot(id: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'idle',
    outputImages: [],
  }
}

function makeFusionSnapshot(
  id: number,
  outputStatus: FusionNodeItem['outputStatus'] = 'ready',
): FusionNodeItem {
  return {
    id,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 280,
    prompt: '',
    outputCount: 1,
    aspectRatio: 'auto',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus,
    outputImages: [],
  }
}

function makeFlowNode(id: string, kind: 'image', snapshotNode: ImageNodeItem): Node
function makeFlowNode(id: string, kind: 'prompt', snapshotNode: PromptNodeItem): Node
function makeFlowNode(id: string, kind: 'fusion', snapshotNode: FusionNodeItem): Node
function makeFlowNode(
  id: string,
  kind: 'image' | 'prompt' | 'fusion',
  snapshotNode: ImageNodeItem | PromptNodeItem | FusionNodeItem,
): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind,
      nodeType: kind,
      title: id,
      snapshotNode,
    },
  }
}

function makeEdge(id: string, source: string, target: string): Edge {
  return { id, source, target }
}

function createMutationsHarness(initialNodes: Node[], initialEdges: Edge[]) {
  const nodes = shallowRef<Node[]>(initialNodes)
  const edges = shallowRef<Edge[]>(initialEdges)
  const commitNodeMutation = vi.fn(
    (
      mutator: (currentNodes: Node[]) => Node[],
      _options?: {
        recordHistory?: boolean
        historyBefore?: FlowHistorySnapshot
        markDirty?: boolean
      },
    ) => {
      nodes.value = mutator(nodes.value)
    },
  )

  const mutations = useFlowNodeMutations({
    nodes,
    edges,
    createNodeKind: shallowRef<CanvasCreatableNodeKind>('prompt'),
    commitGraphMutation: vi.fn(),
    commitNodeMutation,
    commitEdgeMutation: vi.fn(),
    onWarning: vi.fn(),
  })

  return { nodes, edges, commitNodeMutation, mutations }
}

function getOutputStatus(nodes: Node[], nodeId: string) {
  const snapshotNode = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshotNode || typeof snapshotNode !== 'object' || !('outputStatus' in snapshotNode)) {
    return null
  }
  return snapshotNode.outputStatus
}

describe('useFlowNodeMutations', () => {
  it('marks ready downstream fusion nodes stale in one node mutation', () => {
    const { nodes, commitNodeMutation, mutations } = createMutationsHarness(
      [
        makeFlowNode('prompt-1', 'prompt', makePromptSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(2)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(3)),
      ],
      [makeEdge('e-1', 'prompt-1', 'fusion-1'), makeEdge('e-2', 'fusion-1', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('prompt-1')

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('stale')
    expect(commitNodeMutation).toHaveBeenCalledTimes(1)
    expect(commitNodeMutation).toHaveBeenLastCalledWith(expect.any(Function), {
      recordHistory: false,
    })
  })

  it('keeps self ready when includeSelf is false', () => {
    const { nodes, mutations } = createMutationsHarness(
      [
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(2)),
      ],
      [makeEdge('e-1', 'fusion-1', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('fusion-1', { includeSelf: false })

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('ready')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('stale')
  })

  it('marks self stale when includeSelf is true', () => {
    const { nodes, mutations } = createMutationsHarness(
      [
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(2)),
      ],
      [makeEdge('e-1', 'fusion-1', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('fusion-1', { includeSelf: true })

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('stale')
  })

  it('does not mark unrelated branches stale', () => {
    const { nodes, mutations } = createMutationsHarness(
      [
        makeFlowNode('image-1', 'image', makeImageSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(2)),
        makeFlowNode('image-2', 'image', makeImageSnapshot(3)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(4)),
      ],
      [makeEdge('e-1', 'image-1', 'fusion-1'), makeEdge('e-2', 'image-2', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('image-1')

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('ready')
  })
})
