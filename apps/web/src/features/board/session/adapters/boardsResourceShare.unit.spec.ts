import { beforeEach, describe, expect, test, vi } from 'vitest'
import { primeAuthenticatedWorkspace } from '../../testing'
import { BoardPayloadError, fetchBoardAccessFact, replaceBoardSharing } from './boardsResource'

const WORKSPACE = 'ws_test123'
const BOARD = 'board_share1'
const mocks = vi.hoisted(() => ({
  getBoardAccess: vi.fn(),
  updateBoardAccess: vi.fn(),
}))
vi.mock('../../../../api/generated-query/public/boards/boards', () => ({
  getBoardAccess: mocks.getBoardAccess,
  updateBoardAccess: mocks.updateBoardAccess,
}))
const fact = {
  workspace_id: WORKSPACE,
  board_id: BOARD,
  workspace_access: 'viewer',
  member_user_ids: ['usr_member'],
  access_version: 3,
  access_state: 'workspace',
}

beforeEach(() => {
  vi.clearAllMocks()
  primeAuthenticatedWorkspace(WORKSPACE)
})

describe('Board sharing transport', () => {
  test('reads the common role, exact selected IDs, version and final visibility', async () => {
    mocks.getBoardAccess.mockResolvedValue({ data: fact })
    expect(await fetchBoardAccessFact(WORKSPACE, BOARD)).toEqual({
      workspaceAccess: 'viewer',
      memberUserIds: ['usr_member'],
      accessVersion: 3,
      accessState: 'workspace',
    })
  })

  test.each([
    { access_version: 0 },
    { member_user_ids: null },
    { member_user_ids: ['not-a-user'] },
    { access_state: 'unknown' },
  ])('rejects malformed authority facts %s', async (invalid) => {
    mocks.getBoardAccess.mockResolvedValue({ data: { ...fact, ...invalid } })
    await expect(fetchBoardAccessFact(WORKSPACE, BOARD)).rejects.toBeInstanceOf(BoardPayloadError)
  })

  test('one PUT carries the full selection with optimistic concurrency and idempotency', async () => {
    mocks.updateBoardAccess.mockResolvedValue({
      data: { ...fact, workspace_access: 'none', member_user_ids: [], access_state: 'private' },
    })
    const result = await replaceBoardSharing(
      WORKSPACE,
      BOARD,
      { workspaceAccess: 'none', memberUserIds: [] },
      { expectedVersion: 2, idempotencyKey: 'share-operation' },
    )
    expect(result.memberUserIds).toEqual([])
    expect(mocks.updateBoardAccess).toHaveBeenCalledExactlyOnceWith(
      WORKSPACE,
      BOARD,
      { data: { workspace_access: 'none', member_user_ids: [] } },
      { headers: { 'If-Match': 'access-2', 'Idempotency-Key': 'share-operation' } },
    )
  })
})
