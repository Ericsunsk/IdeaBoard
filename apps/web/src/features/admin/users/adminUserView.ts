import type { User } from '../../../shared/api/generated-query/model'
import { formatDateOnly } from '../../../shared/utils/dateTime'

export type AdminUserRole = 'admin' | 'user'
export type AdminUserStatus = 0 | 1
export type AdminUserTagSeverity = 'danger' | 'secondary' | 'success'

export type AdminUserCommandTarget = {
  id: number
  uid: string
  username: string
  authUserId: string
  role: string
  status: number
}

export type AdminUserRowView = AdminUserCommandTarget & {
  avatarUrl: string
  initial: string
  authUserIdLabel: string
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

export function getUserInitial(user: Pick<AdminUserCommandTarget, 'username' | 'uid'>) {
  return (user.username || user.uid || '?').slice(0, 1).toUpperCase()
}

export function createAdminUserRowView(user: User): AdminUserRowView {
  const authUserId = user.auth_user_id || ''
  const isAdmin = isAdminUser(user)
  const isActive = isActiveUser(user)

  return {
    id: user.id,
    uid: user.uid,
    username: user.username,
    authUserId,
    role: user.role,
    status: user.status,
    avatarUrl: user.avatar || '',
    initial: getUserInitial(user),
    authUserIdLabel: authUserId || '-',
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
  const hasAuthIdentity = Boolean(row.authUserId)
  return [
    {
      key: 'status',
      label: isActiveUser(row) ? '用户封禁' : '解除封禁',
      disabled: isAdmin,
    },
    { key: 'revoke', label: '吊销会话', disabled: !hasAuthIdentity },
    { key: 'impersonate', label: '伪装登录', disabled: !hasAuthIdentity },
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
