import {
  assertLatestBoardSnapshot,
  assertLatestBoardState,
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type BoardState,
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

export function makeBoardSnapshot(
  overrides: Partial<BoardSnapshot> = {},
): BoardSnapshot {
  return assertLatestBoardSnapshot(
    {
      ...createEmptyBoardSnapshot(),
      ...overrides,
    },
    'test-fixture-snapshot',
  )
}

export function makeBoardState(overrides: Partial<BoardState> = {}): BoardState {
  const {
    updatedAt,
    previewSnapshotDataUrl,
    previewSnapshotUpdatedAt,
    previewSnapshotVersion,
    ...snapshotOverrides
  } = overrides
  return assertLatestBoardState(
    {
      ...createEmptyBoardSnapshot(),
      ...snapshotOverrides,
      updatedAt: updatedAt ?? 1,
      previewSnapshotDataUrl,
      previewSnapshotUpdatedAt,
      previewSnapshotVersion,
    },
    'test-fixture-state',
  )
}
