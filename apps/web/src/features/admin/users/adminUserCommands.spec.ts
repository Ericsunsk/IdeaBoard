import { describe, expect, it, vi } from 'vitest'
import { useAdminUserCommands } from './adminUserCommands'
import type { AdminUserCommandTarget } from './adminUserView'

type ConfirmRequest = {
  header: string
  message: string
  acceptLabel: string
  severity: 'warn' | 'danger'
  accept: () => void | Promise<void>
}

describe('admin user commands', () => {
  it('opens the ban dialog for active users and submits the ban payload', async () => {
    const harness = createHarness()
    const user = makeUser({ id: 7, username: 'alice', status: 1 })
    const commands = harness.createCommands()

    commands.openUserActionMenu(user)
    commands.handleActionSelect('status')
    commands.banForm.value = { expiresInDays: 7, reason: 'abuse' }
    await commands.confirmBanUser()

    expect(commands.showBanModal.value).toBe(false)
    expect(harness.mutations.banUser.mutateAsync).toHaveBeenCalledWith({
      id: 7,
      reason: 'abuse',
      expires_in_days: 7,
    })
    expect(harness.message.success).toHaveBeenCalledWith('已高级封禁用户 alice')
  })

  it('blocks status changes for admins before calling mutations', () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ role: 'admin', status: 1 }))
    commands.handleActionSelect('status')

    expect(harness.message.warning).toHaveBeenCalledWith('管理员账号不可封禁或解封')
    expect(harness.mutations.banUser.mutateAsync).not.toHaveBeenCalled()
  })

  it('confirms unban and reports mutation failures as observable errors', async () => {
    const harness = createHarness()
    harness.mutations.unbanUser.mutateAsync.mockRejectedValueOnce(new Error('backend denied'))
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ id: 3, status: 0, username: 'bob' }))
    commands.handleActionSelect('status')
    await harness.acceptLastConfirm()

    expect(harness.confirm.require).toHaveBeenCalledWith(
      expect.objectContaining({ header: '确认解除封禁', acceptLabel: '确认解封' }),
    )
    expect(harness.mutations.unbanUser.mutateAsync).toHaveBeenCalledWith(3)
    expect(harness.message.error).toHaveBeenCalledWith('backend denied')
  })

  it('confirms role toggles with the next role payload', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ id: 4, role: 'user', username: 'cara' }))
    commands.handleActionSelect('role')
    await harness.acceptLastConfirm()

    expect(harness.confirm.require).toHaveBeenCalledWith(
      expect.objectContaining({ header: '确认设为管理员' }),
    )
    expect(harness.mutations.updateRole.mutateAsync).toHaveBeenCalledWith({
      id: 4,
      role: 'admin',
    })
    expect(harness.message.success).toHaveBeenCalledWith('已设为管理员')
  })

  it('confirms session revoke only when the user has an auth identity', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ authUserId: '' }))
    commands.handleActionSelect('revoke')
    commands.openUserActionMenu(makeUser({ id: 5, authUserId: 'auth-5', username: 'dana' }))
    commands.handleActionSelect('revoke')
    await harness.acceptLastConfirm()

    expect(harness.message.warning).toHaveBeenCalledWith('目标用户缺少 Auth 标识，无法吊销会话')
    expect(harness.mutations.revokeSessions.mutateAsync).toHaveBeenCalledWith(5)
    expect(harness.message.success).toHaveBeenCalledWith('已强制吊销所有会话')
  })

  it('delegates impersonation side effects to the session command adapter', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ authUserId: 'auth-6', username: 'erin' }))
    commands.handleActionSelect('impersonate')
    await Promise.resolve()

    expect(harness.sessionCommands.startImpersonation).toHaveBeenCalledWith('auth-6')
    expect(harness.message.success).toHaveBeenCalledWith('正在以 erin 的身份登录...')
  })

  it('shows an impersonation error before adapter work when auth identity is missing', async () => {
    const harness = createHarness()
    const commands = harness.createCommands()

    commands.openUserActionMenu(makeUser({ authUserId: '' }))
    commands.handleActionSelect('impersonate')
    await Promise.resolve()

    expect(harness.sessionCommands.startImpersonation).not.toHaveBeenCalled()
    expect(harness.message.error).toHaveBeenCalledWith('目标用户缺少 Auth 标识，无法伪装登录')
  })
})

function createHarness() {
  const message = {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  }
  const confirmRequests: ConfirmRequest[] = []
  const confirm = {
    require: vi.fn((request: ConfirmRequest) => confirmRequests.push(request)),
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
    message,
    confirm,
    mutations,
    sessionCommands,
    createCommands: () =>
      useAdminUserCommands(
        { message, confirm },
        {
          mutations,
          sessionCommands,
        },
      ),
    acceptLastConfirm: async () => {
      const request = confirmRequests.at(-1)
      expect(request).toBeTruthy()
      await request?.accept()
    },
  }
}

function makeUser(overrides: Partial<AdminUserCommandTarget> = {}): AdminUserCommandTarget {
  return {
    id: 1,
    uid: 'user-1',
    username: 'user',
    role: 'user',
    status: 1,
    authUserId: 'auth-1',
    ...overrides,
  }
}
