import {
  assertLatestBoardSnapshot,
  assertLatestBoardState,
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type BoardState,
  type ImageNodeItem,
  type ImageNodeOutputItem,
} from '../domain'

export type MockBoardRecordFixture = {
  board: Record<string, unknown>
  document: Record<string, unknown>
}

export type MockWorkspaceBoardApiState = {
  boardsResponse: {
    data: Array<Record<string, unknown>>
    meta: { next_cursor: string | null; has_more: boolean }
  }
  documentById: Record<string, Record<string, unknown>>
  savedCalls: Array<{
    url: string
    method: string
    headers: Record<string, unknown>
    body: unknown
  }>
  createdBoards: Array<Record<string, unknown>>
  archivedBoardIds: string[]
  joinedBoard: Record<string, unknown> | null
  listError?: unknown
  saveError?: unknown
  createError?: unknown
}

export function createMockWorkspaceBoardApiState(
  overrides: Partial<MockWorkspaceBoardApiState> = {},
): MockWorkspaceBoardApiState {
  return {
    boardsResponse: { data: [], meta: { next_cursor: null, has_more: false } },
    documentById: {},
    savedCalls: [],
    createdBoards: [],
    archivedBoardIds: [],
    joinedBoard: null,
    ...overrides,
  }
}

function parseBody(body: unknown): unknown {
  if (typeof body === 'string') {
    try {
      return JSON.parse(body) as unknown
    } catch {
      return body
    }
  }
  return body
}

type MockRequestConfig = { method?: string; headers?: unknown; body?: unknown }

function configHeaders(config?: MockRequestConfig): Record<string, unknown> {
  return config?.headers && typeof config.headers === 'object'
    ? (config.headers as Record<string, unknown>)
    : {}
}

function mockListBoards(
  state: MockWorkspaceBoardApiState,
  workspaceId: string,
): Promise<unknown> {
  if (state.listError) return Promise.reject(state.listError)
  void workspaceId
  return Promise.resolve(state.boardsResponse)
}

function mockCreateBoard(
  state: MockWorkspaceBoardApiState,
  workspaceId: string,
  config?: MockRequestConfig,
): Promise<unknown> {
  if (state.createError) return Promise.reject(state.createError)
  const body = parseBody(config?.body) as { data?: { name?: string } }
  const created = {
    id: 'board_server_created',
    workspace_id: workspaceId,
    name: body?.data?.name ?? 'Untitled',
    revision: 1,
    schema_version: 4,
    archived_at: null,
    created_at: '2026-09-25T00:00:00.000Z',
    updated_at: '2026-09-25T00:00:00Z',
  }
  state.createdBoards.push(created)
  return Promise.resolve({ data: created })
}

function mockJoinBoard(
  state: MockWorkspaceBoardApiState,
  url: string,
  config?: MockRequestConfig,
): Promise<unknown> {
  state.savedCalls.push({
    url,
    method: 'POST',
    headers: configHeaders(config),
    body: parseBody(config?.body),
  })
  if (state.joinedBoard) return Promise.resolve({ data: state.joinedBoard })
  return Promise.reject(new Error(`Unexpected board join without fixture: POST ${url}`))
}

function mockReadDocument(
  state: MockWorkspaceBoardApiState,
  boardId: string,
): Promise<unknown> {
  const document = state.documentById[boardId]
  if (!document) {
    return Promise.reject({ status: 404, code: 'board_not_found', msg: '看板不存在' })
  }
  return Promise.resolve({ data: document })
}

function mockReplaceDocument(
  state: MockWorkspaceBoardApiState,
  url: string,
  boardId: string,
  config?: MockRequestConfig,
): Promise<unknown> {
  state.savedCalls.push({
    url,
    method: 'PUT',
    headers: configHeaders(config),
    body: parseBody(config?.body),
  })
  if (state.saveError) return Promise.reject(state.saveError)
  const previous = state.documentById[boardId]
  const body = parseBody(config?.body) as {
    data?: { schema_version?: number; payload?: Record<string, unknown> }
  }
  // The backend stores the full `{schema_version, payload}` snapshot
  // verbatim; keep the mock faithful so save→read round-trips honestly.
  const next = {
    board_id: boardId,
    revision: Number((previous?.revision as number | undefined) ?? 1) + 1,
    schema_version: body?.data?.schema_version ?? 4,
    document: {
      schema_version: body?.data?.schema_version ?? 4,
      payload: body?.data?.payload ?? {},
    },
    updated_at: '2026-09-25T00:00:01.000Z',
  }
  state.documentById[boardId] = next
  return Promise.resolve({ data: next })
}

