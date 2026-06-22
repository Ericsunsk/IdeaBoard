import { computed, ref, watch } from 'vue'
import { useListAdminUsers } from '../../../../shared/api/generated-query/admin-users/admin-users'
import {
  type ListAdminUsersParams,
  type ListAdminUsersRole as ListAdminUsersRoleValue,
  type ListAdminUsersStatus as ListAdminUsersStatusValue,
} from '../../../../shared/api/generated-query/model'
import type { User } from '../../../../shared/types'
import {
  USER_ROLE_FILTER_ALL,
  USER_STATUS_FILTER_ALL,
  type UserRoleFilter,
  type UserStatusFilter,
} from './adminUserView'

export function useAdminUsersList() {
  const currentPage = ref(1)
  const pageSize = ref(10)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const userRoleFilter = ref<UserRoleFilter>(USER_ROLE_FILTER_ALL)
  const userStatusFilter = ref<UserStatusFilter>(USER_STATUS_FILTER_ALL)
  const selectedUsers = ref<User[]>([])

  const hasUserFilter = computed(
    () =>
      userRoleFilter.value !== USER_ROLE_FILTER_ALL ||
      userStatusFilter.value !== USER_STATUS_FILTER_ALL,
  )
  const hasAppliedUserQuery = computed(
    () => Boolean(appliedSearchKeyword.value) || hasUserFilter.value,
  )
  const selectedUserRole = computed<ListAdminUsersParams['role']>(() =>
    userRoleFilter.value === USER_ROLE_FILTER_ALL
      ? undefined
      : (userRoleFilter.value as ListAdminUsersRoleValue),
  )
  const selectedUserStatus = computed<ListAdminUsersParams['status']>(() => {
    if (userStatusFilter.value === USER_STATUS_FILTER_ALL) return undefined
    return userStatusFilter.value as ListAdminUsersStatusValue
  })
  const userQueryParams = computed<ListAdminUsersParams>(() => ({
    page: currentPage.value,
    page_size: pageSize.value,
    keyword: appliedSearchKeyword.value || undefined,
    role: selectedUserRole.value,
    status: selectedUserStatus.value,
  }))
  const usersQuery = useListAdminUsers(userQueryParams, {
    query: {
      placeholderData: (previous) => previous,
    },
  })
  const usersData = computed(() => usersQuery.data.value?.data)
  const users = computed<User[]>(() => usersData.value?.list ?? [])
  const loading = computed(() => usersQuery.isLoading.value || usersQuery.isFetching.value)
  const totalRecords = computed(() => usersData.value?.total ?? 0)
  const activeUserCount = computed(() => usersData.value?.active_count ?? 0)
  const bannedUserCount = computed(() => usersData.value?.banned_count ?? 0)
  const adminUserCount = computed(() => usersData.value?.admin_count ?? 0)
  const totalUsersLabel = computed(() =>
    hasAppliedUserQuery.value ? `${totalRecords.value} 匹配用户` : `${totalRecords.value} 用户`,
  )
  const totalPages = computed(() => Math.ceil(totalRecords.value / pageSize.value) || 1)
  const emptyUserMessage = computed(() =>
    hasAppliedUserQuery.value ? '暂无匹配用户' : '暂无用户',
  )
  const userPlaceholderRows = computed(() => {
    if (loading.value || users.value.length === 0) return 0
    return Math.max(0, pageSize.value - users.value.length)
  })

  watch(totalPages, (newTotal) => {
    if (currentPage.value > newTotal) {
      currentPage.value = Math.max(1, newTotal)
    }
  })

  watch([userRoleFilter, userStatusFilter], () => {
    currentPage.value = 1
  })

  watch(userQueryParams, () => {
    selectedUsers.value = []
  })

  function resetUserFilters() {
    userRoleFilter.value = USER_ROLE_FILTER_ALL
    userStatusFilter.value = USER_STATUS_FILTER_ALL
  }

  function handlePrevPage() {
    if (currentPage.value > 1) {
      currentPage.value--
    }
  }

  function handleNextPage() {
    if (currentPage.value < totalPages.value) {
      currentPage.value++
    }
  }

  function clearSearch() {
    searchKeyword.value = ''
    if (appliedSearchKeyword.value) {
      appliedSearchKeyword.value = ''
    }
    currentPage.value = 1
  }

  function loadUsers() {
    const nextKeyword = searchKeyword.value.trim()
    if (appliedSearchKeyword.value !== nextKeyword) {
      currentPage.value = 1
      appliedSearchKeyword.value = nextKeyword
      return
    }
    void usersQuery.refetch()
  }

  return {
    currentPage,
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    selectedUsers,
    hasUserFilter,
    hasAppliedUserQuery,
    userQueryParams,
    usersQuery,
    users,
    loading,
    totalRecords,
    activeUserCount,
    bannedUserCount,
    adminUserCount,
    totalUsersLabel,
    totalPages,
    emptyUserMessage,
    userPlaceholderRows,
    resetUserFilters,
    handlePrevPage,
    handleNextPage,
    clearSearch,
    loadUsers,
  }
}
