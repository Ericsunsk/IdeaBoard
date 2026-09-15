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

export type MockBoardApiState = {
  listResponse: { code?: number; msg?: string; data: { list: unknown[] } }
  upsertResponse: unknown
}

/** Canvas owns this test seam locally to avoid importing Board's private testing module. */
export function createMockBoardApiRequest(state: MockBoardApiState) {
  return (url: string, config?: { method?: string }) => {
    const method = config?.method ?? 'GET'
    if (url === '/user/boards' && method === 'GET') {
      return Promise.resolve({ code: 0, msg: 'ok', ...state.listResponse })
    }
    if (url.startsWith('/user/boards/') && method === 'PUT') {
      return Promise.resolve(state.upsertResponse)
    }
    if (url.startsWith('/user/boards/') && method === 'DELETE') {
      return Promise.resolve({ code: 0, data: null })
    }
    return Promise.reject(new Error(`Unexpected board API request: ${method} ${url}`))
  }
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
