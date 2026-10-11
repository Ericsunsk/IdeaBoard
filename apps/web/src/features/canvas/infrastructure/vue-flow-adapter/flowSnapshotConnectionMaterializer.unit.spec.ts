import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type CanvasConnectionNodeKind,
  type CanvasNodeItem,
} from '../../../board'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
import {
  materializeFlowSnapshotConnections,
  type FlowSnapshotConnectionEdge,
  type FlowSnapshotConnectionNodeRef,
} from './flowSnapshotConnectionMaterializer'
import { resolveFlowEdgeCanvasConnectionId } from './flowConnectionIdentityResolver'

function imageSnapshot(id: number): CanvasNodeItem {
  return {
    id,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: `https://example.com/image-${id}.png`,
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
  }
}

function promptSnapshot(id: number): CanvasNodeItem {
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

function fusionSnapshot(id: number): CanvasNodeItem {
  return {
    id,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 286,
    prompt: '',
    outputCount: 1,
    aspectRatio: 'auto',
    blendPoint: { x: 0.5, y: 0.5 },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
  }
}

function nodeData(kind: CanvasConnectionNodeKind, snapshotNode: CanvasNodeItem): FlowNodeData {
  return {
    kind,
    nodeType: kind,
    title: `${kind}-${snapshotNode.id}`,
    snapshotNode,
  }
}

function createHarness() {
  const persistedFlowNodeRefs = new Map<string, FlowSnapshotConnectionNodeRef>([
    ['image-1', { nodeType: 'image', id: 1 }],
    ['image-2', { nodeType: 'image', id: 2 }],
    ['image-3', { nodeType: 'image', id: 3 }],
    ['prompt-4', { nodeType: 'prompt', id: 4 }],
    ['fusion-5', { nodeType: 'fusion', id: 5 }],
  ])
  const flowNodeData = new Map<string, FlowNodeData>([
    ['image-1', nodeData('image', imageSnapshot(1))],
    ['image-2', nodeData('image', imageSnapshot(2))],
    ['image-3', nodeData('image', imageSnapshot(3))],
    ['prompt-4', nodeData('prompt', promptSnapshot(4))],
    ['fusion-5', nodeData('fusion', fusionSnapshot(5))],
  ])

  return {
    persistedFlowNodeRefs,
    lookupData: (nodeId: string) => flowNodeData.get(nodeId),
  }
}

describe('flowSnapshotConnectionMaterializer', () => {
  it('reuses the base connection id for a generated edge with matching endpoints', () => {
    const harness = createHarness()
    const base = createEmptyBoardSnapshot()
    base.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 4,
      toPort: 'image',
    })

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      baseConnections: base.connections,
      edges: [
        {
          id: 'e1-2',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
        },
      ],
    })

    expect(connections).toEqual([
      expect.objectContaining({
        id: 42,
        fromNodeId: 1,
        toNodeId: 4,
      }),
    ])
  })

  it('prefers a canonical Flow Edge id over a matching base endpoint', () => {
    const harness = createHarness()
    const base = createEmptyBoardSnapshot()
    base.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 4,
      toPort: 'image',
    })

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      baseConnections: base.connections,
      edges: [
        {
          id: 'connection-5',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
        },
      ],
    })

    expect(connections.map((connection) => connection.id)).toEqual([5])
  })

  it('restores one duplicate connection id and generates the collision after base ids', () => {
    const harness = createHarness()
    const base = createEmptyBoardSnapshot()
    base.connections.push({
      id: 42,
      fromNodeType: 'image',
      fromNodeId: 99,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 100,
      toPort: 'image',
    })

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      baseConnections: base.connections,
      edges: [
        {
          id: 'edge-restored',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 7 } },
        },
        {
          id: 'edge-colliding',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 7 } },
        },
      ],
    })

    expect(connections.map((connection) => connection.id)).toEqual([7, 43])
  })

  it('keeps repeated restored-id materialization stable against endpoint base ids', () => {
    const harness = createHarness()
    const base = createEmptyBoardSnapshot()
    base.connections.push(
      {
        id: 7,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 4,
        toPort: 'image',
      },
      {
        id: 43,
        fromNodeType: 'image',
        fromNodeId: 2,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 4,
        toPort: 'image',
      },
    )

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      baseConnections: base.connections,
      edges: [
        {
          id: 'edge-first',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 7 } },
        },
        {
          id: 'edge-colliding',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 7 } },
        },
      ],
    })

    expect(connections.map((connection) => connection.id)).toEqual([7, 43])
  })

  it('reserves a later explicit id before an earlier generated edge claims its base endpoint', () => {
    const harness = createHarness()
    const base = createEmptyBoardSnapshot()
    base.connections.push({
      id: 7,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 4,
      toPort: 'image',
    })

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      baseConnections: base.connections,
      edges: [
        {
          id: 'edge-generated',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
        },
        {
          id: 'edge-explicit',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 7 } },
        },
      ],
    })

    expect(connections.map((connection) => connection.id)).toEqual([8, 7])
  })

  it('prefers restored snapshot ids over edge ids before allocating new ids', () => {
    const harness = createHarness()
    const connections = materializeFlowSnapshotConnections({
      ...harness,
      edges: [
        {
          id: 'connection-5',
          source: 'image-1',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
          data: { connectionSnapshot: { id: 9 } },
        },
        {
          id: 'edge-new',
          source: 'image-2',
          sourceHandle: 'out:image',
          target: 'prompt-4',
          targetHandle: 'in:image',
        },
      ],
    })

    expect(resolveFlowEdgeCanvasConnectionId({ id: 'connection-5' })).toBe(5)
    expect(
      resolveFlowEdgeCanvasConnectionId({
        id: 'connection-5',
        data: { connectionSnapshot: { id: 9 } },
      }),
    ).toBe(9)
    expect(connections.map((connection) => connection.id)).toEqual([9, 10])
  })

  it('keeps the last materialized connection for single-input target ports', () => {
    const harness = createHarness()
    const connections = materializeFlowSnapshotConnections({
      ...harness,
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

    expect(connections).toEqual([
      expect.objectContaining({
        id: 12,
        fromNodeId: 2,
        toNodeType: 'image',
        toNodeId: 3,
        toPort: 'image',
      }),
    ])
  })

  it('normalizes auto target handles against already materialized target edges', () => {
    const harness = createHarness()
    const edges: FlowSnapshotConnectionEdge[] = [
      {
        id: 'edge-image-fusion',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'fusion-5',
        targetHandle: 'in:__auto',
      },
      {
        id: 'edge-prompt-fusion',
        source: 'prompt-4',
        sourceHandle: 'out:prompt',
        target: 'fusion-5',
        targetHandle: 'in:__auto',
      },
    ]

    const connections = materializeFlowSnapshotConnections({
      ...harness,
      edges,
    })

    expect(connections).toEqual([
      expect.objectContaining({ fromNodeType: 'image', toPort: 'image' }),
      expect.objectContaining({ fromNodeType: 'prompt', toPort: 'prompt' }),
    ])
  })
})
