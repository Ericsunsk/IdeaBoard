import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminUserCommands } from './commands'
import type { AdminUserCommandTarget } from './view'

type ConfirmRequest = {
  onConfirm: () => void | Promise<void>
}

const userQueryClientMock = vi.hoisted(() => ({
  invalidateQueries: vi.fn().mockResolvedValue(undefined),
}))

const userApiMocks = vi.hoisted(() => ({
  banAdminUser: vi.fn().mockResolvedValue(undefined),
  deleteAdminUser: vi.fn().mockResolvedValue(undefined),
  revokeAdminUserSessions: vi.fn().mockResolvedValue(undefined),
  unbanAdminUser: vi.fn().mockResolvedValue(undefined),
  updateAdminUserRole: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQueryClient: () => userQueryClientMock,
  useMutation: (options: {
    mutationFn: (input: unknown) => Promise<unknown>
    onSuccess?: (data: unknown, input: unknown) => Promise<unknown> | unknown
  }) => ({
    mutateAsync: async (input: unknown) => {
      const data = await options.mutationFn(input)
      await options.onSuccess?.(data, input)
      return data
    },
  }),
}))

vi.mock('./api', () => ({
  banAdminUser: userApiMocks.banAdminUser,
  deleteAdminUser: userApiMocks.deleteAdminUser,
  revokeAdminUserSessions: userApiMocks.revokeAdminUserSessions,
  unbanAdminUser: userApiMocks.unbanAdminUser,
  updateAdminUserRole: userApiMocks.updateAdminUserRole,
}))

vi.mock('./resource', () => ({
  getAdminUsersListQueryKey: () => ['admin', 'users'],
}))

vi.mock('./sessionCommands', () => ({
  createAdminUserSessionCommands: () => ({
    startImpersonation: vi.fn().mockResolvedValue(undefined),
  }),
}))

describe('admin user command query invalidation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    userQueryClientMock.invalidateQueries.mockResolvedValue(undefined)
    userApiMocks.banAdminUser.mockResolvedValue(undefined)
    userApiMocks.deleteAdminUser.mockResolvedValue(undefined)
    userApiMocks.revokeAdminUserSessions.mockResolvedValue(undefined)
    userApiMocks.unbanAdminUser.mockResolvedValue(undefined)
    userApiMocks.updateAdminUserRole.mockResolvedValue(undefined)
  })

  it('invalidates the users list query after ban, unban, role, and delete mutations succeed', async () => {
    const harness = createHarness()
    const commands = harness.commands

    commands.openUserActionMenu(makeUser({ publicId: 'user-1', status: 1, displayName: 'alice' }))
    commands.handleActionSelect('status')
    await commands.confirmBanUser()

    commands.openUserActionMenu(makeUser({ publicId: 'user-2', status: 0, displayName: 'bob' }))
    commands.handleActionSelect('status')
    await harness.acceptLastConfirm()

    commands.openUserActionMenu(makeUser({ publicId: 'user-3', role: 'user', displayName: 'cara' }))
    commands.handleActionSelect('role')
    await harness.acceptLastConfirm()

    commands.openUserActionMenu(makeUser({ publicId: 'user-4', role: 'user', displayName: 'dana' }))
    commands.handleActionSelect('delete')
    await harness.acceptLastConfirm()

    expect(userQueryClientMock.invalidateQueries).toHaveBeenCalledTimes(4)
    expect(userQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(1, {
      queryKey: ['admin', 'users'],
    })
    expect(userQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(2, {
      queryKey: ['admin', 'users'],
    })
    expect(userQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(3, {
      queryKey: ['admin', 'users'],
    })
    expect(userQueryClientMock.invalidateQueries).toHaveBeenNthCalledWith(4, {
      queryKey: ['admin', 'users'],
    })
  })
})

function createHarness() {
  const confirmRequests: ConfirmRequest[] = []
  const confirm = {
    request: vi.fn((request: ConfirmRequest) => confirmRequests.push(request)),
  }
  const feedback = createAdminUsersFeedback()
  const commands = useAdminUserCommands(
    { feedback, confirm },
    {
      sessionCommands: {
        startImpersonation: vi.fn().mockResolvedValue(undefined),
      },
    },
  )

  return {
    commands,
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
