import type { AdminUserListItem } from '../../../shared/api/generated-query/model'
import { formatDateOnly } from '../../../shared/utils/dateTime'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'

export type AdminUserRole = 'admin' | 'user'
export type AdminUserStatus = 0 | 1
export type AdminUserTagSeverity = 'danger' | 'secondary' | 'success'

export type AdminUserCommandTarget = {
  publicId: string
  displayName: string
  role: string
  status: number
}

export type AdminUserRowView = AdminUserCommandTarget & {
  email: string
  availableCredits: number
  availableCreditsLabel: string
  avatarUrl: string
  initial: string
  createdDateLabel: string
  isAdmin: boolean
  isActive: boolean
  roleLabel: string
  roleSeverity: AdminUserTagSeverity
  statusLabel: string
  statusSeverity: AdminUserTagSeverity
}

export type UserActionKey = 'status' | 'revoke' | 'impersonate' | 'role' | 'delete'

export type UserActionItem = {
  key: UserActionKey
  label: string
  disabled?: boolean
  divided?: boolean
}

export const USER_ROLE_FILTER_ALL = 'all'
export const USER_STATUS_FILTER_ALL = 'all'

export type UserRoleFilter = typeof USER_ROLE_FILTER_ALL | AdminUserRole
export type UserStatusFilter = typeof USER_STATUS_FILTER_ALL | AdminUserStatus

export const userRoleFilterOptions: Array<{ label: string; value: UserRoleFilter }> = [
  { label: '全部角色', value: USER_ROLE_FILTER_ALL },
  { label: '管理员', value: 'admin' },
  { label: '普通用户', value: 'user' },
]

export const userStatusFilterOptions: Array<{ label: string; value: UserStatusFilter }> = [
  { label: '全部状态', value: USER_STATUS_FILTER_ALL },
  { label: '正常', value: 1 },
  { label: '封禁', value: 0 },
]

export function isAdminUser(user: Pick<AdminUserCommandTarget, 'role'>) {
  return user.role === 'admin'
}

export function isActiveUser(user: Pick<AdminUserCommandTarget, 'status'>) {
  return user.status === 1
}

export function getUserInitial(user: Pick<AdminUserCommandTarget, 'displayName' | 'publicId'>) {
  return (user.displayName || user.publicId || '?').slice(0, 1).toUpperCase()
}

export function createAdminUserRowView(user: AdminUserListItem): AdminUserRowView {
  const isAdmin = isAdminUser(user)
  const isActive = isActiveUser(user)

  return {
    publicId: user.public_id,
    email: user.email ?? '',
    displayName: user.display_name,
    role: user.role,
    status: user.status,
    availableCredits: user.available_credits,
    availableCreditsLabel: formatIdeaAmount(user.available_credits),
    avatarUrl: user.avatar || '',
    initial: getUserInitial({ displayName: user.display_name, publicId: user.public_id }),
    createdDateLabel: user.created_at ? formatDateOnly(user.created_at) : '-',
    isAdmin,
    isActive,
    roleLabel: isAdmin ? 'Admin' : 'User',
    roleSeverity: isAdmin ? 'danger' : 'secondary',
    statusLabel: isActive ? '正常' : '封禁',
    statusSeverity: isActive ? 'success' : 'danger',
  }
}

export function getUserActionItems(row: AdminUserCommandTarget): UserActionItem[] {
  const isAdmin = isAdminUser(row)
  return [
    {
      key: 'status',
      label: isActiveUser(row) ? '用户封禁' : '解除封禁',
      disabled: isAdmin,
    },
    { key: 'revoke', label: '吊销会话' },
    { key: 'impersonate', label: '伪装登录' },
    {
      key: 'role',
      label: isAdmin ? '取消管理' : '设为管理',
    },
    {
      key: 'delete',
      label: '注销用户',
      disabled: isAdmin,
      divided: true,
    },
  ]
}
