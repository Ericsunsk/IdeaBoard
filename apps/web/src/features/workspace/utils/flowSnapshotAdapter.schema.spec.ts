import {
  canvasBoardSnapshotToFlow,
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

function makeModelFlowNode(params: { flowId: string; id: number }): FlowAdapterNode {
  return {
    id: params.flowId,
    type: 'canvas',
    position: { x: 460, y: 120 },
    data: {
      kind: 'model',
      nodeType: 'model',
      title: `Model #${params.id}`,
      summary: 'Canvas model node',
      snapshotNode: {
        id: params.id,
        type: 'model',
        x: 460,
        y: 120,
        width: 260,
        angleSpec: { yaw: 0, framing: 'full' },
        modelStrength: 80,
        poseStrength: 70,
        background: 'white',
        outputStatus: 'idle',
        outputImages: [],
      },
    },
  }
}

function makeFusionFlowNode(params: { flowId: string; id: number; prompt?: string }): FlowAdapterNode {
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

function testAggregateInputHandlesAreResolvedBeforePersistence() {
  const nodes: FlowAdapterNode[] = [
    makeImageFlowNode({
      flowId: 'image-1',
      id: 1,
      outputStatus: 'ready',
      sourceImageUrl: 'https://example.com/source.png',
    }),
    makePromptFlowNode({ flowId: 'prompt-3', id: 3, text: 'studio light' }),
    makeModelFlowNode({ flowId: 'model-4', id: 4 }),
  ]
  const edges: FlowAdapterEdge[] = [
    {
      id: 'edge-image-model',
      source: 'image-1',
      sourceHandle: 'out:image',
      target: 'model-4',
      targetHandle: 'in:__auto',
    },
    {
      id: 'edge-prompt-model',
      source: 'prompt-3',
      sourceHandle: 'out:prompt',
      target: 'model-4',
      targetHandle: 'in:__auto',
    },
  ]

  const snapshot = flowToCanvasBoardSnapshot({ nodes, edges })
  expect(snapshot.connections.length === 2, 'expected aggregate handle connections to persist')
  expect(
    snapshot.connections.some(
      (connection) => connection.fromNodeType === 'image' && connection.toPort === 'image',
    ),
    'expected image aggregate connection to resolve to image input',
  )
  expect(
    snapshot.connections.some(
      (connection) => connection.fromNodeType === 'prompt' && connection.toPort === 'prompt',
    ),
    'expected prompt aggregate connection to resolve to prompt input',
  )
}

function testConnectionRoundTripPreservesEdges() {
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

  expect(savedAgain.connections.length === 1, 'expected round-trip connection to persist')
  expect(
    savedAgain.connections[0]?.fromNodeId === 1 && savedAgain.connections[0]?.toNodeId === 2,
    'expected round-trip connection endpoints to be stable',
  )
}

function testPromptImageInputPreservesMultipleConnections() {
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
  expect(promptImageInputs.length === 2, 'expected prompt image input to preserve multiple edges')
}

function testFusionImageInputPreservesMultipleConnections() {
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
  expect(fusionImageInputs.length === 2, 'expected fusion image input to preserve multiple edges')
}

function testFusionBlendPointRoundTripPreserved() {
  const flow = canvasBoardSnapshotToFlow({
    imageNodes: [],
    promptNodes: [],
    modelNodes: [],
    viewpointNodes: [],
    maskNodes: [],
    editNodes: [],
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
    enhanceNodes: [],
    wearNodes: [],
    connections: [],
    viewport: { canvasX: 0, canvasY: 0, zoomLevel: 100 },
  })
  const restored = flowToCanvasBoardSnapshot(flow)
  expect(restored.fusionNodes[0]?.blendPoint.x === 0.62, 'expected fusion blendPoint x to persist')
  expect(restored.fusionNodes[0]?.blendPoint.y === 0.41, 'expected fusion blendPoint y to persist')
}

testTransientRunningImageNodeIsNotPersisted()
testReadyImageNodeIsPersistedWithConnection()
testAggregateInputHandlesAreResolvedBeforePersistence()
testConnectionRoundTripPreservesEdges()
testPromptImageInputPreservesMultipleConnections()
testFusionImageInputPreservesMultipleConnections()
testFusionBlendPointRoundTripPreserved()

console.log('flowSnapshotAdapter snapshot checks passed')
