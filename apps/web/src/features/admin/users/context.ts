import { computed, ref, watch } from 'vue'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { usePagePagination } from '../shared/composables/usePagePagination'
import { useAdminUserCommands } from './commands'
import type { AdminUserSessionCommands } from './sessionCommands'
import { useAdminUsersResource, type AdminUsersQueryParams } from './resource'
import {
  createAdminUserRowView,
  type AdminUserRowView,
  USER_ROLE_FILTER_ALL,
  USER_STATUS_FILTER_ALL,
  type UserRoleFilter,
  type UserStatusFilter,
  userRoleFilterOptions,
  userStatusFilterOptions,
} from './view'

type AdminUsersContextDependencies = {
  sessionCommands?: AdminUserSessionCommands | undefined
}

function createAdminUsersQueryState() {
  const pageState = ref(1)
  const pageSize = ref(10)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const userRoleFilter = ref<UserRoleFilter>(USER_ROLE_FILTER_ALL)
  const userStatusFilter = ref<UserStatusFilter>(USER_STATUS_FILTER_ALL)

  const hasUserFilter = computed(
    () =>
      userRoleFilter.value !== USER_ROLE_FILTER_ALL ||
      userStatusFilter.value !== USER_STATUS_FILTER_ALL,
  )
  const hasAppliedUserQuery = computed(
    () => Boolean(appliedSearchKeyword.value) || hasUserFilter.value,
  )
  const selectedUserRole = computed<AdminUsersQueryParams['role']>(() =>
    userRoleFilter.value === USER_ROLE_FILTER_ALL
      ? undefined
      : (userRoleFilter.value as AdminUsersQueryParams['role']),
  )
  const selectedUserStatus = computed<AdminUsersQueryParams['status']>(() => {
    if (userStatusFilter.value === USER_STATUS_FILTER_ALL) return undefined
    return userStatusFilter.value as AdminUsersQueryParams['status']
  })
  const userQueryParams = computed<AdminUsersQueryParams>(() => ({
    page: pageState.value,
    page_size: pageSize.value,
    keyword: appliedSearchKeyword.value || undefined,
    role: selectedUserRole.value,
    status: selectedUserStatus.value,
  }))
  return {
    pageState,
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    hasUserFilter,
    hasAppliedUserQuery,
    userQueryParams,
  }
}

export function useAdminUsersContext(
  options: {
    feedback: AdminUsersFeedback
    confirm: AdminConfirmPort
  },
  dependencies: AdminUsersContextDependencies = {},
) {
  const {
    pageState,
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    hasUserFilter,
    hasAppliedUserQuery,
    userQueryParams,
  } = createAdminUsersQueryState()
  const selectedUsers = ref<AdminUserRowView[]>([])
  const usersQuery = useAdminUsersResource(userQueryParams)
  const usersData = computed(() => usersQuery.data.value)
  const users = computed<AdminUserRowView[]>(() =>
    (usersData.value?.list ?? []).map(createAdminUserRowView),
  )
  const loading = computed(() => usersQuery.isLoading.value || usersQuery.isFetching.value)
  const paginationTotalRecords = computed(() => usersData.value?.total)
  const totalRecordCount = computed(() => paginationTotalRecords.value ?? 0)
  const activeUserCount = computed(() => usersData.value?.active_count ?? 0)
  const bannedUserCount = computed(() => usersData.value?.banned_count ?? 0)
  const adminUserCount = computed(() => usersData.value?.admin_count ?? 0)
  const totalUsersLabel = computed(() =>
    hasAppliedUserQuery.value
      ? `${totalRecordCount.value} 匹配用户`
      : `${totalRecordCount.value} 用户`,
  )
  const pagination = usePagePagination({
    currentPage: pageState,
    totalRecords: paginationTotalRecords,
    pageSize,
    pending: loading,
  })
  const emptyUserMessage = computed(() => (hasAppliedUserQuery.value ? '暂无匹配用户' : '暂无用户'))

  watch(
    [userRoleFilter, userStatusFilter],
    () => {
      pagination.reset()
    },
    { flush: 'sync' },
  )

  watch(userQueryParams, () => {
    selectedUsers.value = []
  })

  watch(
    () => usersQuery.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )

  function resetUserFilters() {
    userRoleFilter.value = USER_ROLE_FILTER_ALL
    userStatusFilter.value = USER_STATUS_FILTER_ALL
  }

  function clearSearch() {
    searchKeyword.value = ''
    if (appliedSearchKeyword.value) {
      appliedSearchKeyword.value = ''
    }
    pagination.reset()
  }

  function loadUsers() {
    const nextKeyword = searchKeyword.value.trim()
    if (appliedSearchKeyword.value !== nextKeyword) {
      pagination.reset()
      appliedSearchKeyword.value = nextKeyword
      return
    }
    void usersQuery.refetch()
  }

  const list = {
    pagination,
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    selectedUsers,
    hasUserFilter,
    hasAppliedUserQuery,
    users,
    loading,
    activeUserCount,
    bannedUserCount,
    adminUserCount,
    totalUsersLabel,
    emptyUserMessage,
    resetUserFilters,
    clearSearch,
    loadUsers,
  }
  const commands = useAdminUserCommands(
    {
      feedback: options.feedback,
      confirm: options.confirm,
    },
    {
      sessionCommands: dependencies.sessionCommands,
    },
  )
  const view = {
    userRoleFilterOptions,
    userStatusFilterOptions,
  }

  return {
    list,
    commands,
    view,
  }
}
