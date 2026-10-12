import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../../shared'
import { requireDestructiveReason } from '../../shared'
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
} from '../core/view'

type AsyncMutation<TInput> = {
  mutateAsync: (input: TInput) => Promise<void>
}

// Every destructive control mutation requires an operator reason (issue
// issue 808: control_reason_required). Reasons are collected from the ban form
// or the confirm dialog (required reason field) and forwarded both as the
// body reason and the X-Operator-Reason header by api.ts.
type OperatorReasonInput = {
  publicId: string
  operatorReason: string
}

type AdminUserCommandDependencies = {
  mutations?: Partial<{
    banUser: AsyncMutation<BanAdminUserInput>
    unbanUser: AsyncMutation<OperatorReasonInput>
    revokeSessions: AsyncMutation<OperatorReasonInput>
    updateRole: AsyncMutation<UpdateAdminUserRoleInput>
    deleteUser: AsyncMutation<OperatorReasonInput>
  }>
  sessionCommands?: AdminUserSessionCommands | undefined
}

type AdminUserMutations = Required<NonNullable<AdminUserCommandDependencies['mutations']>>

type BanAdminUserInput = {
  publicId: string
  reason?: string | undefined
  expires_in_days?: number | undefined
  operatorReason: string
}

type UpdateAdminUserRoleInput = {
  publicId: string
  role: 'admin' | 'user'
  operatorReason: string
}

export const banDurationOptions: Array<{ label: string; value: number }> = [
  { label: '永久封禁 (Permanent)', value: 0 },
  { label: '1 天 (24 小时警告)', value: 1 },
  { label: '7 天 (严重警告)', value: 7 },
  { label: '30 天 (短期隔离)', value: 30 },
]

const OPERATOR_REASON_SPEC = {
  isRequired: true,
  label: '操作原因（内部审计记录）',
  placeholder: '请填写操作原因，例如：用户申诉工单 1234…',
} as const

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
      mutationFn: (input: BanAdminUserInput) => banAdminUser(input),
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const unbanUserMutation =
    injectedMutations?.unbanUser ??
    useMutation({
      mutationFn: (input: OperatorReasonInput) => unbanAdminUser(input.publicId, input),
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const revokeUserSessionsMutation =
    injectedMutations?.revokeSessions ??
    useMutation({
      mutationFn: (input: OperatorReasonInput) => revokeAdminUserSessions(input.publicId, input),
    })
  const updateUserRoleMutation =
    injectedMutations?.updateRole ??
    useMutation({
      mutationFn: (input: UpdateAdminUserRoleInput) => updateAdminUserRole(input),
      onSuccess: () => invalidateAdminUsersQuery(),
    })
  const deleteUserMutation =
    injectedMutations?.deleteUser ??
    useMutation({
      mutationFn: (input: OperatorReasonInput) => deleteAdminUser(input.publicId, input),
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

  // confirmWithReason runs the confirm dialog with a required operator
  // reason and returns the trimmed reason, or null when cancelled. An empty
  // reason toasts and aborts instead of vanishing silently.
  async function confirmWithReason(request: {
    title: string
    message: string
    confirmLabel: string
    intent: 'warn' | 'danger'
  }): Promise<string | null> {
    const confirmed = await options.confirm.confirm({
      ...request,
      reason: { ...OPERATOR_REASON_SPEC },
    })
    if (!confirmed) return null
    return requireDestructiveReason(options.confirm)
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

    const operatorReason = await confirmWithReason({
      title: '确认解除封禁',
      message: `确定要恢复用户 ${row.displayName} 的访问权限吗？`,
      confirmLabel: '确认解封',
      intent: 'warn',
    })
    if (!operatorReason) return
    try {
      await unbanUserMutation.mutateAsync({ publicId: row.publicId, operatorReason })
      options.feedback.unbanSucceeded()
    } catch (error) {
      options.feedback.unbanFailed(error)
    }
  }

  const confirmBanUser = async () => {
    if (!banUserContext.value) return
    const row = banUserContext.value
    const operatorReason = banForm.value.reason.trim()
    if (!operatorReason) {
      options.feedback.banFailed(new Error('需要填写封禁原因'))
      return
    }
    banFormLoading.value = true
    try {
      await banUserMutation.mutateAsync({
        publicId: row.publicId,
        reason: operatorReason,
        expires_in_days: banForm.value.expiresInDays || undefined,
        operatorReason,
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
    const operatorReason = await confirmWithReason({
      title: '确认踢下线',
      message: `将强制吊销账号 ${row.displayName} 的所有活跃会话，是否继续？`,
      confirmLabel: '确认',
      intent: 'warn',
    })
    if (!operatorReason) return
    try {
      await revokeUserSessionsMutation.mutateAsync({ publicId: row.publicId, operatorReason })
      options.feedback.revokeSessionsSucceeded()
    } catch (error) {
      options.feedback.revokeSessionsFailed(error)
    }
  }

  async function handleImpersonate(row: AdminUserCommandTarget) {
    const operatorReason = await confirmWithReason({
      title: '确认伪装登录',
      message: `您将以用户 ${row.displayName} 的身份登录，仅用于问题排查，操作将被审计记录。是否继续？`,
      confirmLabel: '确认伪装',
      intent: 'danger',
    })
    if (!operatorReason) return
    try {
      await sessionCommands.startImpersonation(row.publicId, { operatorReason })
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

    const operatorReason = await confirmWithReason({
      title: '危险操作：彻底注销',
      message: `确定要彻底删除用户 ${row.displayName} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
      confirmLabel: '确定删除',
      intent: 'danger',
    })
    if (!operatorReason) return
    try {
      await deleteUserMutation.mutateAsync({ publicId: row.publicId, operatorReason })
      options.feedback.userDeleted()
    } catch (error: unknown) {
      options.feedback.userDeleteFailed(error)
    }
  }

  async function handleToggleRole(row: AdminUserCommandTarget) {
    const nextRole = isAdminUser(row) ? 'user' : 'admin'
    const operatorReason = await confirmWithReason({
      title: nextRole === 'admin' ? '确认设为管理员' : '确认取消管理员',
      message:
        nextRole === 'admin'
          ? `确定要将用户 ${row.displayName} 设为管理员吗？该用户将获得后台访问权限。`
          : `确定要取消用户 ${row.displayName} 的管理员权限吗？`,
      confirmLabel: '确认',
      intent: 'warn',
    })
    if (!operatorReason) return
    try {
      await updateUserRoleMutation.mutateAsync({
        publicId: row.publicId,
        role: nextRole,
        operatorReason,
      })
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
