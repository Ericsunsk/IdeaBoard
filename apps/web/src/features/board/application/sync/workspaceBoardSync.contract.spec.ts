// Workspace board sync contract (issue #802).
//
// Covers the snake_case transport -> camelCase domain mapping, the
// ETag/If-Match concurrency headers, the 404-vs-403 branches, and idempotent
// retry key reuse. Transport is mocked at `api.request` (no generated-query
// mocks); the shape assertions follow the AGREED envelopes in the #802
// report so backend drift surfaces here.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createWorkspaceBoard,
  replaceWorkspaceBoardDocument,
} from '../../infrastructure/api/workspaceBoardsResource'
import {
  buildBoardWriteHeaders,
  classifyBoardAccessError,
  clearStoredBoardRevision,
  getStoredBoardRevision,
  isBoardIdempotencyConflictError,
  isBoardRevisionConflictError,
  parseWorkspaceBoardItem,
  parseWorkspaceBoardPage,
  parseWorkspaceBoardSnapshot,
  setStoredBoardRevision,
  trackBoardSummary,
  workspaceBoardQueryKeys,
} from './workspaceBoardSync'

const mocks = vi.hoisted(() => {
  const request = vi.fn()
  return { request }
})

vi.mock('../../../../shared/api/client', () => ({
  api: {
    request: mocks.request,
  },
}))

const BOARD_ID = 'board_abc123'
const WORKSPACE_ID = 'ws_xyz789'

function boardPayload(overrides: Record<string, unknown> = {}) {
  return {
    data: {
      id: BOARD_ID,
      workspace_id: WORKSPACE_ID,
      name: 'Atlas',
      revision: 2,
      schema_version: 1,
      archived_at: null,
      created_at: '2026-09-01T00:00:00Z',
      updated_at: '2026-09-02T00:00:00Z',
      ...overrides,
    },
  }
}

describe('workspace board adapter mapping', () => {
  it('maps snake_case transport to camelCase domain', () => {
    const page = parseWorkspaceBoardPage({
      data: [
        {
          id: BOARD_ID,
          workspace_id: WORKSPACE_ID,
          name: 'Atlas',
          revision: 2,
          schema_version: 1,
          archived_at: null,
          created_at: '2026-09-01T00:00:00Z',
          updated_at: '2026-09-02T00:00:00Z',
        },
      ],
      meta: { next_cursor: 'cursor2', has_more: true },
    })
    expect(page.items).toEqual([
      {
        boardId: BOARD_ID,
        workspaceId: WORKSPACE_ID,
        name: 'Atlas',
        revision: 2,
        schemaVersion: 1,
        archivedAt: null,
        createdAt: '2026-09-01T00:00:00Z',
        updatedAt: '2026-09-02T00:00:00Z',
      },
    ])
    expect(page).toMatchObject({ nextCursor: 'cursor2', hasMore: true })
  })

  it('rejects malformed opaque ids instead of rendering them', () => {
    expect(() =>
      parseWorkspaceBoardItem({ id: 'nope', workspace_id: WORKSPACE_ID, name: 'X', revision: 1 }),
    ).toThrow()
    expect(() =>
      parseWorkspaceBoardItem({ id: BOARD_ID, workspace_id: 'org_1', name: 'X', revision: 1 }),
    ).toThrow()
  })

  it('maps the document snapshot and tracks its revision as the ETag', () => {
    clearStoredBoardRevision(BOARD_ID)
    const snapshot = parseWorkspaceBoardSnapshot({
      data: {
        board_id: BOARD_ID,
        revision: 4,
        schema_version: 2,
        document: { schema_version: 2, payload: { nodes: [] } },
        updated_at: '2026-09-03T00:00:00Z',
      },
    })
    expect(snapshot.revision).toBe(4)
    trackBoardSummary({
      boardId: snapshot.boardId,
      workspaceId: WORKSPACE_ID,
      name: 'Atlas',
      revision: snapshot.revision,
      schemaVersion: snapshot.schemaVersion,
      archivedAt: null,
      createdAt: '',
      updatedAt: snapshot.updatedAt,
    })
    expect(getStoredBoardRevision(BOARD_ID)).toBe(4)
    setStoredBoardRevision(BOARD_ID, 5)
    expect(getStoredBoardRevision(BOARD_ID)).toBe(5)
  })
})

