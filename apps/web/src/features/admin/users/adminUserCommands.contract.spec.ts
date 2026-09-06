import { describe, expect, it, vi } from 'vitest'
import { useAdminUserCommands } from './adminUserCommands'
import type { AdminUserCommandTarget } from './adminUserView'

type ConfirmRequest = {
  title: string
  message: string
  confirmLabel: string
  intent: 'warn' | 'danger'
  onConfirm: () => void | Promise<void>
}

describe('admin user commands', () => {
  it('opens the ban dialog for active users and submits the ban payload', async () => {
    const harness = createHarness()
    const user = makeUser({ publicId: 'user-7', displayName: 'alice', status: 1 })
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
    })
    expect(harness.feedback.banSucceeded).toHaveBeenCalledWith('alice')
  })

  it('blocks status changes for admins before calling mutations', () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ role: 'admin', status: 1 }))
    commands.handleActionSelect('status')

    expect(harness.feedback.adminStatusChangeForbidden).toHaveBeenCalled()
    expect(harness.mutations.banUser.mutateAsync).not.toHaveBeenCalled()
  })

  it('blocks admin delete before confirmation and mutation side effects', () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ role: 'admin', displayName: 'root' }))
    commands.handleActionSelect('delete')

    expect(harness.feedback.adminStatusChangeForbidden).toHaveBeenCalled()
    expect(harness.confirm.request).not.toHaveBeenCalled()
    expect(harness.mutations.deleteUser.mutateAsync).not.toHaveBeenCalled()
  })

  it('confirms unban and reports mutation failures as observable errors', async () => {
    const harness = createHarness()
    harness.mutations.unbanUser.mutateAsync.mockRejectedValueOnce(new Error('backend denied'))
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ publicId: 'user-3', status: 0, displayName: 'bob' }))
    commands.handleActionSelect('status')
    await harness.acceptLastConfirm()

    expect(harness.confirm.request).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认解除封禁', confirmLabel: '确认解封' }),
    )
    expect(harness.mutations.unbanUser.mutateAsync).toHaveBeenCalledWith('user-3')
    expect(harness.feedback.unbanFailed).toHaveBeenCalledWith(expect.any(Error))
  })

  it('confirms role toggles with the next role payload', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ publicId: 'user-4', role: 'user', displayName: 'cara' }))
    commands.handleActionSelect('role')
    await harness.acceptLastConfirm()

    expect(harness.confirm.request).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认设为管理员' }),
    )
    expect(harness.mutations.updateRole.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-4',
      role: 'admin',
    })
    expect(harness.feedback.roleUpdated).toHaveBeenCalledWith('admin')
  })

  it('keeps admin role demotion as an intentional confirmed action', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(
      makeUser({ publicId: 'user-8', role: 'admin', displayName: 'root' }),
    )
    commands.handleActionSelect('role')
    await harness.acceptLastConfirm()

    expect(harness.feedback.adminStatusChangeForbidden).not.toHaveBeenCalled()
    expect(harness.mutations.updateRole.mutateAsync).toHaveBeenCalledWith({
      publicId: 'user-8',
      role: 'user',
    })
    expect(harness.feedback.roleUpdated).toHaveBeenCalledWith('user')
  })

  it('confirms session revoke using the public user identity', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ publicId: 'user-5', displayName: 'dana' }))
    commands.handleActionSelect('revoke')
    await harness.acceptLastConfirm()

    expect(harness.feedback.missingAuthUserForSessionRevoke).not.toHaveBeenCalled()
    expect(harness.mutations.revokeSessions.mutateAsync).toHaveBeenCalledWith('user-5')
  })

  it('delegates impersonation side effects to the session command adapter', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ publicId: 'user-6', displayName: 'erin' }))
    commands.handleActionSelect('impersonate')
    await Promise.resolve()

    expect(harness.sessionCommands.startImpersonation).toHaveBeenCalledWith('user-6')
    expect(harness.feedback.impersonationStarted).toHaveBeenCalledWith('erin')
  })

  it('delegates impersonation without checking private auth identity fields', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ publicId: 'user-9', displayName: 'fran' }))
    commands.handleActionSelect('impersonate')
    await Promise.resolve()

    expect(harness.sessionCommands.startImpersonation).toHaveBeenCalledWith('user-9')
  })
})

function createHarness() {
  const feedback = createAdminUsersFeedback()
  const confirmRequests: ConfirmRequest[] = []
  const confirm = {
    request: vi.fn((request: ConfirmRequest) => confirmRequests.push(request)),
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
    acceptLastConfirm: async () => {
      const request = confirmRequests.at(-1)
      expect(request).toBeTruthy()
      await request?.onConfirm()
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

function makeUser(overrides: Partial<AdminUserCommandTarget> = {}): AdminUserCommandTarget {
  return {
    publicId: 'user-1',
    displayName: 'user',
    role: 'user',
    status: 1,
    ...overrides,
  }
}
