import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  DEFAULT_FUSION_BLEND_POINT,
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
  type CanvasConnection,
  type CanvasViewportState,
  type FusionNodeItem,
  type ImageNodeItem,
  type ImageNodeOutputItem,
  type PromptNodeInputImage,
  type PromptNodeItem,
} from '../domain'

export function makeCanvasViewport(
  overrides: Partial<CanvasViewportState> = {},
): CanvasViewportState {
  return {
    canvasX: 0,
    canvasY: 0,
    zoomLevel: 100,
    ...overrides,
  }
}

function makeImageOutput(overrides: Partial<ImageNodeOutputItem> = {}): ImageNodeOutputItem {
  return {
    url: 'https://example.com/output.png',
    ratio: 1,
    ...overrides,
  }
}

function makeNodeInputImage(overrides: Partial<PromptNodeInputImage> = {}): PromptNodeInputImage {
  return {
    id: 'input-1',
    sourceType: 'node',
    sourceId: 'image:1',
    sourceNodeId: 'image-1',
    ...makeImageOutput({ url: 'https://example.com/input.png' }),
    ...overrides,
  }
}

function makeImageNodeSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [makeImageOutput()],
    ...overrides,
  }
}

export function makePendingImageNodeSnapshot(
  overrides: Partial<ImageNodeItem> = {},
): ImageNodeItem {
  return makeImageNodeSnapshot({
    sourceImageUrl: '',
    pendingTaskId: 'task_pending_1',
    outputStatus: 'running',
    outputImages: [],
    ...overrides,
  })
}

function makePromptNodeSnapshot(overrides: Partial<PromptNodeItem> = {}): PromptNodeItem {
  return {
    id: 2,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    text: 'prompt',
    outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    inputImages: [],
    ...overrides,
  }
}

function makeFusionNodeSnapshot(overrides: Partial<FusionNodeItem> = {}): FusionNodeItem {
  return {
    id: 3,
    type: 'fusion',
    x: 0,
    y: 0,
    width: 286,
    prompt: 'fusion prompt',
    outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    blendPoint: { ...DEFAULT_FUSION_BLEND_POINT },
    inputImages: [],
    outputStatus: 'idle',
    outputImages: [],
    ...overrides,
  }
}

function makeCanvasConnection(overrides: Partial<CanvasConnection> = {}): CanvasConnection {
  return {
    id: 1,
    fromNodeType: 'image',
    fromNodeId: 1,
    fromPort: 'image',
    toNodeType: 'fusion',
    toNodeId: 3,
    toPort: 'image',
    ...overrides,
  }
}

export function makeCanvasBoardSnapshot(
  overrides: Partial<CanvasBoardSnapshot> = {},
): CanvasBoardSnapshot {
  return {
    ...createEmptyCanvasBoardSnapshot(),
    ...overrides,
    viewport: makeCanvasViewport(overrides.viewport),
  }
}

export function makeCanvasBoardState(overrides: Partial<CanvasBoardState> = {}): CanvasBoardState {
  return {
    ...makeCanvasBoardSnapshot(overrides),
    updatedAt: overrides.updatedAt ?? 1,
    previewSnapshotDataUrl: overrides.previewSnapshotDataUrl,
    previewSnapshotUpdatedAt: overrides.previewSnapshotUpdatedAt,
    previewSnapshotVersion: overrides.previewSnapshotVersion,
  }
}

function makePopulatedCanvasBoardState(
  overrides: Partial<CanvasBoardState> = {},
): CanvasBoardState {
  const imageNode = makeImageNodeSnapshot({ id: 2 })
  const promptNode = makePromptNodeSnapshot({ id: 3, title: 'Custom Prompt' })
  const fusionNode = makeFusionNodeSnapshot({ id: 8 })
  const connection = makeCanvasConnection({
    id: 12,
    fromNodeId: imageNode.id,
    toNodeId: fusionNode.id,
  })
  return makeCanvasBoardState({
    imageNodes: [imageNode],
    promptNodes: [promptNode],
    fusionNodes: [fusionNode],
    connections: [connection],
    previewSnapshotDataUrl: 'data:image/webp;base64,preview',
    previewSnapshotUpdatedAt: 1,
    previewSnapshotVersion: 1,
    updatedAt: 1,
    ...overrides,
  })
}
