import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { setStoredAuth } from '../../../../core/utils/auth'
import { getWorkspaceSelectionStore } from '../../../workspace'
import { primeAuthenticatedWorkspace } from '../../testing'
import {
  BoardCatalogIncompleteError,
  BoardCatalogScopeChangedError,
  BoardPayloadError,
  createWorkspaceBoard,
  fetchWorkspaceBoards,
} from './boardsResource'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listBoards: vi.fn(),
  getBoardDocument: vi.fn(),
  createBoard: vi.fn(),
}))

vi.mock('../../../../api/generated-query/public/boards/boards', () => ({
  listBoards: mocks.listBoards,
  getBoardDocument: mocks.getBoardDocument,
  createBoard: mocks.createBoard,
}))

function loadFixtureScene(name: string): unknown {
  const raw = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      '../../../../../../api/internal/boards/previewscene/testdata',
      `${name}.json`,
    ),
    'utf8',
  )
  return (JSON.parse(raw) as { scene: unknown }).scene
}

function boardDto(extra: Record<string, unknown> = {}) {
  return {
    id: 'board-1',
    workspace_id: WORKSPACE_ID,
    name: 'Scene Board',
    revision: 3,
    schema_version: 4,
    creator_user_id: 'usr_creator',
    archived_at: null,
    created_at: new Date(100).toISOString(),
    updated_at: new Date(200).toISOString(),
    ...extra,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  primeAuthenticatedWorkspace(WORKSPACE_ID)
  mocks.listBoards.mockResolvedValue({ data: [], meta: { has_more: false } })
})

describe('boardsResource catalog scenes', () => {
  test('retains the Creator public identity for cached Board consumers', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [boardDto({ creator_user_id: 'usr_other_creator' })],
      meta: { has_more: false },
    })
    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)
    expect(meta?.creatorUserId).toBe('usr_other_creator')
  })

  test.each([undefined, '', 'not-a-user'])(
    'rejects invalid Creator identity %s',
    async (creator) => {
      mocks.listBoards.mockResolvedValue({
        data: [boardDto({ creator_user_id: creator })],
        meta: { has_more: false },
      })
      await expect(fetchWorkspaceBoards(WORKSPACE_ID)).rejects.toBeInstanceOf(BoardPayloadError)
    },
  )

  test('carries the strict-parsed server scene and content version', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [boardDto({ preview_scene: loadFixtureScene('mixed'), content_version: 'cv-1' })],
      meta: { has_more: false },
    })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.contentVersion).toBe('cv-1')
    // The scene keeps its own verbatim identity (extraction never derives
    // it); the envelope identity rides beside it for skip-re-render.
    expect(meta?.previewScene?.contentVersion).toBe('cv-fixture-mixed-01')
    expect(meta?.previewScene?.nodes.map((node) => node.id)).toEqual([
      'image-1',
      'prompt-2',
      'fusion-4',
    ])
    expect(mocks.getBoardDocument).not.toHaveBeenCalled()
  })

  test('omitted scenes stay null with an empty content version', async () => {
    mocks.listBoards.mockResolvedValue({ data: [boardDto()], meta: { has_more: false } })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.contentVersion).toBe('')
    expect(meta?.previewScene).toBeNull()
    expect(meta?.revision).toBe(3)
  })

  test('invalid scenes fail closed without breaking the meta', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [
        boardDto({ preview_scene: { version: 2 }, content_version: 'cv-bad' }),
        boardDto({
          id: 'board-2',
          preview_scene: { version: 1, unexpected_scene_field: 'note' },
          content_version: 42,
        }),
      ],
      meta: { has_more: false },
    })

    const metas = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(metas).toHaveLength(2)
    // The wire identity is carried verbatim when present...
    expect(metas[0]?.contentVersion).toBe('cv-bad')
    expect(metas[0]?.previewScene).toBeNull()
    // ...but a non-string content version never becomes an identity.
    expect(metas[1]?.contentVersion).toBe('')
    expect(metas[1]?.previewScene).toBeNull()
  })
})

describe('boardsResource access facts', () => {
  test('consumes server capabilities verbatim without recomputing roles', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [
        boardDto({
          workspace_access: 'viewer',
          access_state: 'workspace',
          relationship: { is_creator: false, has_direct_grant: false },
          capabilities: {
            read: true,
            write: false,
            manage: false,
            generate: false,
          },
        }),
      ],
      meta: { has_more: false },
    })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.capabilities).toEqual({
      read: true,
      write: false,
      manage: false,
      generate: false,
    })
    expect(meta?.accessState).toBe('workspace')
    expect(meta?.workspaceAccess).toBe('viewer')
    expect(meta?.isCreator).toBe(false)
  })

  test('missing capability facts fail closed to read-only', async () => {
    mocks.listBoards.mockResolvedValue({ data: [boardDto()], meta: { has_more: false } })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.capabilities?.write).toBe(false)
    expect(meta?.capabilities?.read).toBe(true)
    expect(meta?.accessState).toBe('private')
    expect(meta?.workspaceAccess).toBe('none')
  })

  test('unknown access facts collapse to the closed posture', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [
        boardDto({
          workspace_access: 'superadmin',
          access_state: 'shared-with-everyone',
          capabilities: {
            read: true,
            write: 'yes',
            manage: false,
            generate: false,
          },
        }),
      ],
      meta: { has_more: false },
    })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.workspaceAccess).toBe('none')
    expect(meta?.accessState).toBe('private')
    expect(meta?.capabilities?.write).toBe(false)
  })
})

