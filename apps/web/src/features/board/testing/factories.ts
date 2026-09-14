import {
  assertLatestBoardSnapshot,
  assertLatestBoardState,
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type BoardState,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../domain'

export type MockBoardApiState = {
  listResponse: { code?: number; msg?: string; data: { list: unknown[] } }
  upsertResponse: unknown
}

/**
 * Shared request mock for the user boards API contract: list on GET
 * /user/boards, echo the configured upsert response on PUT, empty success on
 * DELETE. State stays caller-owned so each spec drives its own fixtures.
 */
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
