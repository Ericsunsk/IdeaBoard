import { describe, expect, test, vi } from 'vitest'
import {
  changeWorkspaceMemberRole,
  classifyWorkspaceMemberError,
  fetchWorkspaceMembers,
  invalidateWorkspaceMemberQueries,
  inviteWorkspaceMember,
  lookupWorkspaceMemberId,
  removeWorkspaceMemberEntry,
  workspaceMembersQueryOptions,
  WorkspaceMemberPayloadError,
} from './workspaceMembersResource'

const WORKSPACE_ID = 'ws_test123'

const mocks = vi.hoisted(() => ({
  listWorkspaceMembers: vi.fn(),
  lookupWorkspaceMemberByEmail: vi.fn(),
  addWorkspaceMember: vi.fn(),
  updateWorkspaceMember: vi.fn(),
  removeWorkspaceMember: vi.fn(),
}))

vi.mock('../../../api/generated-query/public/workspace-members/workspace-members', () => ({
  listWorkspaceMembers: mocks.listWorkspaceMembers,
  lookupWorkspaceMemberByEmail: mocks.lookupWorkspaceMemberByEmail,
  addWorkspaceMember: mocks.addWorkspaceMember,
  updateWorkspaceMember: mocks.updateWorkspaceMember,
  removeWorkspaceMember: mocks.removeWorkspaceMember,
  getListWorkspaceMembersQueryKey: (workspaceId: string) =>
    ['workspaces', workspaceId, 'members'] as const,
}))

function codedError(code: string) {
  return Object.assign(new Error('request failed'), { code })
}

describe('workspaceMembersResource reads', () => {
  test('lists the roster with display profiles and wire roles', async () => {
    mocks.listWorkspaceMembers.mockResolvedValue({
      data: [
        {
          user_id: 'usr_owner',
          role: 'owner',
          email: 'owner@example.com',
          avatar: ' /avatar.png ',
        },
        { user_id: 'usr_member1', role: 'member' },
      ],
    })

    await expect(fetchWorkspaceMembers(WORKSPACE_ID)).resolves.toEqual([
      {
        userId: 'usr_owner',
        role: 'owner',
        email: 'owner@example.com',
        avatar_url: '/avatar.png',
      },
      { userId: 'usr_member1', role: 'member' },
    ])
    expect(mocks.listWorkspaceMembers).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.not.objectContaining({ headers: expect.anything() }),
    )
  })

  test('omits blank or invalid display fields without failing the roster', async () => {
    mocks.listWorkspaceMembers.mockResolvedValue({
      data: [
        { user_id: 'usr_a', role: 'member', email: '  ', avatar: '  ' },
        { user_id: 'usr_b', role: 'admin', email: 42, avatar: 42 },
      ],
    })

    await expect(fetchWorkspaceMembers(WORKSPACE_ID)).resolves.toEqual([
      { userId: 'usr_a', role: 'member' },
      { userId: 'usr_b', role: 'admin' },
    ])
  })

  test('rejects roster rows leaking numeric identities or unknown roles', async () => {
    mocks.listWorkspaceMembers.mockResolvedValue({ data: [{ user_id: '42', role: 'member' }] })
    await expect(fetchWorkspaceMembers(WORKSPACE_ID)).rejects.toBeInstanceOf(
      WorkspaceMemberPayloadError,
    )

    mocks.listWorkspaceMembers.mockResolvedValue({ data: [{ user_id: 'usr_x', role: 'super' }] })
    await expect(fetchWorkspaceMembers(WORKSPACE_ID)).rejects.toBeInstanceOf(
      WorkspaceMemberPayloadError,
    )
  })

  test('looks up a registered user id by exact email', async () => {
    mocks.lookupWorkspaceMemberByEmail.mockResolvedValue({ data: { user_id: 'usr_new' } })

    await expect(lookupWorkspaceMemberId(WORKSPACE_ID, '  new@example.com ')).resolves.toBe(
      'usr_new',
    )
    expect(mocks.lookupWorkspaceMemberByEmail).toHaveBeenCalledWith(
      WORKSPACE_ID,
      { email: 'new@example.com' },
      expect.not.objectContaining({ headers: expect.anything() }),
    )
  })

  test('rejects blank lookup emails before any network call', async () => {
    mocks.lookupWorkspaceMemberByEmail.mockClear()

    await expect(lookupWorkspaceMemberId(WORKSPACE_ID, '   ')).rejects.toBeInstanceOf(
      WorkspaceMemberPayloadError,
    )
    expect(mocks.lookupWorkspaceMemberByEmail).not.toHaveBeenCalled()
  })

  test('rejects malformed workspace ids before any network call', async () => {
    mocks.listWorkspaceMembers.mockClear()

    await expect(fetchWorkspaceMembers('nope')).rejects.toBeInstanceOf(WorkspaceMemberPayloadError)
    expect(mocks.listWorkspaceMembers).not.toHaveBeenCalled()
  })
})

