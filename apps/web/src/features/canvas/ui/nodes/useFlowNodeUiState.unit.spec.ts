import { describe, expect, it } from 'vitest'
import { ref, shallowRef, triggerRef } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { FusionNodeItem, ImageNodeItem } from '../../../board'
import type { FlowNodeData } from '../../core/nodes/flowTypes.ts'
import { useFlowNodeUiState } from './useFlowNodeUiState'

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeFusionSnapshot(overrides: Partial<FusionNodeItem> = {}): FusionNodeItem {
  return {
    id: 2,
    type: 'fusion',
    x: 320,
    y: 0,
    width: 286,
    title: 'Fusion',
    prompt: '',
    outputCount: 1,
    aspectRatio: '1:1',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function makeFlowNode(
  id: string,
  kind: FlowNodeData['kind'],
  snapshotNode: ImageNodeItem | FusionNodeItem,
): Node<FlowNodeData> {
  return {
    id,
    type: 'canvas',
    position: { x: snapshotNode.x, y: snapshotNode.y },
    data: {
      kind,
      nodeType: kind,
      title: snapshotNode.title || kind,
      snapshotNode,
    },
  }
}

describe('useFlowNodeUiState', () => {
  it('treats an image node latest output as ready when its source url is empty', () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode(
        'image-1',
        'image',
        makeImageSnapshot({
          sourceImageUrl: '',
          outputStatus: 'ready',
          outputImages: [{ url: 'https://example.com/generated.png', ratio: 1.4 }],
        }),
      ),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes,
      edges,
      isNodeExecutionActive: () => false,
    })
    const fusionState = getFlowNodeUiStateCached('fusion-1')
    const pendingInputs = fusionState.connection.pending as
      Record<string, boolean | undefined> | undefined

    expect(pendingInputs?.image).toBe(false)
    expect(fusionState.primaryAction).toEqual({ label: 'Generate', disabled: false })
  })

  it('marks a multi-image input as pending when any connected image is still running', () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode(
        'image-ready',
        'image',
        makeImageSnapshot({
          id: 1,
          sourceImageUrl: 'https://example.com/ready.png',
          outputStatus: 'ready',
        }),
      ),
      makeFlowNode(
        'image-running',
        'image',
        makeImageSnapshot({
          id: 2,
          sourceImageUrl: 'https://example.com/running.png',
          outputStatus: 'running',
        }),
      ),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-ready',
        source: 'image-ready',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
      {
        id: 'edge-running',
        source: 'image-running',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes,
      edges,
      isNodeExecutionActive: (nodeId) => nodeId === 'image-running',
    })
    const fusionState = getFlowNodeUiStateCached('fusion-1')
    const pendingInputs = fusionState.connection.pending as
      Record<string, boolean | undefined> | undefined

    expect(pendingInputs?.image).toBe(true)
    expect(fusionState.primaryAction).toEqual({ label: 'Waiting for Image', disabled: true })
  })

  it('preserves ui state identity across position-only node replacements', () => {
    const imageSnapshot = makeImageSnapshot()
    const fusionSnapshot = makeFusionSnapshot()
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', imageSnapshot),
      makeFlowNode('fusion-1', 'fusion', fusionSnapshot),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes,
      edges,
      isNodeExecutionActive: () => false,
    })
    const imageState = getFlowNodeUiStateCached('image-1')
    const fusionState = getFlowNodeUiStateCached('fusion-1')

    nodes.value = nodes.value.map((node) =>
      node.id === 'fusion-1'
        ? {
            ...node,
            position: { x: node.position.x + 80, y: node.position.y + 40 },
          }
        : node,
    )

    expect(getFlowNodeUiStateCached('image-1')).toBe(imageState)
    expect(getFlowNodeUiStateCached('fusion-1')).toBe(fusionState)
  })

  it('returns a new ui state when a relevant connection status changes', () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode(
        'image-running',
        'image',
        makeImageSnapshot({
          sourceImageUrl: 'https://example.com/running.png',
          outputStatus: 'running',
        }),
      ),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes,
      edges,
      isNodeExecutionActive: (nodeId) => nodeId === 'image-running',
    })
    const disconnectedFusionState = getFlowNodeUiStateCached('fusion-1')

    edges.value = [
      {
        id: 'edge-running',
        source: 'image-running',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[]

    const connectedFusionState = getFlowNodeUiStateCached('fusion-1')
    const connectedInputs = connectedFusionState.connection.connected as
      Record<string, boolean | undefined> | undefined
    const pendingInputs = connectedFusionState.connection.pending as
      Record<string, boolean | undefined> | undefined

    expect(connectedFusionState).not.toBe(disconnectedFusionState)
    expect(connectedInputs?.image).toBe(true)
    expect(pendingInputs?.image).toBe(true)
    expect(connectedFusionState.primaryAction).toEqual({
      label: 'Waiting for Image',
      disabled: true,
    })
  })

  it('refreshes connection state when edges update with the same array identity', () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([] as Edge[])

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes,
      edges,
      isNodeExecutionActive: () => false,
    })
    const disconnectedFusionState = getFlowNodeUiStateCached('fusion-1')

    edges.value.push({
      id: 'edge-1',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'fusion-1',
      targetHandle: 'in:image',
    } as Edge)
    triggerRef(edges)

    const connectedFusionState = getFlowNodeUiStateCached('fusion-1')
    const connectedInputs = connectedFusionState.connection.connected as
      Record<string, boolean | undefined> | undefined

    expect(connectedFusionState).not.toBe(disconnectedFusionState)
    expect(connectedInputs?.image).toBe(true)
  })

  it('flips to running when only the execution epoch changes', () => {
    const epochNodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode('fusion-1', 'fusion', makeFusionSnapshot()),
    ] as Node[])
    const epochEdges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])
    const activeNodeIds = new Set<string>()
    const executionEpoch = ref(0)

    const { getFlowNodeUiStateCached } = useFlowNodeUiState({
      nodes: epochNodes,
      edges: epochEdges,
      isNodeExecutionActive: (nodeId) => activeNodeIds.has(nodeId),
      executionEpoch,
    })
    const readyState = getFlowNodeUiStateCached('fusion-1')
    expect(readyState.isRunning).toBe(false)
    expect(readyState.primaryAction).toEqual({ label: 'Generate', disabled: false })

    // Start execution without touching the graph: the epoch bump alone must
    // invalidate the cache so the running label appears immediately.
    activeNodeIds.add('fusion-1')
    executionEpoch.value += 1

    const runningState = getFlowNodeUiStateCached('fusion-1')
    expect(runningState).not.toBe(readyState)
    expect(runningState.isRunning).toBe(true)
    expect(runningState.primaryAction?.disabled).toBe(true)
  })
})
