import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import type { AdminUsersFeedback } from '../feedback/useAdminFeedback'
import {
  banAdminUser,
  deleteAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
} from './adminUserApi'
import type { AdminUserSessionCommands } from './adminUserSessionCommands'
import { createAdminUserSessionCommands } from './adminUserSessionCommands'
import { getAdminUsersListQueryKey } from './adminUsersResource'
import {
  getUserActionItems,
  isActiveUser,
  isAdminUser,
  type AdminUserCommandTarget,
  type UserActionKey,
} from './adminUserView'

type AsyncMutation<TInput> = {
  mutateAsync: (input: TInput) => Promise<void>
}

type AdminConfirmOptions = {
  header: string
  message: string
  acceptLabel: string
  severity: 'warn' | 'danger'
  accept: () => void | Promise<void>
}

type AdminUserActionsConfirm = {
  require: (options: AdminConfirmOptions) => void
}

type AdminUserCommandDependencies = {
  mutations?: Partial<{
    banUser: AsyncMutation<BanAdminUserInput>
    unbanUser: AsyncMutation<number>
    revokeSessions: AsyncMutation<number>
    updateRole: AsyncMutation<UpdateAdminUserRoleInput>
    deleteUser: AsyncMutation<number>
  }>
  sessionCommands?: AdminUserSessionCommands
}

type AdminUserMutations = Required<NonNullable<AdminUserCommandDependencies['mutations']>>

type BanAdminUserInput = {
  id: number
  reason?: string
  expires_in_days?: number
}

type UpdateAdminUserRoleInput = {
  id: number
  role: 'admin' | 'user'
}

export const banDurationOptions: Array<{ label: string; value: number }> = [
  { label: '永久封禁 (Permanent)', value: 0 },
  { label: '1 天 (24 小时警告)', value: 1 },
  { label: '7 天 (严重警告)', value: 7 },
  { label: '30 天 (短期隔离)', value: 30 },
]

