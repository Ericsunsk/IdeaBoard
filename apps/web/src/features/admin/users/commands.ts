import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import {
  banAdminUser,
  deleteAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
} from './api'
import type { AdminUserSessionCommands } from './sessionCommands'
import { createAdminUserSessionCommands } from './sessionCommands'
import { getAdminUsersListQueryKey } from './resource'
import {
  getUserActionItems,
  isActiveUser,
  isAdminUser,
  type AdminUserCommandTarget,
  type UserActionKey,
} from './view'

type AsyncMutation<TInput> = {
  mutateAsync: (input: TInput) => Promise<void>
}

type AdminUserCommandDependencies = {
  mutations?: Partial<{
    banUser: AsyncMutation<BanAdminUserInput>
    unbanUser: AsyncMutation<string>
    revokeSessions: AsyncMutation<string>
    updateRole: AsyncMutation<UpdateAdminUserRoleInput>
    deleteUser: AsyncMutation<string>
  }>
  sessionCommands?: AdminUserSessionCommands | undefined
}

type AdminUserMutations = Required<NonNullable<AdminUserCommandDependencies['mutations']>>

type BanAdminUserInput = {
  publicId: string
  reason?: string | undefined
  expires_in_days?: number | undefined
}

type UpdateAdminUserRoleInput = {
  publicId: string
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
    confirm: AdminConfirmPort
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
      queryClient?.invalidateQueries({ queryKey: getAdminUsersListQueryKey() }) ?? Promise.resolve()
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
      ? `您即将对用户 ${banUserContext.value.displayName} 实施强制访问限制。`
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
    void handleAction(key, row)
  }

  async function handleAction(key: UserActionKey, row: AdminUserCommandTarget) {
    switch (key) {
      case 'status':
        await handleToggleStatus(row)
        break
      case 'revoke':
        await handleRevokeSessions(row)
        break
      case 'impersonate':
        await handleImpersonate(row)
        break
      case 'role':
        await handleToggleRole(row)
        break
      case 'delete':
        await handleDeleteUser(row)
        break
    }
  }

  async function handleToggleStatus(row: AdminUserCommandTarget) {
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

    const confirmed = await options.confirm.confirm({
      title: '确认解除封禁',
      message: `确定要恢复用户 ${row.displayName} 的访问权限吗？`,
      confirmLabel: '确认解封',
      intent: 'warn',
    })
    if (!confirmed) return
    try {
      await unbanUserMutation.mutateAsync(row.publicId)
      options.feedback.unbanSucceeded()
    } catch (error) {
      options.feedback.unbanFailed(error)
    }
  }

  const confirmBanUser = async () => {
    if (!banUserContext.value) return
    const row = banUserContext.value
    banFormLoading.value = true
    try {
      await banUserMutation.mutateAsync({
        publicId: row.publicId,
        reason: banForm.value.reason || undefined,
        expires_in_days: banForm.value.expiresInDays || undefined,
      })
      options.feedback.banSucceeded(row.displayName)
      showBanModal.value = false
      banUserContext.value = null
    } catch (error) {
      options.feedback.banFailed(error)
    } finally {
      banFormLoading.value = false
    }
  }

  async function handleRevokeSessions(row: AdminUserCommandTarget) {
    const confirmed = await options.confirm.confirm({
      title: '确认踢下线',
      message: `将强制吊销账号 ${row.displayName} 的所有活跃会话，是否继续？`,
      confirmLabel: '确认',
      intent: 'warn',
    })
    if (!confirmed) return
    try {
      await revokeUserSessionsMutation.mutateAsync(row.publicId)
      options.feedback.revokeSessionsSucceeded()
    } catch (error) {
      options.feedback.revokeSessionsFailed(error)
    }
  }

  async function handleImpersonate(row: AdminUserCommandTarget) {
    try {
      await sessionCommands.startImpersonation(row.publicId)
      options.feedback.impersonationStarted(row.displayName)
    } catch (error: unknown) {
      options.feedback.impersonationFailed(error)
    }
  }

  async function handleDeleteUser(row: AdminUserCommandTarget) {
    if (isAdminUser(row)) {
      options.feedback.adminStatusChangeForbidden()
      return
    }

    const confirmed = await options.confirm.confirm({
      title: '危险操作：彻底注销',
      message: `确定要彻底删除用户 ${row.displayName} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
      confirmLabel: '确定删除',
      intent: 'danger',
    })
    if (!confirmed) return
    try {
      await deleteUserMutation.mutateAsync(row.publicId)
      options.feedback.userDeleted()
    } catch (error: unknown) {
      options.feedback.userDeleteFailed(error)
    }
  }

  async function handleToggleRole(row: AdminUserCommandTarget) {
    const nextRole = isAdminUser(row) ? 'user' : 'admin'
    const confirmed = await options.confirm.confirm({
      title: nextRole === 'admin' ? '确认设为管理员' : '确认取消管理员',
      message:
        nextRole === 'admin'
          ? `确定要将用户 ${row.displayName} 设为管理员吗？该用户将获得后台访问权限。`
          : `确定要取消用户 ${row.displayName} 的管理员权限吗？`,
      confirmLabel: '确认',
      intent: 'warn',
    })
    if (!confirmed) return
    try {
      await updateUserRoleMutation.mutateAsync({ publicId: row.publicId, role: nextRole })
      options.feedback.roleUpdated(nextRole)
    } catch (error: unknown) {
      options.feedback.roleUpdateFailed(error)
    }
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
