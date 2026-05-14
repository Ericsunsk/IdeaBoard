import {
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from './flowSnapshotAdapter'

function expect(condition: unknown, message: string) {
  if (!condition) throw new Error(message)
}

function makeImageFlowNode(params: {
  flowId: string
  id: number
  outputStatus: 'idle' | 'running' | 'ready' | 'stale'
  sourceImageUrl: string
  outputImages?: Array<{ url: string; ratio: number }>
}): FlowAdapterNode {
  return {
    id: params.flowId,
    type: 'canvas',
    position: { x: 100, y: 80 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: `Image #${params.id}`,
      summary: 'Canvas image node',
      snapshotNode: {
        id: params.id,
        type: 'image-node',
        x: 100,
        y: 80,
        width: 320,
        sourceImageUrl: params.sourceImageUrl,
        sourceImageRatio: 1,
        outputStatus: params.outputStatus,
        outputImages: params.outputImages ?? [],
      },
    },
  }
}

function testTransientRunningImageNodeIsNotPersisted() {
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
  expect(snapshot.imageNodes.length === 1, 'expected only non-transient image node to persist')
  expect(snapshot.imageNodes[0]?.id === 1, 'expected persisted image node id to be 1')
  expect(
    snapshot.connections.length === 0,
    'expected connections to transient image node to be removed',
  )
}

function testReadyImageNodeIsPersistedWithConnection() {
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
  expect(snapshot.imageNodes.length === 2, 'expected ready image nodes to persist')
  expect(
    snapshot.connections.length === 1,
    'expected connection between ready image nodes to persist',
  )
  expect(snapshot.connections[0]?.fromNodeType === 'image', 'expected fromNodeType to be image')
  expect(snapshot.connections[0]?.toNodeType === 'image', 'expected toNodeType to be image')
}

testTransientRunningImageNodeIsNotPersisted()
testReadyImageNodeIsPersistedWithConnection()

console.log('flowSnapshotAdapter snapshot checks passed')