export function useAdminUserCommands(
  options: {
    feedback: AdminUsersFeedback
    confirm: AdminUserActionsConfirm
  },
  dependencies: AdminUserCommandDependencies = {},
) {
  const sessionCommands = dependencies.sessionCommands ?? createAdminUserSessionCommands()
  const activeActionUser = ref<AdminUserCommandTarget | null>(null)
  const showBanModal = ref(false)
  const banFormLoading = ref(false)
  const banUserContext = ref<AdminUserCommandTarget | null>(null)
  const banForm = ref({
    expiresInDays: 0,
    reason: '',
  })

  const injectedMutations = resolveInjectedMutations(dependencies.mutations)
  const queryClient = injectedMutations ? null : useQueryClient()

  function invalidateAdminUsersQuery() {
    return (
      queryClient?.invalidateQueries({ queryKey: getAdminUsersListQueryKey() }) ??
      Promise.resolve()
    )
  }

  const banUserMutation =
    injectedMutations?.banUser ??
    useMutation({
      mutationFn: banAdminUser,
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const unbanUserMutation =
    injectedMutations?.unbanUser ??
    useMutation({
      mutationFn: unbanAdminUser,
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const revokeUserSessionsMutation =
    injectedMutations?.revokeSessions ??
    useMutation({
      mutationFn: revokeAdminUserSessions,
    })
  const updateUserRoleMutation =
    injectedMutations?.updateRole ??
    useMutation({
      mutationFn: updateAdminUserRole,
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const deleteUserMutation =
    injectedMutations?.deleteUser ??
    useMutation({
      mutationFn: deleteAdminUser,
      onSuccess: () => invalidateAdminUsersQuery(),
    })

  const userActionItems = computed(() => {
    const user = activeActionUser.value
    if (!user) return []
    return getUserActionItems(user)
  })
  const banModalDescription = computed(() =>
    banUserContext.value
      ? `您即将对用户 ${banUserContext.value.username} 实施强制访问限制。`
      : '请选择用户后再配置访问限制。',
  )

  function openUserActionMenu(user: AdminUserCommandTarget) {
    activeActionUser.value = user
  }

  function clearActiveActionUser() {
    activeActionUser.value = null
  }

  function closeBanModal() {
    if (banFormLoading.value) return
    showBanModal.value = false
    banUserContext.value = null
  }

  function handleBanDialogVisibleUpdate(visible: boolean) {
    if (!visible) closeBanModal()
  }

  function handleActionSelect(key: UserActionKey) {
    const row = activeActionUser.value
    if (!row) return
    activeActionUser.value = null
    handleAction(key, row)
  }

  function handleAction(key: UserActionKey, row: AdminUserCommandTarget) {
    switch (key) {
      case 'status':
        handleToggleStatus(row)
        break
      case 'revoke':
        handleRevokeSessions(row)
        break
      case 'impersonate':
        void handleImpersonate(row)
        break
      case 'role':
        handleToggleRole(row)
        break
      case 'delete':
        handleDeleteUser(row)
        break
    }
  }

  function handleToggleStatus(row: AdminUserCommandTarget) {
    if (isAdminUser(row)) {
      options.feedback.adminStatusChangeForbidden()
      return
    }

    if (isActiveUser(row)) {
      banUserContext.value = row
      banForm.value = { expiresInDays: 0, reason: '' }
      showBanModal.value = true
      return
    }

    options.confirm.require({
      header: '确认解除封禁',
      message: `确定要恢复用户 ${row.username} 的访问权限吗？`,
      acceptLabel: '确认解封',
      severity: 'warn',
      accept: async () => {
        try {
          await unbanUserMutation.mutateAsync(row.id)
          options.feedback.unbanSucceeded()
        } catch (error) {
          options.feedback.unbanFailed(error)
        }
      },
    })
  }

  const confirmBanUser = async () => {
    if (!banUserContext.value) return
    const row = banUserContext.value
    banFormLoading.value = true
    try {
      await banUserMutation.mutateAsync({
        id: row.id,
        reason: banForm.value.reason || undefined,
        expires_in_days: banForm.value.expiresInDays || undefined,
      })
      options.feedback.banSucceeded(row.username)
      showBanModal.value = false
      banUserContext.value = null
    } catch (error) {
      options.feedback.banFailed(error)
    } finally {
      banFormLoading.value = false
    }
  }

  function handleRevokeSessions(row: AdminUserCommandTarget) {
    if (!row.authUserId) {
      options.feedback.missingAuthUserForSessionRevoke()
      return
    }
    options.confirm.require({
      header: '确认踢下线',
      message: `将强制吊销账号 ${row.username} 的所有活跃会话，是否继续？`,
      acceptLabel: '确认',
      severity: 'warn',
      accept: async () => {
        try {
          await revokeUserSessionsMutation.mutateAsync(row.id)
          options.feedback.revokeSessionsSucceeded()
        } catch (error) {
          options.feedback.revokeSessionsFailed(error)
        }
      },
    })
  }

  async function handleImpersonate(row: AdminUserCommandTarget) {
    if (!row.authUserId) {
      options.feedback.missingAuthUserForImpersonation()
      return
    }
    try {
      await sessionCommands.startImpersonation(row.authUserId)
      options.feedback.impersonationStarted(row.username)
    } catch (error: unknown) {
      options.feedback.impersonationFailed(error)
    }
  }

  function handleDeleteUser(row: AdminUserCommandTarget) {
    options.confirm.require({
      header: '危险操作：彻底注销',
      message: `确定要彻底删除用户 ${row.username} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
      acceptLabel: '确定删除',
      severity: 'danger',
      accept: async () => {
        try {
          await deleteUserMutation.mutateAsync(row.id)
          options.feedback.userDeleted()
        } catch (error: unknown) {
          options.feedback.userDeleteFailed(error)
        }
      },
    })
  }

  function handleToggleRole(row: AdminUserCommandTarget) {
    const nextRole = isAdminUser(row) ? 'user' : 'admin'
    options.confirm.require({
      header: nextRole === 'admin' ? '确认设为管理员' : '确认取消管理员',
      message:
        nextRole === 'admin'
          ? `确定要将用户 ${row.username} 设为管理员吗？该用户将获得后台访问权限。`
          : `确定要取消用户 ${row.username} 的管理员权限吗？`,
      acceptLabel: '确认',
      severity: 'warn',
      accept: async () => {
        try {
          await updateUserRoleMutation.mutateAsync({ id: row.id, role: nextRole })
          options.feedback.roleUpdated(nextRole)
        } catch (error: unknown) {
          options.feedback.roleUpdateFailed(error)
        }
      },
    })
  }

  return {
    activeActionUser,
    userActionItems,
    showBanModal,
    banFormLoading,
    banUserContext,
    banForm,
    banModalDescription,
    openUserActionMenu,
    clearActiveActionUser,
    closeBanModal,
    handleBanDialogVisibleUpdate,
    handleActionSelect,
    confirmBanUser,
  }
}

function resolveInjectedMutations(
  mutations: AdminUserCommandDependencies['mutations'],
): AdminUserMutations | null {
  if (
    mutations?.banUser &&
    mutations.unbanUser &&
    mutations.revokeSessions &&
    mutations.updateRole &&
    mutations.deleteUser
  ) {
    return mutations as AdminUserMutations
  }
  return null
}
