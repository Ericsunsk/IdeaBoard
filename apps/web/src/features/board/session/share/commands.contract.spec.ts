import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient } from '@tanstack/vue-query'
import { primeAuthenticatedWorkspace, selectWorkspace } from '../../testing'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import {
  captureShareScope,
  classifyShareError,
  isShareScopeCurrent,
  mintShareAttempt,
  replaceShareAccess,
  type ShareTransport,
} from './commands'
import type { WorkspaceBoardAccessFact } from '../adapters/boardsResource'

const WORKSPACE = 'ws_share123'
const BOARD = 'board_share1'
const config: WorkspaceBoardAccessFact = {
  workspaceAccess: 'viewer',
  memberUserIds: ['usr_member'],
  accessVersion: 3,
  accessState: 'workspace',
}

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const transport: ShareTransport = {
    fetchMeta: vi.fn().mockResolvedValue({ id: BOARD }),
    fetchAccess: vi.fn().mockResolvedValue(config),
    fetchRoster: vi.fn().mockResolvedValue([]),
    fetchBoardCreatorIdentity: vi.fn().mockResolvedValue('usr_creator'),
    replaceAccess: vi.fn().mockResolvedValue(config),
  }
  const scope = captureShareScope(BOARD)
  if (!scope) throw new Error('missing scope')
  return { client, transport, scope }
}

beforeEach(() => {
  primeAuthenticatedWorkspace(WORKSPACE)
})

describe('single sharing resource commands', () => {
  test('replaces the complete selected set with a common role and exact attempt credentials', async () => {
    const { transport, client, scope } = setup()
    const attempt = mintShareAttempt(2)
    const input = { workspaceAccess: 'viewer' as const, memberUserIds: ['usr_member'] }
    expect(await replaceShareAccess({ transport, client }, scope, attempt, input)).toEqual({
      isOk: true,
      access: config,
    })
    expect(transport.replaceAccess).toHaveBeenCalledExactlyOnceWith(WORKSPACE, BOARD, input, {
      expectedVersion: 2,
      idempotencyKey: attempt.key,
    })
    expect(client.getQueryData(boardQueryKeys.access(WORKSPACE, BOARD))).toEqual(config)
  })

  test('unknown outcome retries retain the provided key and observed version', async () => {
    const { transport, client, scope } = setup()
    vi.mocked(transport.replaceAccess).mockRejectedValueOnce(new Error('network'))
    const attempt = mintShareAttempt(2)
    const input = { workspaceAccess: 'none' as const, memberUserIds: [] }
    expect(await replaceShareAccess({ transport, client }, scope, attempt, input)).toMatchObject({
      isOk: false,
      reason: 'sync-failed',
    })
    await replaceShareAccess({ transport, client }, scope, attempt, input)
    expect(vi.mocked(transport.replaceAccess).mock.calls.map((call) => call[3])).toEqual([
      { expectedVersion: 2, idempotencyKey: attempt.key },
      { expectedVersion: 2, idempotencyKey: attempt.key },
    ])
  })

  test('A→B→A drops a late accepted write without seeding the fresh residency cache', async () => {
    const { transport, client, scope } = setup()
    let resolve!: (access: WorkspaceBoardAccessFact) => void
    vi.mocked(transport.replaceAccess).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const operation = replaceShareAccess({ transport, client }, scope, mintShareAttempt(2), config)
    await vi.waitFor(() => expect(transport.replaceAccess).toHaveBeenCalledOnce())
    selectWorkspace('ws_share456')
    selectWorkspace(WORKSPACE)
    const fresh = { ...config, accessVersion: 10, memberUserIds: [] }
    client.setQueryData(boardQueryKeys.access(WORKSPACE, BOARD), fresh)
    resolve(config)
    expect(await operation).toEqual({ isOk: false, reason: 'scope-changed' })
    expect(isShareScopeCurrent(scope)).toBe(false)
    expect(client.getQueryData(boardQueryKeys.access(WORKSPACE, BOARD))).toEqual(fresh)
  })

  test('stale configuration conflicts are surfaced without retrying against a new version', async () => {
    const { transport, client, scope } = setup()
    vi.mocked(transport.replaceAccess).mockRejectedValue(
      Object.assign(new Error('conflict'), { code: 'board_access_conflict' }),
    )
    expect(
      await replaceShareAccess({ transport, client }, scope, mintShareAttempt(2), config),
    ).toMatchObject({ isOk: false, reason: 'access-conflict' })
    expect(transport.replaceAccess).toHaveBeenCalledOnce()
  })

  test('classifies authority loss separately from a stale write', () => {
    expect(classifyShareError({ code: 'board_forbidden' })).toBe('forbidden')
    expect(classifyShareError({ code: 'board_access_conflict' })).toBe('access-conflict')
    expect(classifyShareError(new Error('network'))).toBe('sync-failed')
  })
})