describe('workspace board concurrency headers', () => {
  beforeEach(() => {
    mocks.request.mockReset()
    clearStoredBoardRevision(BOARD_ID)
  })

  it('sends the stored revision as If-Match with an idempotency key', async () => {
    mocks.request.mockResolvedValueOnce(boardPayload({ revision: 3 }))
    setStoredBoardRevision(BOARD_ID, 2)
    const headers = buildBoardWriteHeaders({
      ifMatch: getStoredBoardRevision(BOARD_ID),
      idempotencyKey: 'board-doc:fixed-key',
      operation: 'document-replace',
    })
    await replaceWorkspaceBoardDocument(
      WORKSPACE_ID,
      BOARD_ID,
      { schema_version: 1, payload: {} },
      { ifMatch: headers['If-Match'], idempotencyKey: headers['Idempotency-Key'] },
    )
    const [, config] = mocks.request.mock.calls[0] as [string, Record<string, unknown>]
    const sent = config.headers as Record<string, string>
    expect(sent['If-Match']).toBe('2')
    expect(sent['Idempotency-Key']).toBe('board-doc:fixed-key')
    expect(sent['Request-Id']).toBeTruthy()
  })

  it('classifies stale writes as revision conflicts and refresh-then-retry reuses the key', async () => {
    const conflict = { status: 409, data: { error: { code: 'board_revision_conflict' } } }
    mocks.request.mockRejectedValueOnce(conflict)
    mocks.request.mockResolvedValueOnce(boardPayload({ revision: 3 }))
    const idempotencyKey = 'board-doc:retry-key'
    setStoredBoardRevision(BOARD_ID, 2)

    await expect(
      replaceWorkspaceBoardDocument(
        WORKSPACE_ID,
        BOARD_ID,
        { schema_version: 1, payload: {} },
        { ifMatch: getStoredBoardRevision(BOARD_ID), idempotencyKey },
      ),
    ).rejects.toMatchObject({ status: 409 })
    expect(isBoardRevisionConflictError(conflict)).toBe(true)

    // Refresh from the server, then retry with the SAME key so the server
    // replays instead of appending a duplicate revision.
    setStoredBoardRevision(BOARD_ID, 3)
    await replaceWorkspaceBoardDocument(
      WORKSPACE_ID,
      BOARD_ID,
      { schema_version: 1, payload: {} },
      { ifMatch: getStoredBoardRevision(BOARD_ID), idempotencyKey },
    )
    const keys = mocks.request.mock.calls.map(
      ([, config]) => (config as Record<string, Record<string, string>>).headers['Idempotency-Key'],
    )
    expect(keys).toEqual([idempotencyKey, idempotencyKey])
    const matches = mocks.request.mock.calls.map(
      ([, config]) => (config as Record<string, Record<string, string>>).headers['If-Match'],
    )
    expect(matches).toEqual(['2', '3'])
  })

  it('distinguishes idempotency conflicts from revision conflicts', () => {
    expect(
      isBoardIdempotencyConflictError({ status: 409, code: 'board_idempotency_conflict' }),
    ).toBe(true)
    expect(
      isBoardIdempotencyConflictError({ status: 409, code: 'board_revision_conflict' }),
    ).toBe(false)
    expect(isBoardRevisionConflictError({ status: 409, code: 'board_revision_conflict' })).toBe(true)
  })
})

describe('workspace board access branches', () => {
  it('keeps 404 hidden and 403 forbidden distinct', () => {
    expect(classifyBoardAccessError({ status: 404 })).toBe('hidden')
    expect(classifyBoardAccessError({ status: 403 })).toBe('forbidden')
    expect(
      classifyBoardAccessError({ status: 500, data: { error: { code: 'board_not_found' } } }),
    ).toBe('hidden')
    expect(classifyBoardAccessError({ status: 500, data: { code: 'board_forbidden' } })).toBe(
      'forbidden',
    )
    expect(classifyBoardAccessError({ status: 500 })).toBeNull()
  })
})

describe('workspace board idempotent retry', () => {
  beforeEach(() => {
    mocks.request.mockReset()
  })

  it('reuses the caller key across retries of the same attempt', async () => {
    mocks.request.mockResolvedValue(boardPayload({ id: 'board_new1' }))
    const idempotencyKey = 'board-create:attempt-1'
    await createWorkspaceBoard(WORKSPACE_ID, { name: 'Atlas' }, { idempotencyKey })
    await createWorkspaceBoard(WORKSPACE_ID, { name: 'Atlas' }, { idempotencyKey })
    const keys = mocks.request.mock.calls.map(
      ([, config]) => (config as Record<string, Record<string, string>>).headers['Idempotency-Key'],
    )
    expect(keys).toEqual([idempotencyKey, idempotencyKey])
  })
})

describe('workspace board query keys', () => {
  it('scopes caches under the workspace key root without duplicating selection', async () => {
    const { workspaceQueryKeys } = await import('../../../workspace/index')
    const keys = workspaceBoardQueryKeys(WORKSPACE_ID)
    expect(keys.all[0]).toBe(workspaceQueryKeys.all[0])
    expect(keys.all).toContain(WORKSPACE_ID)
    expect(keys.detail('board_1')).not.toEqual(workspaceBoardQueryKeys('ws_other').detail('board_1'))
    expect(keys.document(BOARD_ID)).toContain(BOARD_ID)
  })
})
