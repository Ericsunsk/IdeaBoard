import { describe, expect, it } from 'vitest'
import {
  canvasBoardSnapshotToFlow,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from './flowSnapshotAdapter'
import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  createEmptyCanvasBoardSnapshot,
  type CanvasGroup,
} from '../../canvas-board/domain'

const GROUP_GEOMETRY = {
  x: 20,
  y: 20,
  width: 700,
  height: 420,
}

function makeGroup(
  id: CanvasGroup['id'],
  nodeIds: CanvasGroup['nodeIds'],
  geometry: Pick<CanvasGroup, 'x' | 'y' | 'width' | 'height'> = GROUP_GEOMETRY,
): CanvasGroup {
  return {
    id,
    nodeIds,
    ...geometry,
  }
}

function makeImageFlowNode(params: {
  flowId: string
  id: number
  outputStatus: 'idle' | 'running' | 'ready' | 'stale'
  sourceImageUrl: string
  sourceMediaAssetId?: number
  pendingTaskId?: string
  outputImages?: Array<{ url: string; ratio: number; mediaAssetId?: number }>
}): FlowAdapterNode {
  const snapshotNode = {
    id: params.id,
    type: 'image-node',
    x: 100,
    y: 80,
    width: 320,
    sourceImageUrl: params.sourceImageUrl,
    sourceMediaAssetId: params.sourceMediaAssetId,
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
  outputImages?: Array<{ url: string; ratio: number; mediaAssetId?: number }>
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
        outputStatus: params.outputImages?.length ? 'ready' : 'idle',
        outputImages: params.outputImages ?? [],
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

  it('uses snapshot node identity when a Flow node id drifts', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'node-image-2',
        id: 2,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
      }),
      makePromptFlowNode({ flowId: 'node-prompt-1', id: 1, text: 'use this image' }),
    ]
    const edges: FlowAdapterEdge[] = [
      {
        id: 'edge-drifted',
        source: 'node-image-2',
        sourceHandle: 'out:image',
        target: 'node-prompt-1',
        targetHandle: 'in:image',
      },
    ]

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges })

    expect(snapshot.imageNodes.map((node) => node.id)).toEqual([2])
    expect(snapshot.promptNodes.map((node) => node.id)).toEqual([1])
    expect(snapshot.connections).toMatchObject([
      {
        fromNodeType: 'image',
        fromNodeId: 2,
        toNodeType: 'prompt',
        toNodeId: 1,
      },
    ])
  })

  it('does not persist local-only empty running image nodes without pending task metadata', () => {
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

  it('can include local-only empty running image nodes for runtime materialization', () => {
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

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges }, undefined, {
      projection: 'runtime',
    })
    expect(snapshot.imageNodes.map((node) => node.id)).toEqual([1, 2])
    expect(snapshot.connections).toHaveLength(1)
    expect(snapshot.imageNodes[1]).toMatchObject({
      id: 2,
      outputStatus: 'running',
      sourceImageUrl: '',
      outputImages: [],
    })
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

  it('allocates new connection ids after base ids when restored ids collide', () => {
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
    const base = createEmptyCanvasBoardSnapshot()
    base.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 99,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 100,
      toPort: 'image',
    })

    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes,
        edges: [
          {
            id: 'edge-restored',
            source: 'image-1',
            sourceHandle: 'out:image',
            target: 'prompt-3',
            targetHandle: 'in:image',
            data: { connectionSnapshot: { id: 7 } },
          },
          {
            id: 'edge-colliding',
            source: 'image-2',
            sourceHandle: 'out:image',
            target: 'prompt-3',
            targetHandle: 'in:image',
            data: { connectionSnapshot: { id: 7 } },
          },
        ],
      },
      base,
    )

    expect(snapshot.connections.map((connection) => connection.id)).toEqual([7, 43])
  })

  it('preserves a base connection id when snapshotting a generated edge by endpoint', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source-1.png',
      }),
      makePromptFlowNode({ flowId: 'prompt-3', id: 3, text: 'studio light' }),
    ]
    const base = createEmptyCanvasBoardSnapshot()
    base.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 3,
      toPort: 'image',
    })

    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes,
        edges: [
          {
            id: 'vueflow__edge-image-1out:image-prompt-3in:image',
            source: 'image-1',
            sourceHandle: 'out:image',
            target: 'prompt-3',
            targetHandle: 'in:image',
          },
        ],
      },
      base,
    )

    expect(snapshot.connections).toEqual([
      expect.objectContaining({
        id: 42,
        fromNodeId: 1,
        toNodeId: 3,
      }),
    ])
  })

  it('keeps the last edge for single-input target ports', () => {
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
      makeImageFlowNode({
        flowId: 'image-3',
        id: 3,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/target.png',
      }),
    ]

    const snapshot = flowToCanvasBoardSnapshot({
      nodes,
      edges: [
        {
          id: 'edge-first',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'image-3',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 11 } },
        },
        {
          id: 'edge-last',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'image-3',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 12 } },
        },
      ],
    })

    expect(snapshot.connections).toHaveLength(1)
    expect(snapshot.connections[0]).toMatchObject({
      id: 12,
      fromNodeId: 2,
      toNodeId: 3,
      toPort: 'image',
    })
  })

  it('preserves fusion blend points across round trips', () => {
    const flow = canvasBoardSnapshotToFlow({
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
      imageNodes: [],
      promptNodes: [],
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
      groups: [],
    })
    const restored = flowToCanvasBoardSnapshot(flow)
    expect(restored.fusionNodes[0]?.blendPoint.x).toBe(0.62)
    expect(restored.fusionNodes[0]?.blendPoint.y).toBe(0.41)
  })

  it('preserves current groups while exporting flow state', () => {
    const base = createEmptyCanvasBoardSnapshot()
    base.groups = [
      makeGroup('group-1', ['image-1', 'prompt-1']),
      makeGroup('group-2', ['image-1', 'prompt-2']),
    ]

    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: [
          makeImageFlowNode({
            flowId: 'image-1',
            id: 1,
            outputStatus: 'ready',
            sourceImageUrl: 'https://example.com/source.png',
          }),
          makePromptFlowNode({ flowId: 'prompt-1', id: 1, text: 'prompt' }),
        ],
        edges: [],
      },
      base,
    )

    expect(snapshot.groups).toEqual([
      makeGroup('group-1', ['image-1', 'prompt-1']),
      makeGroup('group-2', []),
    ])
    expect(snapshot.groups).not.toBe(base.groups)
  })

  it('drops stale board node media refs while exporting flow state', () => {
    const base = createEmptyCanvasBoardSnapshot()
    base.mediaRefs = [{ mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'source' }]

    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: [makePromptFlowNode({ flowId: 'prompt-1', id: 1, text: 'prompt' })],
        edges: [],
      },
      base,
    )

    expect(snapshot.mediaRefs).toBeUndefined()
  })

  it('rebuilds current image node media refs while exporting flow state', () => {
    const base = createEmptyCanvasBoardSnapshot()
    base.mediaRefs = [
      { mediaAssetId: 99, refType: 'board', refId: 'cover', role: 'preview' },
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'result' },
      { mediaAssetId: 13, refType: 'board_node', refId: 'image-2', role: 'source' },
    ]

    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: [
          makeImageFlowNode({
            flowId: 'image-1',
            id: 1,
            outputStatus: 'ready',
            sourceImageUrl: 'https://example.com/source.png',
            sourceMediaAssetId: 12,
          }),
        ],
        edges: [],
      },
      base,
    )

    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 99, refType: 'board', refId: 'cover', role: 'preview' },
      { mediaAssetId: 12, refType: 'board_node', refId: 'image-1', role: 'result' },
    ])
    expect(snapshot.mediaRefs).not.toBe(base.mediaRefs)
  })

  it('rebuilds image and fusion output media refs and drops refs for removed outputs', () => {
    const current = flowToCanvasBoardSnapshot({
      nodes: [
        makeImageFlowNode({
          flowId: 'image-1',
          id: 1,
          outputStatus: 'ready',
          sourceImageUrl: 'https://example.com/source.png',
          outputImages: [
            { url: 'https://example.com/image-output-a.png', ratio: 1, mediaAssetId: 21 },
            { url: 'https://example.com/image-output-b.png', ratio: 1, mediaAssetId: 22 },
          ],
        }),
        makeFusionFlowNode({
          flowId: 'fusion-2',
          id: 2,
          outputImages: [
            { url: 'https://example.com/fusion-output.png', ratio: 1, mediaAssetId: 31 },
          ],
        }),
      ],
      edges: [],
    })

    expect(current.mediaRefs).toEqual([
      { mediaAssetId: 21, refType: 'board_node', refId: 'image-1', role: 'output:0' },
      { mediaAssetId: 22, refType: 'board_node', refId: 'image-1', role: 'output:1' },
      { mediaAssetId: 31, refType: 'board_node', refId: 'fusion-2', role: 'output:0' },
    ])

    const afterDelete = flowToCanvasBoardSnapshot(
      {
        nodes: [
          makeImageFlowNode({
            flowId: 'image-1',
            id: 1,
            outputStatus: 'ready',
            sourceImageUrl: 'https://example.com/source.png',
            outputImages: [
              { url: 'https://example.com/image-output-b.png', ratio: 1, mediaAssetId: 22 },
            ],
          }),
          makeFusionFlowNode({ flowId: 'fusion-2', id: 2 }),
        ],
        edges: [],
      },
      current,
    )

    expect(afterDelete.mediaRefs).toEqual([
      { mediaAssetId: 22, refType: 'board_node', refId: 'image-1', role: 'output:0' },
    ])
  })

  it('adds stable node dimensions for minimap geometry before nodes are rendered', () => {
    const flow = canvasBoardSnapshotToFlow({
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 20,
          y: 30,
          width: 320,
          sourceImageUrl: 'https://example.com/source.png',
          sourceImageRatio: 0.5,
          outputStatus: 'ready',
          outputImages: [],
        },
      ],
      promptNodes: [
        {
          id: 2,
          type: 'prompt',
          x: 480,
          y: 30,
          width: 280,
          text: 'prompt',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [],
        },
      ],
      fusionNodes: [],
      connections: [],
      groups: [],
    })

    expect(flow.nodes.find((node) => node.id === 'image-1')?.dimensions).toEqual({
      width: 320,
      height: 182,
    })
    expect(flow.nodes.find((node) => node.id === 'prompt-2')?.dimensions).toEqual({
      width: 280,
      height: 302,
    })
  })

  it('creates save-owned snapshot nodes without sharing nested flow node data', () => {
    const nodes: FlowAdapterNode[] = [
      makeImageFlowNode({
        flowId: 'image-1',
        id: 1,
        outputStatus: 'ready',
        sourceImageUrl: 'https://example.com/source.png',
        outputImages: [{ url: 'https://example.com/output.png', ratio: 1 }],
      }),
    ]
    const sourceSnapshotNode = (
      nodes[0]?.data as { snapshotNode: { outputImages: Array<{ url: string }> } }
    ).snapshotNode

    const snapshot = flowToCanvasBoardSnapshot({ nodes, edges: [] })
    snapshot.imageNodes[0]!.outputImages[0]!.url = 'https://example.com/stabilized.png'

    expect(sourceSnapshotNode.outputImages[0]?.url).toBe('https://example.com/output.png')
  })
})