describe('boardsResource create', () => {
  test('creation sends a name-only body: the server fixes workspace_access=none', async () => {
    mocks.createBoard.mockResolvedValue({
      data: boardDto({
        name: 'Fresh',
        workspace_access: 'none',
        access_state: 'private',
        relationship: { is_creator: true, has_direct_grant: false },
        capabilities: {
          read: true,
          write: true,
          manage: true,
          generate: true,
        },
      }),
    })

    const meta = await createWorkspaceBoard(WORKSPACE_ID, 'Fresh')

    expect(mocks.createBoard).toHaveBeenCalledTimes(1)
    const [workspace, body, config] = mocks.createBoard.mock.calls[0] as [
      string,
      Record<string, unknown>,
      { headers?: Record<string, string> },
    ]
    expect(workspace).toBe(WORKSPACE_ID)
    // No ACL, grant, or visibility field may ride creation: the server
    // owns the none default and copies inherit nothing.
    expect(body).toEqual({ data: { name: 'Fresh' } })
    expect(config.headers?.['Idempotency-Key']).toMatch(/^.+$/)
    expect(meta.workspaceAccess).toBe('none')
    expect(meta.isCreator).toBe(true)
  })
})

describe('boardsResource range facts', () => {
  test('carries has_direct_grant for the personal-range derivation', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [
        boardDto({ relationship: { is_creator: false, has_direct_grant: true } }),
        boardDto({ id: 'board-2', relationship: { is_creator: true, has_direct_grant: false } }),
      ],
      meta: { has_more: false },
    })

    const metas = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(metas.map((meta) => meta.hasDirectGrant)).toEqual([true, false])
  })

  test('missing relationship facts fail closed without a grant', async () => {
    mocks.listBoards.mockResolvedValue({ data: [boardDto()], meta: { has_more: false } })

    const [meta] = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(meta?.hasDirectGrant).toBe(false)
    expect(meta?.isCreator).toBe(false)
  })
})

describe('boardsResource catalog walk', () => {
  test('aggregates every page into one complete walk', async () => {
    mocks.listBoards
      .mockResolvedValueOnce({
        data: [boardDto()],
        meta: { has_more: true, next_cursor: 'cursor-2' },
      })
      .mockResolvedValueOnce({
        data: [boardDto({ id: 'board-2' })],
        meta: { has_more: false, next_cursor: null },
      })

    const metas = await fetchWorkspaceBoards(WORKSPACE_ID)

    expect(metas.map((meta) => meta.id)).toEqual(['board-1', 'board-2'])
    expect(mocks.listBoards).toHaveBeenCalledTimes(2)
    expect(mocks.listBoards.mock.calls[0]?.[1]).not.toHaveProperty('cursor')
    expect(mocks.listBoards.mock.calls[1]?.[1]).toMatchObject({ cursor: 'cursor-2' })
    expect(mocks.getBoardDocument).not.toHaveBeenCalled()
  })

  test('rejects partial page failures instead of returning a partial list', async () => {
    mocks.listBoards
      .mockResolvedValueOnce({
        data: [boardDto()],
        meta: { has_more: true, next_cursor: 'cursor-2' },
      })
      .mockRejectedValueOnce(new Error('page two failed'))

    await expect(fetchWorkspaceBoards(WORKSPACE_ID)).rejects.toThrow('page two failed')
  })

  test('rejects an unbounded walk as explicitly incomplete', async () => {
    mocks.listBoards.mockResolvedValue({
      data: [boardDto()],
      meta: { has_more: true, next_cursor: 'cursor-next' },
    })

    const failure = await fetchWorkspaceBoards(WORKSPACE_ID).then(
      () => null,
      (error: unknown) => error,
    )
    expect(failure).toBeInstanceOf(BoardCatalogIncompleteError)
    expect(mocks.listBoards).toHaveBeenCalledTimes(50)
  })

  test('aborts the walk when the workspace selection moves mid-walk', async () => {
    mocks.listBoards.mockImplementationOnce(async () => {
      getWorkspaceSelectionStore().select('ws_moved999')
      return {
        data: [boardDto()],
        meta: { has_more: true, next_cursor: 'cursor-2' },
      }
    })

    const failure = await fetchWorkspaceBoards(WORKSPACE_ID).then(
      () => null,
      (error: unknown) => error,
    )
    expect(failure).toBeInstanceOf(BoardCatalogScopeChangedError)
    expect(mocks.listBoards).toHaveBeenCalledTimes(1)
  })

  test('aborts the walk when the caller identity moves mid-walk', async () => {
    mocks.listBoards.mockImplementationOnce(async () => {
      setStoredAuth({
        public_id: '00000000-0000-0000-0000-000000000002',
        display_name: 'other',
        avatar: '',
        role: 'user',
      })
      return {
        data: [boardDto()],
        meta: { has_more: true, next_cursor: 'cursor-2' },
      }
    })

    const failure = await fetchWorkspaceBoards(WORKSPACE_ID).then(
      () => null,
      (error: unknown) => error,
    )
    expect(failure).toBeInstanceOf(BoardCatalogScopeChangedError)
    expect(mocks.listBoards).toHaveBeenCalledTimes(1)
  })
})