function mockReadOrRenameBoard(
  state: MockWorkspaceBoardApiState,
  url: string,
  boardId: string,
  config?: MockRequestConfig,
): Promise<unknown> {
  const method = config?.method ?? 'GET'
  if (method === 'PATCH') {
    state.savedCalls.push({
      url,
      method,
      headers: configHeaders(config),
      body: parseBody(config?.body),
    })
  }
  const board = state.boardsResponse.data.find((item) => item.id === boardId)
  if (!board) {
    return Promise.reject({ status: 404, code: 'board_not_found', msg: '看板不存在' })
  }
  if (method === 'PATCH') {
    const body = parseBody(config?.body) as { data?: { name?: string } }
    if (typeof body?.data?.name === 'string') board.name = body.data.name
  }
  return Promise.resolve({ data: board })
}

function mockArchiveBoard(
  state: MockWorkspaceBoardApiState,
  boardId: string,
): Promise<unknown> {
  state.archivedBoardIds.push(boardId)
  const board = state.boardsResponse.data.find((item) => item.id === boardId)
  return Promise.resolve({
    data: {
      ...(board ?? { id: boardId, name: 'Board' }),
      archived_at: '2026-09-25T00:00:00.000Z',
    },
  })
}

function mockRestoreBoard(
  state: MockWorkspaceBoardApiState,
  boardId: string,
): Promise<unknown> {
  state.archivedBoardIds = state.archivedBoardIds.filter((id) => id !== boardId)
  const board = state.boardsResponse.data.find((item) => item.id === boardId)
  return Promise.resolve({
    data: {
      ...(board ?? { id: boardId, name: 'Board' }),
      archived_at: null,
    },
  })
}

/**
 * Shared request mock for the v1 workspace boards contract
 * (`/api/v1/workspaces/{ws}/boards/*` through `publicMutator`): cursor
 * list on GET collection, document reads on GET `.../document`, rename on
 * PATCH, document replace on PUT (echoes revision + 1 unless overridden),
 * server-mint create on POST collection, archive on POST `.../archive`,
 * restore on POST `.../restore`, share join on POST `.../join`. State stays caller-owned so each spec
 * drives its own fixtures; failures inject via `listError`/`saveError`/
 * `createError` as rejected transport errors.
 */
type BoardRouteHandler = (
  state: MockWorkspaceBoardApiState,
  url: string,
  config: MockRequestConfig | undefined,
  match: RegExpMatchArray,
) => Promise<unknown>

const BOARD_COLLECTION_PATTERN = /^\/v1\/workspaces\/([^/]+)\/boards$/

const BOARD_ROUTE_TABLE: Array<{
  pattern: RegExp
  methods: readonly string[]
  run: BoardRouteHandler
}> = [
  {
    pattern: BOARD_COLLECTION_PATTERN,
    methods: ['GET'],
    run: (state, _url, _config, match) => mockListBoards(state, match[1] ?? ''),
  },
  {
    pattern: BOARD_COLLECTION_PATTERN,
    methods: ['POST'],
    run: (state, _url, config, match) => mockCreateBoard(state, match[1] ?? '', config),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/join$/,
    methods: ['POST'],
    run: (state, url, config) => mockJoinBoard(state, url, config),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)\/document$/,
    methods: ['GET'],
    run: (state, _url, _config, match) => mockReadDocument(state, match[1] ?? ''),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)\/document$/,
    methods: ['PUT'],
    run: (state, url, config, match) => mockReplaceDocument(state, url, match[1] ?? '', config),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)$/,
    methods: ['GET', 'PATCH'],
    run: (state, url, config, match) => mockReadOrRenameBoard(state, url, match[1] ?? '', config),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)\/archive$/,
    methods: ['POST'],
    run: (state, _url, _config, match) => mockArchiveBoard(state, match[1] ?? ''),
  },
  {
    pattern: /^\/v1\/workspaces\/[^/]+\/boards\/([^/]+)\/restore$/,
    methods: ['POST'],
    run: (state, _url, _config, match) => mockRestoreBoard(state, match[1] ?? ''),
  },
]

export function createMockWorkspaceBoardApiRequest(state: MockWorkspaceBoardApiState) {
  return (url: string, config?: MockRequestConfig) => {
    const method = config?.method ?? 'GET'
    const path = url.split('?')[0] ?? url
    for (const route of BOARD_ROUTE_TABLE) {
      if (!route.methods.includes(method)) continue
      const match = path.match(route.pattern)
      if (match) return route.run(state, url, config, match)
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

export function makeBoardSnapshot(overrides: Partial<BoardSnapshot> = {}): BoardSnapshot {
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
