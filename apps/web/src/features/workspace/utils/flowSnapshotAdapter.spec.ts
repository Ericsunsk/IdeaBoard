import { describe, expect, it } from 'vitest'
import {
  canvasBoardSnapshotToFlow,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from './flowSnapshotAdapter'

function makeImageFlowNode(params: {
  flowId: string
  id: number
  outputStatus: 'idle' | 'running' | 'ready' | 'stale'
  sourceImageUrl: string
  pendingTaskId?: string
  outputImages?: Array<{ url: string; ratio: number }>
}): FlowAdapterNode {
  const snapshotNode = {
    id: params.id,
    type: 'image-node',
    x: 100,
    y: 80,
    width: 320,
    sourceImageUrl: params.sourceImageUrl,
    sourceImageRatio: 1,
    outputStatus: params.outputStatus,
    outputImages: params.outputImages ?? [],
    ...(params.pendingTaskId ? { pendingTaskId: params.pendingTaskId } : {}),
  }

  return {
    id: params.flowId,
    type: 'canvas',
    position: { x: 100, y: 80 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: `Image #${params.id}`,
      summary: 'Canvas image node',
      snapshotNode,
    },
  }
}

function makePromptFlowNode(params: { flowId: string; id: number; text: string }): FlowAdapterNode {
  return {
    id: params.flowId,
    type: 'canvas',
    position: { x: 40, y: 320 },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      title: `Prompt #${params.id}`,
      summary: 'Canvas prompt node',
      snapshotNode: {
        id: params.id,
        type: 'prompt',
        x: 40,
        y: 320,
        width: 280,
        text: params.text,
      },
    },
  }
}

function makeFusionFlowNode(params: {
  flowId: string
  id: number
  prompt?: string
}): FlowAdapterNode {
  return {
    id: params.flowId,
    type: 'canvas',
    position: { x: 340, y: 340 },
    data: {
      kind: 'fusion',
      nodeType: 'fusion',
      title: `Fusion #${params.id}`,
      summary: 'Canvas fusion node',
      snapshotNode: {
        id: params.id,
        type: 'fusion',
        x: 340,
        y: 340,
        width: 280,
        prompt: params.prompt ?? '',
        outputCount: 1,
        aspectRatio: 'auto',
        blendPoint: { x: 0.5, y: 0.5 },
        inputImages: [],
        outputStatus: 'idle',
        outputImages: [],
      },
    },
  }
}

