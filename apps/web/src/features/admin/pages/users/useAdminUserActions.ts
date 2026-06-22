import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import { useUserStore } from '../../../../app/stores/user'
import { getListAdminUsersQueryKey } from '../../../../shared/api/generated-query/admin-users/admin-users'
import { ListAdminUsersRole } from '../../../../shared/api/generated-query/model'
import { authClient, extractBetterAuthErrorMessage } from '../../../../shared/auth/client'
import type { User } from '../../../../shared/types'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import {
  banAdminUser,
  deleteAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
} from '../../shared/queries/adminActions'
import { getUserActionItems, isActiveUser, isAdminUser, type UserActionKey } from './adminUserView'

type AdminUserActionsMessage = {
  error: (message: string) => void
  success: (message: string) => void
  warning: (message: string) => void
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

export const banDurationOptions: Array<{ label: string; value: number }> = [
  { label: '永久封禁 (Permanent)', value: 0 },
  { label: '1 天 (24 小时警告)', value: 1 },
  { label: '7 天 (严重警告)', value: 7 },
  { label: '30 天 (短期隔离)', value: 30 },
]

export function useAdminUserActions(options: {
  message: AdminUserActionsMessage
  confirm: AdminUserActionsConfirm
}) {
  const queryClient = useQueryClient()
  const userStore = useUserStore()
  const activeActionUser = ref<User | null>(null)
  const showBanModal = ref(false)
  const banFormLoading = ref(false)
  const banUserContext = ref<User | null>(null)
  const banForm = ref({
    expiresInDays: 0,
    reason: '',
  })

  function invalidateAdminUsersQuery() {
    return queryClient.invalidateQueries({ queryKey: getListAdminUsersQueryKey() })
  }

  const banUserMutation = useMutation({
    mutationFn: banAdminUser,
    onSuccess: () => invalidateAdminUsersQuery(),
  })
  const unbanUserMutation = useMutation({
    mutationFn: unbanAdminUser,
    onSuccess: () => invalidateAdminUsersQuery(),
  })
  const revokeUserSessionsMutation = useMutation({
    mutationFn: revokeAdminUserSessions,
  })
  const updateUserRoleMutation = useMutation({
    mutationFn: updateAdminUserRole,
    onSuccess: () => invalidateAdminUsersQuery(),
  })
  const deleteUserMutation = useMutation({
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

  function openUserActionMenu(user: User) {
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

  function handleAction(key: UserActionKey, row: User) {
    switch (key) {
      case 'status':
        handleToggleStatus(row)
        break
      case 'revoke':
        handleRevokeSessions(row)
        break
      case 'impersonate':
        handleImpersonate(row)
        break
      case 'role':
        handleToggleRole(row)
        break
      case 'delete':
        handleDeleteUser(row)
        break
    }
  }

  function handleToggleStatus(row: User) {
    if (isAdminUser(row)) {
      options.message.warning('管理员账号不可封禁或解封')
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
          options.message.success('已解除封禁')
        } catch (error) {
          options.message.error(formatDisplayErrorMessage(error, '解封失败'))
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
      options.message.success(`已高级封禁用户 ${row.username}`)
      showBanModal.value = false
      banUserContext.value = null
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '封禁失败'))
    } finally {
      banFormLoading.value = false
    }
  }

  function handleRevokeSessions(row: User) {
    if (!row.auth_user_id) {
      options.message.warning('目标用户缺少 Auth 标识，无法吊销会话')
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
          options.message.success('已强制吊销所有会话')
        } catch (error) {
          options.message.error(formatDisplayErrorMessage(error, '操作失败'))
        }
      },
    })
  }

  async function handleImpersonate(row: User) {
    if (!row.auth_user_id) {
      options.message.error('目标用户缺少 Auth 标识，无法伪装登录')
      return
    }
    try {
      const { error } = await authClient.admin.impersonateUser({ userId: row.auth_user_id })
      if (error) throw error
      userStore.clearAuth()
      options.message.success(`正在以 ${row.username} 的身份登录...`)
      setTimeout(() => (window.location.href = '/'), 1000)
    } catch (error: unknown) {
      options.message.error(extractBetterAuthErrorMessage(error, '伪装失败'))
    }
  }

  function handleDeleteUser(row: User) {
    options.confirm.require({
      header: '危险操作：彻底注销',
      message: `确定要彻底删除用户 ${row.username} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
      acceptLabel: '确定删除',
      severity: 'danger',
      accept: async () => {
        try {
          await deleteUserMutation.mutateAsync(row.id)
          options.message.success('用户已受控彻底注销')
        } catch (error: unknown) {
          options.message.error(formatDisplayErrorMessage(error, '注销失败'))
        }
      },
    })
  }

  function handleToggleRole(row: User) {
    const nextRole = isAdminUser(row) ? ListAdminUsersRole.user : ListAdminUsersRole.admin
    options.confirm.require({
      header: nextRole === ListAdminUsersRole.admin ? '确认设为管理员' : '确认取消管理员',
      message:
        nextRole === ListAdminUsersRole.admin
          ? `确定要将用户 ${row.username} 设为管理员吗？该用户将获得后台访问权限。`
          : `确定要取消用户 ${row.username} 的管理员权限吗？`,
      acceptLabel: '确认',
      severity: 'warn',
      accept: async () => {
        try {
          await updateUserRoleMutation.mutateAsync({ id: row.id, role: nextRole })
          options.message.success(
            nextRole === ListAdminUsersRole.admin ? '已设为管理员' : '已取消管理员',
          )
        } catch (error: unknown) {
          options.message.error(formatDisplayErrorMessage(error, '角色更新失败'))
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
