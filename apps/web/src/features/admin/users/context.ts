import { computed, ref, watch } from 'vue'
import type { AdminUsersFeedback } from './useUsersFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { useAdminUserCommands } from './commands'
import type { AdminUserSessionCommands } from './sessionCommands'
import { useAdminUsersResource, type AdminUsersQueryFilters } from './resource'
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
  const selectedUserRole = computed<AdminUsersQueryFilters['role']>(() =>
    userRoleFilter.value === USER_ROLE_FILTER_ALL
      ? undefined
      : (userRoleFilter.value as AdminUsersQueryFilters['role']),
  )
  const selectedUserStatus = computed(() => {
    if (userStatusFilter.value === USER_STATUS_FILTER_ALL) return undefined
    return userStatusFilter.value as AdminUsersQueryFilters['status']
  })
  const userQueryFilters = computed<AdminUsersQueryFilters>(() => ({
    keyword: appliedSearchKeyword.value || undefined,
    role: selectedUserRole.value,
    status: selectedUserStatus.value,
    pageSize: pageSize.value,
  }))
  return {
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    hasUserFilter,
    hasAppliedUserQuery,
    userQueryFilters,
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
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    hasUserFilter,
    hasAppliedUserQuery,
    userQueryFilters,
  } = createAdminUsersQueryState()
  const collection = useAdminUsersResource(userQueryFilters)
  const users = computed<AdminUserRowView[]>(() =>
    (collection.data.value?.items ?? []).map(createAdminUserRowView),
  )
  const loading = computed(() => collection.loading.value)
  // Cursor collections carry no total: the footer names the scope instead
  // of a count, and the pager navigates known pages only.
  const totalUsersLabel = computed(() => (hasAppliedUserQuery.value ? '匹配用户' : '全部用户'))
  const pagination = {
    currentPage: collection.pagination.currentPage,
    totalPages: collection.pagination.knownPageCount,
    canPrevious: collection.pagination.canRetreat,
    canNext: collection.pagination.canAdvance,
    previous: () => {
      collection.previousPage()
    },
    next: () => {
      collection.nextPage()
    },
    reset: () => {
      collection.pagination.reset()
    },
  }
  const emptyUserMessage = computed(() => (hasAppliedUserQuery.value ? '暂无匹配用户' : '暂无用户'))

  watch([userRoleFilter, userStatusFilter, pageSize], () => {
    pagination.reset()
  })

  watch(
    [() => collection.initialError.value, () => collection.backgroundError.value],
    ([initialError, backgroundError]) => {
      if (initialError || backgroundError) options.feedback.loadFailed(collection.query.error.value)
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
    void collection.refresh()
  }

  const list = {
    pagination,
    pageSize,
    searchKeyword,
    appliedSearchKeyword,
    userRoleFilter,
    userStatusFilter,
    hasUserFilter,
    hasAppliedUserQuery,
    users,
    loading,
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
