import {
  assertLatestBoardSnapshot,
  assertLatestBoardState,
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type BoardState,
} from '../../../board'

export function makeBoardSnapshot(overrides: Partial<BoardSnapshot> = {}): BoardSnapshot {
  return assertLatestBoardSnapshot(
    {
      ...createEmptyBoardSnapshot(),
      ...overrides,
    },
    'canvas-application-test-fixture-snapshot',
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
    'canvas-test-fixture-state',
  )
}
