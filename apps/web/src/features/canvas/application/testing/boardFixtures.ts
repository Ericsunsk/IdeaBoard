import {
  assertLatestBoardSnapshot,
  createEmptyBoardSnapshot,
  type BoardSnapshot,
} from '../../../board'

export function makeBoardSnapshot(
  overrides: Partial<BoardSnapshot> = {},
): BoardSnapshot {
  return assertLatestBoardSnapshot(
    {
      ...createEmptyBoardSnapshot(),
      ...overrides,
    },
    'canvas-application-test-fixture-snapshot',
  )
}

// Single source lives in board/testing/factories; re-exported here so canvas
// specs keep a stable local import without a second copy of the factory.
export { makeBoardState } from '../../../board/testing/factories'
export {
  createMockBoardApiRequest,
  type MockBoardApiState,
} from '../../../board/testing/factories'
