import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
  type CanvasViewportState,
  type ImageNodeItem,
  type ImageNodeOutputItem,
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