describe('flowSnapshotAdapter', () => {
  it('persists pending running image nodes without image sources', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'running',
        sourceImageUrl: '',
        pendingTaskId: 'task_pending_2',
      }),
    ]

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges: [] })
    expect(snapshot.imageNodes.length).toBe(1)
    expect(snapshot.imageNodes[0]?.id).toBe(2)
    expect(snapshot.imageNodes[0]?.pendingTaskId).toBe('task_pending_2')

    const flow = canvasBoardSnapshotToFlow(snapshot)
    const savedAgain = flowToCanvasBoardSnapshot(flow)
    expect(savedAgain.imageNodes[0]?.pendingTaskId).toBe('task_pending_2')
    expect(savedAgain.imageNodes[0]?.outputStatus).toBe('running')
  })

  it('does not persist legacy empty running image nodes without pending task metadata', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
      }),
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'running',
        sourceImageUrl: '',
      }),
    ]
    const edges: FlowAdapterEdge[] = [
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'image-2',
        targetHandle: 'in:image',
      },
    ]

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges })
    expect(snapshot.imageNodes.length).toBe(1)
    expect(snapshot.imageNodes[0]?.id).toBe(1)
    expect(snapshot.connections.length).toBe(0)
  })

  it('persists ready image nodes with connections', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
      }),
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/result.png',
      }),
    ]
    const edges: FlowAdapterEdge[] = [
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'image-2',
        targetHandle: 'in:image',
      },
    ]

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges })
    expect(snapshot.imageNodes.length).toBe(2)
    expect(snapshot.connections.length).toBe(1)
    expect(snapshot.connections[0]?.fromNodeType).toBe('image')
    expect(snapshot.connections[0]?.toNodeType).toBe('image')
  })

  it('resolves aggregate input handles before persistence', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
      }),
      makePromptFlowNode({ flowId: 'prompt-3', id: 3, text: 'studio light' }),
      makeFusionFlowNode({ flowId: 'fusion-4', id: 4 }),
    ]
    const edges: FlowAdapterEdge[] = [
      {
        id: 'edge-image-fusion',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-4',
        targetHandle: 'in:__auto',
      },
      {
        id: 'edge-prompt-fusion',
        source: 'prompt-3',
        sourceHandle: 'out:prompt',
        target: 'fusion-4',
        targetHandle: 'in:__auto',
      },
    ]

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges })
    expect(snapshot.connections.length).toBe(2)
    expect(
      snapshot.connections.some(
        (connection) => connection.fromNodeType === 'image' && connection.toPort === 'image',
      ),
    ).toBe(true)
    expect(
      snapshot.connections.some(
        (connection) => connection.fromNodeType === 'prompt' && connection.toPort === 'prompt',
      ),
    ).toBe(true)
  })

  it('preserves edges across connection round trips', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
      }),
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/result.png',
      }),
    ]
    const snapshot = flowToCanvasBoardSnapshot({
      nodes,
      edges: [
        {
          id: 'edge-1',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'image-2',
          targetHandle: 'in:image',
        },
      ],
    })
    const flow = canvasBoardSnapshotToFlow(snapshot)
    const savedAgain = flowToCanvasBoardSnapshot(flow)

    expect(savedAgain.connections.length).toBe(1)
    expect(savedAgain.connections[0]?.fromNodeId).toBe(1)
    expect(savedAgain.connections[0]?.toNodeId).toBe(2)
  })

  it('preserves multiple prompt image input connections', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source-1.png',
      }),
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source-2.png',
      }),
      makePromptFlowNode({ flowId: 'prompt-3', id: 3, text: 'studio light' }),
    ]
    const snapshot = flowToCanvasBoardSnapshot({
      nodes,
      edges: [
        {
          id: 'edge-image-1-prompt',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-3',
          targetHandle: 'in:image',
        },
        {
          id: 'edge-image-2-prompt',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'prompt-3',
          targetHandle: 'in:image',
        },
      ],
    })

    const promptImageInputs = snapshot.connections.filter(
      (connection) => connection.toNodeType === 'prompt' && connection.toPort === 'image',
    )
    expect(promptImageInputs.length).toBe(2)
  })

  it('preserves multiple fusion image input connections', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source-1.png',
      }),
      makeImageFlowNode({
        flowId: 'image-2',
        id: 2,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source-2.png',
      }),
      makeFusionFlowNode({ flowId: 'fusion-3', id: 3, prompt: 'blend two references' }),
    ]
    const snapshot = flowToCanvasBoardSnapshot({
      nodes,
      edges: [
        {
          id: 'edge-image-1-fusion',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'fusion-3',
          targetHandle: 'in:image',
        },
        {
          id: 'edge-image-2-fusion',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'fusion-3',
          targetHandle: 'in:image',
        },
      ],
    })

    const fusionImageInputs = snapshot.connections.filter(
      (connection) => connection.toNodeType === 'fusion' && connection.toPort === 'image',
    )
    expect(fusionImageInputs.length).toBe(2)
  })

  it('preserves fusion blend points across round trips', () => {
    const flow = canvasBoardSnapshotToFlow({
      imageNodes: [],
      promptNodes: [],
      viewpointNodes: [],
      fusionNodes: [
        {
          id: 31,
          type: 'fusion',
          x: 200,
          y: 120,
          width: 280,
          prompt: '',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.62, y: 0.41 },
          inputImages: [],
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
      connections: [],
      viewport: { canvasX: 0, canvasY: 0, zoomLevel: 100 },
    })
    const restored = flowToCanvasBoardSnapshot(flow)
    expect(restored.fusionNodes[0]?.blendPoint.x).toBe(0.62)
    expect(restored.fusionNodes[0]?.blendPoint.y).toBe(0.41)
  })
})
