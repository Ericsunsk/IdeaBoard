import { computed, ref, watch } from 'vue'
import type {
  ListAdminTasksParams,
  ListAdminTasksStatus as ListAdminTasksStatusValue,
} from '../../../shared/api/generated-query/model'
import { useAdminPagePagination } from '../shared/composables/useAdminPagePagination'
import { useAdminQueryPolling } from '../shared/composables/useAdminQueryPolling'
import { useAdminTaskMonitorSettingQuery } from '../system-settings/taskMonitorSettingsResource'
import { useAdminTaskListResource, type AdminTask } from './taskMonitorResource'

type TaskStatusStats = {
  running: number
  pending: number
  completed: number
  failed: number
  cancelled: number
  other: number
}

export type AdminTaskRow = AdminTask

export function useAdminTaskList() {
  const pageState = ref(1)
  const filterStatus = ref<ListAdminTasksStatusValue | null>(null)
  const accountIdFilter = ref<number | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const hasActiveTasks = ref(true)
  const { settings: taskMonitorSettings, settingsError: taskMonitorSettingsError } =
    useAdminTaskMonitorSettingQuery()
  const pageSize = computed(() => taskMonitorSettings.value.list_page_size)
  const resultPreviewEnabled = computed(() => taskMonitorSettings.value.result_preview_enabled)

  function resolveParams(): ListAdminTasksParams {
    const params: ListAdminTasksParams = {
      status: filterStatus.value || undefined,
      account_id: accountIdFilter.value || undefined,
      page: pageState.value,
      page_size: pageSize.value,
    }
    if (appliedSearchKeyword.value) {
      if (/^\d+$/.test(appliedSearchKeyword.value))
        params.user_id = parseInt(appliedSearchKeyword.value, 10)
      else params.keyword = appliedSearchKeyword.value
    }
    return params
  }

  const polling = useAdminQueryPolling({
    visibleInterval: () =>
      taskMonitorSettings.value.auto_refresh_enabled
        ? taskMonitorSettings.value.refresh_interval_seconds * 1000
        : false,
    hiddenInterval: false,
    isEnabled: () => hasActiveTasks.value,
  })

  const tasksQuery = useAdminTaskListResource(computed(resolveParams), {
    query: {
      placeholderData: (prev) => prev,
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })

  const tasks = computed(() => tasksQuery.data.value?.list || [])
  const paginationTotalRecords = computed(() => tasksQuery.data.value?.total)
  const total = computed(() => paginationTotalRecords.value ?? 0)
  const loading = computed(() => tasksQuery.isLoading.value)
  const paginationPending = computed(
    () => tasksQuery.isLoading.value || tasksQuery.isFetching.value,
  )
  const pagination = useAdminPagePagination({
    currentPage: pageState,
    totalRecords: paginationTotalRecords,
    pageSize,
    pending: paginationPending,
  })
  const isError = computed(() => tasksQuery.isError.value)
  const error = computed(() => (isError.value ? (tasksQuery.error.value ?? null) : null))
  const taskStats = computed<TaskStatusStats>(() => {
    const data = tasksQuery.data.value
    return {
      pending: data?.pending_count ?? 0,
      running: data?.running_count ?? 0,
      completed: data?.completed_count ?? 0,
      failed: data?.failed_count ?? 0,
      cancelled: data?.cancelled_count ?? 0,
      other: data?.other_count ?? 0,
    }
  })

  watch(
    () => {
      const data = tasksQuery.data.value
      if (!data) return true
      return (data.pending_count ?? 0) > 0 || (data.running_count ?? 0) > 0
    },
    (nextHasActiveTasks) => {
      hasActiveTasks.value = nextHasActiveTasks
    },
    { immediate: true },
  )

  function handleSearch() {
    pagination.reset()
    appliedSearchKeyword.value = searchKeyword.value.trim()
  }

  watch(
    [filterStatus, accountIdFilter],
    () => {
      pagination.reset()
    },
    { flush: 'sync' },
  )

  watch(
    pageSize,
    () => {
      pagination.reset()
    },
    { flush: 'sync' },
  )

  watch(polling.isPageVisible, (isVisible, wasVisible) => {
    if (isVisible && !wasVisible && hasActiveTasks.value) void tasksQuery.refetch()
  })

  function loadTasks() {
    void tasksQuery.refetch()
  }

  return {
    tasks,
    total,
    loading,
    isError,
    error,
    pagination,
    pageSize,
    resultPreviewEnabled,
    taskMonitorSettingsError,
    taskStats,
    filterStatus,
    accountIdFilter,
    searchKeyword,
    loadTasks,
    handleSearch,
  }
}
