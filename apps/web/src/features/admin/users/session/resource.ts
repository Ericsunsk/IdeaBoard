// Users collection resource (issue 808): feature-owned boundary over the
// Control generated client for user administration reads. Route pages and
// components must consume this module, never `generated-query` directly.
//
// Transport hits /api/control/v1 (controlMutator prefix) with the contract
// envelope {data, meta}. Cursor pagination follows the converged admin
// collection pattern (useAdminCursorCollection, prev/next without totals).
// Control identifiers are usr_ public ids; the adapter maps ControlUser
// onto the legacy row shape so views keep working, defaulting fields the
// control projection does not carry.
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  getListControlUsersQueryKey,
  listControlUsers,
} from '@/api/generated-query/control/control-users/control-users'
import type { ControlUser, ListControlUsersParams } from '@/api/generated-query/control/model'
import { stripUndefined } from '@/api/http/stripUndefined'
import { normalizePageSize, PAGE_SIZES } from '@/core/config/pagination'
import { useAdminCursorCollection } from '../../shared'

type AdminUsersRoleFilterValue = 'admin' | 'user'
type AdminUsersStatusFilterValue = 0 | 1

// Feature-owned row item: legacy AdminUserListItem shape, control-sourced.
// public_id carries the usr_ public id used by every control mutation.
export type AdminUserListItem = {
  public_id: string
  email: string
  providers: string[]
  display_name: string
  avatar: string
  role: AdminUsersRoleFilterValue
  status: number
  created_at: string
  updated_at: string
  available_credits: number
  last_login_ip: string
  last_login_country: string
}

export type AdminUsersQueryFilters = {
  keyword?: string | undefined
  role?: AdminUsersRoleFilterValue | undefined
  status?: AdminUsersStatusFilterValue | undefined
  pageSize?: number | undefined
}

export type AdminUsersPage = {
  items: AdminUserListItem[]
  nextCursor: string | undefined
  hasMore: boolean
}

const ADMIN_USERS_PAGE_SIZE_DEFAULT = PAGE_SIZES.adminUsers
const ADMIN_USERS_PAGE_SIZE_MAX = PAGE_SIZES.max

function normalizeRole(value: ControlUser['role']): AdminUsersRoleFilterValue {
  return value === 'admin' ? 'admin' : 'user'
}

export function mapControlUserToListItem(user: ControlUser): AdminUserListItem {
  return {
    public_id: user.id,
    email: user.email ?? '',
    providers: user.providers ?? [],
    display_name: user.display_name ?? '',
    avatar: user.avatar ?? '',
    role: normalizeRole(user.role),
    status: typeof user.status === 'number' ? user.status : 0,
    created_at: user.created_at ?? '',
    updated_at: user.updated_at ?? '',
    available_credits: typeof user.available_credits === 'number' ? user.available_credits : 0,
    last_login_ip: user.last_login_ip ?? '',
    last_login_country: user.last_login_country ?? '',
  }
}

function normalizeAdminUsersPageSize(pageSize: number | undefined): number {
  return normalizePageSize(pageSize, ADMIN_USERS_PAGE_SIZE_DEFAULT, ADMIN_USERS_PAGE_SIZE_MAX)
}

function normalizeStatusFilter(
  status: AdminUsersStatusFilterValue | undefined,
): string | undefined {
  if (status === undefined) return undefined
  return String(status)
}

export function buildListControlUsersParams(
  filters: AdminUsersQueryFilters,
  cursor: string | undefined,
): ListControlUsersParams {
  return stripUndefined({
    ...(cursor === undefined ? {} : { cursor }),
    page_size: normalizeAdminUsersPageSize(filters.pageSize),
    keyword: filters.keyword?.trim() ? filters.keyword.trim() : undefined,
    role: filters.role,
    status: normalizeStatusFilter(filters.status),
  })
}

export function getAdminUsersListQueryKey() {
  return getListControlUsersQueryKey()
}

export function useAdminUsersResource(filters: MaybeRefOrGetter<AdminUsersQueryFilters>) {
  const normalizedFilters = computed<AdminUsersQueryFilters>(() => {
    const raw = toValue(filters)
    return {
      keyword: raw.keyword?.trim() ? raw.keyword.trim() : undefined,
      role: raw.role,
      status: raw.status,
      pageSize: normalizeAdminUsersPageSize(raw.pageSize),
    }
  })

  const collection = useAdminCursorCollection<AdminUsersPage, string>({
    buildKey: (cursor) => [
      ...getAdminUsersListQueryKey(),
      stripUndefined({
        keyword: normalizedFilters.value.keyword,
        role: normalizedFilters.value.role,
        status: normalizedFilters.value.status,
        page_size: normalizedFilters.value.pageSize,
      }),
      cursor ?? null,
    ],
    queryFn: async ({ queryKey, signal }) => {
      const cursor = queryKey.at(-1)
      const response = await listControlUsers(
        buildListControlUsersParams(
          normalizedFilters.value,
          typeof cursor === 'string' ? cursor : undefined,
        ),
        { signal },
      )
      const items = Array.isArray(response.data) ? response.data : []
      const meta = response.meta
      return {
        items: items.map(mapControlUserToListItem),
        nextCursor:
          meta?.has_more && typeof meta.next_cursor === 'string' && meta.next_cursor
            ? meta.next_cursor
            : undefined,
        hasMore: meta?.has_more === true,
      }
    },
    getNextCursor: (page) => page.nextCursor,
  })

  return collection
}
