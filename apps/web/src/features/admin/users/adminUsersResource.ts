import type { MaybeRef } from 'vue'
import {
  getListAdminUsersQueryKey,
  useListAdminUsers,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import type {
  ApiResponseUserListData,
  ListAdminUsersParams,
  User,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../shared/api/apiResponse'

export type AdminUserListItem = User
export type AdminUsersRoleFilterValue = 'admin' | 'user'
export type AdminUsersStatusFilterValue = 0 | 1

const selectAdminUsers = createApiDataSelector<ApiResponseUserListData>('加载用户列表失败')

export type AdminUsersQueryParams = {
  page: number
  page_size: number
  keyword?: string
  role?: AdminUsersRoleFilterValue
  status?: AdminUsersStatusFilterValue
}

export function useAdminUsersResource(params: MaybeRef<AdminUsersQueryParams>) {
  return useListAdminUsers<ApiResponseUserListData>(toGeneratedListParams(params), {
    query: {
      placeholderData: (previous) => previous,
      select: selectAdminUsers,
    },
  })
}

export function getAdminUsersListQueryKey() {
  return getListAdminUsersQueryKey()
}

function toGeneratedListParams(params: MaybeRef<AdminUsersQueryParams>) {
  return params as MaybeRef<ListAdminUsersParams>
}
