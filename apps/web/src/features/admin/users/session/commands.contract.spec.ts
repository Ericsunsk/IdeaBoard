import { describe, expect, it, vi } from 'vitest'
import { useAdminUserCommands } from './commands'
import { makeAdminUserCommandTarget } from '../../../../test/factories'

describe('admin user commands', () => {
  it('opens the ban dialog for active users and submits the ban payload', async () => {
    const harness = createHarness()
    const user = makeAdminUserCommandTarget({ publicId: 'user-7', displayName: 'alice', status: 1 })
    const commands = harness.createCommands()

    commands.openUserActionMenu(user)
    commands.handleActionSelect('status')
    commands.banForm.value = { expiresInDays: 7, reason: 'abuse' }
    await commands.confirmBanUser()

    expect(commands.showBanModal.value).toBe(false)
    expect(harness.mutations.banUser.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-7',
      reason: 'abuse',
      expires_in_days: 7,
      operatorReason: 'abuse',
    })
    expect(harness.feedback.banSucceeded).toHaveBeenCalledWith('alice')
  })

  it('refuses ban without an operator reason before calling mutations', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeAdminUserCommandTarget({ publicId: 'user-7', status: 1 }))
    commands.handleActionSelect('status')
    commands.banForm.value = { expiresInDays: 0, reason: '   ' }
    await commands.confirmBanUser()

    expect(harness.mutations.banUser.mutateAsync).not.toHaveBeenCalled()
    expect(commands.showBanModal.value).toBe(true)
    expect(harness.feedback.banFailed).toHaveBeenCalledWith(expect.any(Error))
  })

  it('blocks status changes for admins before calling mutations', () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeAdminUserCommandTarget({ role: 'admin', status: 1 }))
    commands.handleActionSelect('status')

    expect(harness.feedback.adminStatusChangeForbidden).toHaveBeenCalled()
    expect(harness.mutations.banUser.mutateAsync).not.toHaveBeenCalled()
  })

  it('blocks admin delete before confirmation and mutation side effects', () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeAdminUserCommandTarget({ role: 'admin', displayName: 'root' }))
    commands.handleActionSelect('delete')

    expect(harness.feedback.adminStatusChangeForbidden).toHaveBeenCalled()
    expect(harness.confirm.confirm).not.toHaveBeenCalled()
    expect(harness.mutations.deleteUser.mutateAsync).not.toHaveBeenCalled()
  })

  it('confirms unban with an operator reason and reports mutation failures as observable errors', async () => {
    const harness = createHarness()
    harness.mutations.unbanUser.mutateAsync.mockRejectedValueOnce(new Error('backend denied'))
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-3', status: 0, displayName: 'bob' }),
    )
    commands.handleActionSelect('status')
    await harness.settleConfirms()

    expect(harness.confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认解除封禁',
        confirmLabel: '确认解封',
        reason: expect.objectContaining({ isRequired: true }),
      }),
    )
    expect(harness.mutations.unbanUser.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-3',
      operatorReason: 'test reason',
    })
    expect(harness.feedback.unbanFailed).toHaveBeenCalledWith(expect.any(Error))
  })

  it('confirms role toggles with the next role payload and operator reason', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-4', role: 'user', displayName: 'cara' }),
    )
    commands.handleActionSelect('role')
    await harness.settleConfirms()

    expect(harness.confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认设为管理员' }),
    )
    expect(harness.mutations.updateRole.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-4',
      role: 'admin',
      operatorReason: 'test reason',
    })
    expect(harness.feedback.roleUpdated).toHaveBeenCalledWith('admin')
  })

  it('keeps admin role demotion as an intentional confirmed action', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-8', role: 'admin', displayName: 'root' }),
    )
    commands.handleActionSelect('role')
    await harness.settleConfirms()

    expect(harness.feedback.adminStatusChangeForbidden).not.toHaveBeenCalled()
    expect(harness.mutations.updateRole.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-8',
      role: 'user',
      operatorReason: 'test reason',
    })
    expect(harness.feedback.roleUpdated).toHaveBeenCalledWith('user')
  })

  it('confirms session revoke with an operator reason using the public user identity', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-5', displayName: 'dana' }),
    )
    commands.handleActionSelect('revoke')
    await harness.settleConfirms()

    expect(harness.feedback.missingAuthUserForSessionRevoke).not.toHaveBeenCalled()
    expect(harness.mutations.revokeSessions.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-5',
      operatorReason: 'test reason',
    })
  })

  it('confirms impersonation with danger copy and an operator reason', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-6', displayName: 'erin' }),
    )
    commands.handleActionSelect('impersonate')
    await vi.waitFor(() =>
      expect(harness.sessionCommands.startImpersonation).toHaveBeenCalledWith('user-6', {
        operatorReason: 'test reason',
      }),
    )

    expect(harness.confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认伪装登录',
        intent: 'danger',
        reason: expect.objectContaining({ isRequired: true }),
      }),
    )
    expect(harness.feedback.impersonationStarted).toHaveBeenCalledWith('erin')
  })

  it('delegates impersonation without checking private auth identity fields', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeAdminUserCommandTarget({ publicId: 'user-9', displayName: 'fran' }),
    )
    commands.handleActionSelect('impersonate')
    await vi.waitFor(() =>
      expect(harness.sessionCommands.startImpersonation).toHaveBeenCalledWith('user-9', {
        operatorReason: 'test reason',
      }),
    )
  })
})

function createHarness() {
  const feedback = createAdminUsersFeedback()
  const confirm = {
    confirm: vi.fn().mockResolvedValue(true),
    readReason: vi.fn().mockReturnValue('test reason'),
  }
  const mutations = {
    banUser: { mutateAsync: vi.fn().mockResolvedValue(undefined) },
    unbanUser: { mutateAsync: vi.fn().mockResolvedValue(undefined) },
    revokeSessions: { mutateAsync: vi.fn().mockResolvedValue(undefined) },
    updateRole: { mutateAsync: vi.fn().mockResolvedValue(undefined) },
    deleteUser: { mutateAsync: vi.fn().mockResolvedValue(undefined) },
  }
  const sessionCommands = {
    startImpersonation: vi.fn().mockResolvedValue(undefined),
  }

  return {
    feedback,
    confirm,
    mutations,
    sessionCommands,
    createCommands: () =>
      useAdminUserCommands(
        { feedback, confirm },
        {
          mutations,
          sessionCommands,
        },
      ),
    settleConfirms: async () => {
      await vi.waitFor(() => expect(confirm.confirm).toHaveBeenCalled())
      await new Promise((resolve) => setTimeout(resolve, 0))
    },
  }
}

function createAdminUsersFeedback() {
  return {
    loadFailed: vi.fn(),
    adminStatusChangeForbidden: vi.fn(),
    unbanSucceeded: vi.fn(),
    unbanFailed: vi.fn(),
    banSucceeded: vi.fn(),
    banFailed: vi.fn(),
    missingAuthUserForSessionRevoke: vi.fn(),
    revokeSessionsSucceeded: vi.fn(),
    revokeSessionsFailed: vi.fn(),
    missingAuthUserForImpersonation: vi.fn(),
    impersonationStarted: vi.fn(),
    impersonationFailed: vi.fn(),
    userDeleted: vi.fn(),
    userDeleteFailed: vi.fn(),
    roleUpdated: vi.fn(),
    roleUpdateFailed: vi.fn(),
  }
}
