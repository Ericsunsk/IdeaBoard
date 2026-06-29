import { computed, ref, watch } from 'vue'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import { useAdminUserCommands } from './adminUserCommands'
import type { AdminUserSessionCommands } from './adminUserSessionCommands'
import { useAdminUsersResource, type AdminUsersQueryParams } from './adminUsersResource'
import {
  createAdminUserRowView,
  type AdminUserRowView,
  USER_ROLE_FILTER_ALL,
  USER_STATUS_FILTER_ALL,
  type UserRoleFilter,
  type UserStatusFilter,
  userRoleFilterOptions,
  userStatusFilterOptions,
} from './adminUserView'

type AdminUsersMessage = {
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

type AdminUsersConfirm = {
  require: (options: AdminConfirmOptions) => void
}

type AdminUsersContextDependencies = {
  sessionCommands?: AdminUserSessionCommands
}

export function useAdminUsersContext(
  options: {
    message: AdminUsersMessage
    confirm: AdminUsersConfirm
  },
  dependencies: AdminUsersContextDependencies = {},
) {
  const currentPage = ref(1)
  const pageSize = ref(10)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const userRoleFilter = ref<UserRoleFilter>(USER_ROLE_FILTER_ALL)
  const userStatusFilter = ref<UserStatusFilter>(USER_STATUS_FILTER_ALL)
  const selectedUsers = ref<AdminUserRowView[]>([])

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
    page: currentPage.value,
    page_size: pageSize.value,
    keyword: appliedSearchKeyword.value || undefined,
    role: selectedUserRole.value,
    status: selectedUserStatus.value,
  }))
  const usersQuery = useAdminUsersResource(userQueryParams)
  const usersData = computed(() => usersQuery.data.value?.data)
  const users = computed<AdminUserRowView[]>(() =>
    (usersData.value?.list ?? []).map(createAdminUserRowView),
  )
  const loading = computed(() => usersQuery.isLoading.value || usersQuery.isFetching.value)
  const totalRecords = computed(() => usersData.value?.total ?? 0)
  const activeUserCount = computed(() => usersData.value?.active_count ?? 0)
  const bannedUserCount = computed(() => usersData.value?.banned_count ?? 0)
  const adminUserCount = computed(() => usersData.value?.admin_count ?? 0)
  const totalUsersLabel = computed(() =>
    hasAppliedUserQuery.value ? `${totalRecords.value} 匹配用户` : `${totalRecords.value} 用户`,
  )
  const totalPages = computed(() => Math.ceil(totalRecords.value / pageSize.value) || 1)
  const emptyUserMessage = computed(() => (hasAppliedUserQuery.value ? '暂无匹配用户' : '暂无用户'))
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

  watch(
    () => usersQuery.error.value,
    (error) => {
      if (error) options.message.error(formatDisplayErrorMessage(error, '加载用户失败'))
    },
  )

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

  const list = {
    currentPage,
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
  const commands = useAdminUserCommands(
    {
      message: options.message,
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