describe('workspaceMembersResource writes', () => {
  test('invite carries a per-attempt Idempotency-Key', async () => {
    mocks.addWorkspaceMember.mockResolvedValue({ data: { user_id: 'usr_new', role: 'member' } })

    const entry = await inviteWorkspaceMember(
      WORKSPACE_ID,
      { userId: 'usr_new', role: 'member' },
      { idempotencyKey: 'key-1' },
    )

    expect(entry).toEqual({ userId: 'usr_new', role: 'member' })
    expect(mocks.addWorkspaceMember).toHaveBeenCalledWith(
      WORKSPACE_ID,
      { data: { user_id: 'usr_new', role: 'member' } },
      expect.objectContaining({ headers: { 'Idempotency-Key': 'key-1' } }),
    )
  })

  test('invite mints a fresh scoped key per attempt when none is passed', async () => {
    mocks.addWorkspaceMember.mockClear()
    mocks.addWorkspaceMember.mockResolvedValue({ data: { user_id: 'usr_new', role: 'admin' } })

    await inviteWorkspaceMember(WORKSPACE_ID, { userId: 'usr_new', role: 'admin' })
    await inviteWorkspaceMember(WORKSPACE_ID, { userId: 'usr_new', role: 'admin' })

    const keys = mocks.addWorkspaceMember.mock.calls.map(
      (call) => (call[2] as { headers: Record<string, string> }).headers['Idempotency-Key'],
    )
    expect(keys).toHaveLength(2)
    for (const key of keys) {
      expect(key?.startsWith('workspace:members.invite:')).toBe(true)
    }
    expect(new Set(keys).size).toBe(2)
  })

  test('role change carries no Idempotency-Key', async () => {
    mocks.updateWorkspaceMember.mockResolvedValue({ data: { user_id: 'usr_x', role: 'admin' } })

    await changeWorkspaceMemberRole(WORKSPACE_ID, 'usr_x', { role: 'admin' })

    expect(mocks.updateWorkspaceMember).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'usr_x',
      { data: { role: 'admin' } },
      expect.not.objectContaining({ headers: expect.anything() }),
    )
  })

  test('removal carries a per-attempt Idempotency-Key', async () => {
    mocks.removeWorkspaceMember.mockResolvedValue({ data: {} })

    await removeWorkspaceMemberEntry(WORKSPACE_ID, 'usr_x', { idempotencyKey: 'key-3' })

    expect(mocks.removeWorkspaceMember).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'usr_x',
      expect.objectContaining({ headers: { 'Idempotency-Key': 'key-3' } }),
    )
  })

  test('writes fail closed before any network call', async () => {
    mocks.addWorkspaceMember.mockClear()
    mocks.updateWorkspaceMember.mockClear()
    mocks.removeWorkspaceMember.mockClear()

    await expect(
      inviteWorkspaceMember(WORKSPACE_ID, { userId: 'nope', role: 'member' }),
    ).rejects.toBeInstanceOf(WorkspaceMemberPayloadError)
    await expect(
      inviteWorkspaceMember(WORKSPACE_ID, { userId: 'usr_x', role: 'super' as 'member' }),
    ).rejects.toBeInstanceOf(WorkspaceMemberPayloadError)
    await expect(
      changeWorkspaceMemberRole(WORKSPACE_ID, 'usr_x', { role: 'owner ' as 'member' }),
    ).rejects.toBeInstanceOf(WorkspaceMemberPayloadError)
    await expect(removeWorkspaceMemberEntry(WORKSPACE_ID, '42')).rejects.toBeInstanceOf(
      WorkspaceMemberPayloadError,
    )

    expect(mocks.addWorkspaceMember).not.toHaveBeenCalled()
    expect(mocks.updateWorkspaceMember).not.toHaveBeenCalled()
    expect(mocks.removeWorkspaceMember).not.toHaveBeenCalled()
  })
})

describe('workspaceMembersResource errors', () => {
  test('classifies stable backend codes without matching prose', () => {
    expect(classifyWorkspaceMemberError(codedError('workspace_member_not_found'))).toBe(
      'unknown-user',
    )
    expect(classifyWorkspaceMemberError(codedError('workspace_not_found'))).toBe('hidden')
    expect(classifyWorkspaceMemberError(codedError('workspace_forbidden'))).toBe('forbidden')
    expect(classifyWorkspaceMemberError(codedError('workspace_member_limit_reached'))).toBe(
      'member-limit',
    )
    expect(classifyWorkspaceMemberError(codedError('membership_transition_forbidden'))).toBe(
      'last-owner',
    )
    expect(classifyWorkspaceMemberError(codedError('workspace_idempotency_conflict'))).toBe(
      'idempotency-conflict',
    )
    expect(classifyWorkspaceMemberError(codedError('validation_failed'))).toBe('invalid')
    expect(classifyWorkspaceMemberError(codedError('bad_request'))).toBe('invalid')
    expect(classifyWorkspaceMemberError(codedError('idempotency_key_required'))).toBe('invalid')
    expect(classifyWorkspaceMemberError(new Error('boom'))).toBe('sync-failed')
  })

  test('reuses the generated roster key so Share holds no second cache', () => {
    expect(workspaceMembersQueryOptions(WORKSPACE_ID).queryKey).toEqual([
      'workspaces',
      WORKSPACE_ID,
      'members',
    ])
  })

  test('query options fail fast and stay disabled without a workspace id', () => {
    const enabled = workspaceMembersQueryOptions(WORKSPACE_ID)
    expect(enabled.retry).toBe(false)
    expect(enabled.enabled.value).toBe(true)

    const disabled = workspaceMembersQueryOptions('   ')
    expect(disabled.enabled.value).toBe(false)
  })

  test('invalidation targets the exact roster key of one workspace', () => {
    const client = { invalidateQueries: vi.fn() }

    invalidateWorkspaceMemberQueries(client, WORKSPACE_ID)

    const [filters] = client.invalidateQueries.mock.calls[0] ?? []
    expect(filters).toMatchObject({ queryKey: ['workspaces', WORKSPACE_ID, 'members'] })
    expect((filters as Record<string, unknown> | undefined)?.['exact']).toBe(true)
  })
})
