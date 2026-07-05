import type { Edge, EdgeUpdateEvent, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import type {
  CanvasCreatableNodeKind,
  FusionNodeItem,
  ImageNodeItem,
  PromptNodeItem,
} from '../../canvas-board/domain'
import { FLOW_NODE_DRAG_HANDLE } from '../utils/flowNodeDragHandle'
import { useCanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { useFlowNodeMutations } from './useFlowNodeMutations'

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

function makeEdge(
  id: string,
  source: string,
  target: string,
  sourceHandle = 'out:image',
  targetHandle = 'in:image',
): Edge {
  return { id, source, target, sourceHandle, targetHandle }
}

function createMutationsHarness(initialNodes: Node[], initialEdges: Edge[]) {
  const nodes = shallowRef<Node[]>(initialNodes)
  const edges = shallowRef<Edge[]>(initialEdges)
  const graphActions = {
    addNodes: vi.fn(),
    setNodes: vi.fn(),
    removeNodes: vi.fn(),
    addEdges: vi.fn(),
    setEdges: vi.fn(),
  }
  const graphCommand = vi.fn()
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    actions: graphActions,
    afterCommand: graphCommand,
  })
  const onWarning = vi.fn()

  const mutations = useFlowNodeMutations({
    nodes,
    edges,
    createNodeKind: shallowRef<CanvasCreatableNodeKind>('prompt'),
    graphAdapter,
    onWarning,
  })

  return { nodes, edges, graphActions, graphCommand, onWarning, mutations }
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
    const { nodes, graphActions, graphCommand, mutations } = createMutationsHarness(
      [
        makeFlowNode('prompt-1', 'prompt', makePromptSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(2)),
      ],
      [makeEdge('e-1', 'prompt-1', 'fusion-1'), makeEdge('e-2', 'fusion-1', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('prompt-1')

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('stale')
    expect(graphActions.setNodes).toHaveBeenCalledTimes(1)
    expect(graphCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'replaceNodes', didChange: true }),
      undefined,
    )
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
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
        makeFlowNode('image-2', 'image', makeImageSnapshot(2)),
        makeFlowNode('fusion-2', 'fusion', makeFusionSnapshot(2)),
      ],
      [makeEdge('e-1', 'image-1', 'fusion-1'), makeEdge('e-2', 'image-2', 'fusion-2')],
    )

    mutations.markFlowExecutionNodeDirty('image-1')

    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(getOutputStatus(nodes.value, 'fusion-2')).toBe('ready')
  })

  it('adds canvas nodes through the graph adapter while preserving flow node data', () => {
    const { nodes, graphActions, graphCommand, mutations } = createMutationsHarness([], [])

    const nodeId = mutations.createNodeAtPosition('prompt', 24, 48, {
      id: 42,
      title: 'Prompt Draft',
      text: 'describe it',
    })

    expect(nodeId).toBe('prompt-42')
    expect(nodes.value).toHaveLength(1)
    expect(nodes.value[0]).toMatchObject({
      id: 'prompt-42',
      type: 'canvas',
      dragHandle: FLOW_NODE_DRAG_HANDLE,
      position: { x: 24, y: 48 },
      data: {
        kind: 'prompt',
        nodeType: 'prompt',
        title: 'Prompt Draft',
        snapshotNode: {
          id: 42,
          type: 'prompt',
          text: 'describe it',
        },
      },
    })
    expect(graphActions.addNodes).toHaveBeenCalledWith(expect.objectContaining({ id: 'prompt-42' }))
    expect(graphCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'addNodes', didChange: true }),
      undefined,
    )
  })

  it('updates snapshot node data through the graph adapter without replacing flow chrome', () => {
    const promptNode = {
      ...makeFlowNode('prompt-1', 'prompt', makePromptSnapshot(1)),
      dragHandle: FLOW_NODE_DRAG_HANDLE,
      position: { x: 12, y: 16 },
    }
    const { nodes, graphCommand, mutations } = createMutationsHarness([promptNode], [])

    mutations.updateSnapshotNode('prompt-1', (snapshotNode) => {
      if (snapshotNode.type !== 'prompt') return snapshotNode
      return { ...snapshotNode, title: 'Renamed Prompt', text: 'new text' }
    })

    expect(nodes.value[0]).toMatchObject({
      id: 'prompt-1',
      dragHandle: FLOW_NODE_DRAG_HANDLE,
      position: { x: 12, y: 16 },
      data: {
        title: 'Renamed Prompt',
        snapshotNode: {
          title: 'Renamed Prompt',
          text: 'new text',
        },
      },
    })
    expect(graphCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'updateNode', didChange: true }),
      undefined,
    )
  })

  it('deletes a canvas node and connected edges through the graph adapter', () => {
    const { nodes, edges, graphActions, graphCommand, mutations } = createMutationsHarness(
      [
        makeFlowNode('prompt-1', 'prompt', makePromptSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1, 'idle')),
      ],
      [makeEdge('e-1', 'prompt-1', 'fusion-1')],
    )

    mutations.onFlowDeleteNode('prompt-1')

    expect(nodes.value.map((node) => node.id)).toEqual(['fusion-1'])
    expect(edges.value).toHaveLength(0)
    expect(graphActions.removeNodes).toHaveBeenCalledWith(['prompt-1'], true, undefined)
    expect(graphCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'removeNodes', didChange: true }),
      undefined,
    )
  })

  it('creates valid connections through the graph adapter and marks ready targets stale', () => {
    const { nodes, edges, graphActions, graphCommand, mutations } = createMutationsHarness(
      [
        makeFlowNode('image-1', 'image', makeImageSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
      ],
      [],
    )

    mutations.onConnect({
      source: 'image-1',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(edges.value).toHaveLength(1)
    expect(edges.value[0]).toMatchObject({
      source: 'image-1',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
      animated: false,
    })
    expect(graphActions.addEdges).toHaveBeenCalledWith(
      expect.objectContaining({ source: 'image-1' }),
    )
    expect(getOutputStatus(nodes.value, 'fusion-1')).toBe('stale')
    expect(graphCommand.mock.calls.map((call) => call[0].command)).toEqual([
      'addEdges',
      'replaceNodes',
    ])
  })

  it('creates connections with canonical connection edge ids', () => {
    const { edges, graphActions, mutations } = createMutationsHarness(
      [
        makeFlowNode('image-1', 'image', makeImageSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1)),
      ],
      [],
    )

    mutations.onConnect({
      source: 'image-1',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(edges.value[0]).toMatchObject({
      id: 'connection-1',
      source: 'image-1',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
      data: {
        connectionSnapshot: {
          id: 1,
          fromNodeType: 'image',
          fromNodeId: 1,
          fromPort: 'image',
          toNodeType: 'fusion',
          toNodeId: 1,
          toPort: 'image',
        },
      },
    })
    expect(graphActions.addEdges).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'connection-1' }),
    )
  })

  it('keeps single-input connections last-write-wins', () => {
    const { edges, graphActions, mutations } = createMutationsHarness(
      [
        makeFlowNode('prompt-1', 'prompt', makePromptSnapshot(1)),
        makeFlowNode('prompt-2', 'prompt', makePromptSnapshot(2)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1, 'idle')),
      ],
      [makeEdge('connection-1', 'prompt-1', 'fusion-1', 'out:prompt', 'in:prompt')],
    )

    mutations.onConnect({
      source: 'prompt-2',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
    })

    expect(edges.value).toHaveLength(1)
    expect(edges.value[0]).toMatchObject({
      id: 'connection-2',
      source: 'prompt-2',
      target: 'fusion-1',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
    })
    expect(graphActions.setEdges).toHaveBeenCalledOnce()
  })

  it('rejects multi-input connections over the target limit', () => {
    const imageNodes = [1, 2, 3, 4, 5].map((id) =>
      makeFlowNode(`image-${id}`, 'image', makeImageSnapshot(id)),
    )
    const { edges, graphActions, onWarning, mutations } = createMutationsHarness(
      [...imageNodes, makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1, 'idle'))],
      [1, 2, 3, 4].map((id) => makeEdge(`connection-${id}`, `image-${id}`, 'fusion-1')),
    )

    mutations.onConnect({
      source: 'image-5',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(edges.value).toHaveLength(4)
    expect(graphActions.addEdges).not.toHaveBeenCalled()
    expect(graphActions.setEdges).not.toHaveBeenCalled()
    expect(onWarning).toHaveBeenCalledWith('Fusion 最多接入 4 张图片')
  })

  it('rejects invalid connections before graph adapter edge commands run', () => {
    const { edges, graphActions, onWarning, mutations } = createMutationsHarness(
      [
        makeFlowNode('image-1', 'image', makeImageSnapshot(1)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1, 'idle')),
      ],
      [],
    )

    mutations.onConnect({
      source: 'image-1',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:prompt',
    })

    expect(edges.value).toHaveLength(0)
    expect(graphActions.addEdges).not.toHaveBeenCalled()
    expect(graphActions.setEdges).not.toHaveBeenCalled()
    expect(onWarning).toHaveBeenCalledWith('该连线不符合当前连接语义')
  })

  it('updates existing edge connections through the graph adapter', () => {
    const { edges, graphActions, graphCommand, mutations } = createMutationsHarness(
      [
        makeFlowNode('image-1', 'image', makeImageSnapshot(1)),
        makeFlowNode('image-2', 'image', makeImageSnapshot(2)),
        makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot(1, 'idle')),
      ],
      [
        {
          id: 'connection-1',
          source: 'image-1',
          target: 'fusion-1',
          sourceHandle: 'out:image',
          targetHandle: 'in:image',
          data: {
            connectionSnapshot: {
              id: 1,
              fromNodeType: 'image',
              fromNodeId: 1,
              fromPort: 'image',
              toNodeType: 'fusion',
              toNodeId: 1,
              toPort: 'image',
            },
          },
        },
      ],
    )

    mutations.onEdgeUpdate({
      edge: edges.value[0]!,
      connection: {
        source: 'image-2',
        target: 'fusion-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      },
    } as EdgeUpdateEvent)

    expect(edges.value[0]).toMatchObject({
      id: 'connection-1',
      source: 'image-2',
      target: 'fusion-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })
    expect(graphActions.setEdges).toHaveBeenCalledOnce()
    expect(graphCommand).toHaveBeenCalledWith(
      expect.objectContaining({ command: 'updateEdge', didChange: true }),
      undefined,
    )
  })
})
