import { ref, type ComputedRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiResponseUserListData } from '../../../shared/api/generated-query/model'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { useAdminUsersContext } from './context'
import type { AdminUsersQueryParams } from './resource'

const mockState = vi.hoisted(() => ({
  params: undefined as ComputedRef<AdminUsersQueryParams> | undefined,
  query: undefined as unknown,
}))

vi.mock('./resource', () => ({
  useAdminUsersResource: vi.fn((params: ComputedRef<AdminUsersQueryParams>) => {
    mockState.params = params
    return mockState.query
  }),
}))

vi.mock('./commands', () => ({
  useAdminUserCommands: vi.fn(() => ({})),
}))

beforeEach(() => {
  mockState.params = undefined
  mockState.query = {
    data: ref(makeUserList(25)),
    error: ref(null),
    isLoading: ref(false),
    isFetching: ref(false),
    refetch: vi.fn(),
  }
})

describe('admin users pagination contract', () => {
  it('uses the shared page boundary for navigation and pending requests', () => {
    const context = createContext()
    const pagination = context.list.pagination

    expect(pagination.totalPages.value).toBe(3)
    expect(mockState.params?.value.page).toBe(1)
    expect(pagination.next()).toBe(true)
    expect(mockState.params?.value.page).toBe(2)

    const query = mockState.query as { isFetching: ReturnType<typeof ref<boolean>> }
    query.isFetching.value = true

    expect(pagination.canPrevious.value).toBe(false)
    expect(pagination.canNext.value).toBe(false)
    expect(pagination.previous()).toBe(false)
    expect(pagination.next()).toBe(false)
    expect(mockState.params?.value.page).toBe(2)
  })

  it('clamps a stale last page and resets page state before the query changes', () => {
    const context = createContext()
    const pagination = context.list.pagination
    const query = mockState.query as { data: ReturnType<typeof ref<ApiResponseUserListData>> }

    pagination.next()
    pagination.next()
    expect(pagination.currentPage.value).toBe(3)

    query.data.value = makeUserList(11)
    expect(pagination.currentPage.value).toBe(2)
    expect(mockState.params?.value.page).toBe(2)

    context.list.userRoleFilter.value = 'admin'
    expect(pagination.currentPage.value).toBe(1)

    pagination.next()
    context.list.searchKeyword.value = '  alice  '
    context.list.loadUsers()
    expect(pagination.currentPage.value).toBe(1)
    expect(mockState.params?.value.keyword).toBe('alice')
  })

  it('does not reset the page when the current request fails without replacement data', () => {
    const context = createContext()
    const pagination = context.list.pagination
    const query = mockState.query as {
      data: ReturnType<typeof ref<ApiResponseUserListData | undefined>>
      error: ReturnType<typeof ref<Error | null>>
    }

    pagination.next()
    expect(pagination.currentPage.value).toBe(2)

    query.data.value = undefined
    query.error.value = new Error('加载失败')

    expect(pagination.currentPage.value).toBe(2)
    expect(mockState.params?.value.page).toBe(2)
    expect(pagination.totalPages.value).toBe(3)
  })
})

function createContext() {
  return useAdminUsersContext({
    feedback: { loadFailed: vi.fn() } as unknown as AdminUsersFeedback,
    confirm: { confirm: vi.fn().mockResolvedValue(true) } as AdminConfirmPort,
  })
}

function makeUserList(total: number): ApiResponseUserListData {
  return {
    total,
    active_count: total,
    banned_count: 0,
    admin_count: 0,
    list: [],
  }
}
