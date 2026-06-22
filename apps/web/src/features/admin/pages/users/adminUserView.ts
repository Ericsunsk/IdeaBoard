import {
  ListAdminUsersRole,
  ListAdminUsersStatus,
  type ListAdminUsersRole as ListAdminUsersRoleValue,
  type ListAdminUsersStatus as ListAdminUsersStatusValue,
} from '../../../../shared/api/generated-query/model'
import type { User } from '../../../../shared/types'

export type UserActionKey = 'status' | 'revoke' | 'impersonate' | 'role' | 'delete'

export type UserActionItem = {
  key: UserActionKey
  label: string
  disabled?: boolean
  divided?: boolean
}

export const USER_ROLE_FILTER_ALL = 'all'
export const USER_STATUS_FILTER_ALL = 'all'

export type UserRoleFilter = typeof USER_ROLE_FILTER_ALL | ListAdminUsersRoleValue
export type UserStatusFilter = typeof USER_STATUS_FILTER_ALL | ListAdminUsersStatusValue

export const userRoleFilterOptions: Array<{ label: string; value: UserRoleFilter }> = [
  { label: '全部角色', value: USER_ROLE_FILTER_ALL },
  { label: '管理员', value: ListAdminUsersRole.admin },
  { label: '普通用户', value: ListAdminUsersRole.user },
]

export const userStatusFilterOptions: Array<{ label: string; value: UserStatusFilter }> = [
  { label: '全部状态', value: USER_STATUS_FILTER_ALL },
  { label: '正常', value: ListAdminUsersStatus.NUMBER_1 },
  { label: '封禁', value: ListAdminUsersStatus.NUMBER_0 },
]

export function isAdminUser(user: Pick<User, 'role'>) {
  return user.role === ListAdminUsersRole.admin
}

export function isActiveUser(user: Pick<User, 'status'>) {
  return user.status === ListAdminUsersStatus.NUMBER_1
}

export function getUserInitial(user: User) {
  return (user.username || user.uid || '?').slice(0, 1).toUpperCase()
}

export function getUserActionItems(row: User): UserActionItem[] {
  const isAdmin = isAdminUser(row)
  const hasAuthIdentity = Boolean(row.auth_user_id)
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
