import {
  assertLatestCanvasBoardSnapshot,
  assertLatestCanvasBoardState,
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../domain'

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
  return assertLatestCanvasBoardSnapshot(
    {
      ...createEmptyCanvasBoardSnapshot(),
      ...overrides,
    },
    'test-fixture-snapshot',
  )
}

export function makeCanvasBoardState(overrides: Partial<CanvasBoardState> = {}): CanvasBoardState {
  const {
    updatedAt,
    previewSnapshotDataUrl,
    previewSnapshotUpdatedAt,
    previewSnapshotVersion,
    ...snapshotOverrides
  } = overrides
  return assertLatestCanvasBoardState(
    {
      ...createEmptyCanvasBoardSnapshot(),
      ...snapshotOverrides,
      updatedAt: updatedAt ?? 1,
      previewSnapshotDataUrl,
      previewSnapshotUpdatedAt,
      previewSnapshotVersion,
    },
    'test-fixture-state',
  )
}
