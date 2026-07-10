import type { MaybeRef } from 'vue'
import {
  getListAdminUsersQueryKey,
  useListAdminUsers,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import type { ListAdminUsersParams } from '../../../shared/api/generated-query/model'

export type AdminUsersRoleFilterValue = 'admin' | 'user'
export type AdminUsersStatusFilterValue = 0 | 1

export type AdminUsersQueryParams = {
  page: number
  page_size: number
  keyword?: string
  role?: AdminUsersRoleFilterValue
  status?: AdminUsersStatusFilterValue
}

export function useAdminUsersResource(params: MaybeRef<AdminUsersQueryParams>) {
  return useListAdminUsers(toGeneratedListParams(params), {
    query: {
      placeholderData: (previous) => previous,
    },
  })
}

export function getAdminUsersListQueryKey() {
  return getListAdminUsersQueryKey()
}

function toGeneratedListParams(params: MaybeRef<AdminUsersQueryParams>) {
  return params as MaybeRef<ListAdminUsersParams>
}
