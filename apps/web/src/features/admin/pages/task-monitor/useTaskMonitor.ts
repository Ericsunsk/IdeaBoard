import { computed, ref, watch } from 'vue'
import { useListAdminTasks } from '../../../../shared/api/generated-query/admin-tasks/admin-tasks'
import {
  type ListAdminTasksParams,
  type ListAdminTasksStatus as ListAdminTasksStatusValue,
} from '../../../../shared/api/generated-query/model'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'

export const TASK_MONITOR_PAGE_SIZE = 12

type TaskStatusStats = {
  running: number
  pending: number
  completed: number
  failed: number
  cancelled: number
  other: number
}

export function useTaskMonitor() {
  const page = ref(1)
  const filterStatus = ref<ListAdminTasksStatusValue | null>(null)
  const accountIdFilter = ref<number | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const hasActiveTasks = ref(true)

  function resolveParams(): ListAdminTasksParams {
    const params: ListAdminTasksParams = {
      status: filterStatus.value || undefined,
      account_id: accountIdFilter.value || undefined,
      page: page.value,
      page_size: TASK_MONITOR_PAGE_SIZE,
    }
    if (appliedSearchKeyword.value) {
      if (/^\d+$/.test(appliedSearchKeyword.value))
        params.user_id = parseInt(appliedSearchKeyword.value, 10)
      else params.keyword = appliedSearchKeyword.value
    }
    return params
  }

  const polling = useAdminQueryPolling({
    visibleInterval: 4_000,
    hiddenInterval: false,
    isEnabled: () => hasActiveTasks.value,
  })

  const tasksQuery = useListAdminTasks(computed(resolveParams), {
    query: {
      placeholderData: (prev) => prev,
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })

  const tasks = computed(() => tasksQuery.data.value?.data?.list || [])
  const total = computed(() => tasksQuery.data.value?.data?.total || 0)
  const loading = computed(() => tasksQuery.isLoading.value)
  const isError = computed(() => tasksQuery.isError.value)
  const error = computed(() => (isError.value ? (tasksQuery.error.value ?? null) : null))
  const taskStats = computed<TaskStatusStats>(() => {
    const data = tasksQuery.data.value?.data
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
      const data = tasksQuery.data.value?.data
      if (!data) return true
      return (data.pending_count ?? 0) > 0 || (data.running_count ?? 0) > 0
    },
    (nextHasActiveTasks) => {
      hasActiveTasks.value = nextHasActiveTasks
    },
    { immediate: true },
  )

  function handleSearch() {
    page.value = 1
    appliedSearchKeyword.value = searchKeyword.value.trim()
  }

  watch([filterStatus, accountIdFilter], () => {
    page.value = 1
  })

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
    page,
    taskStats,
    filterStatus,
    accountIdFilter,
    searchKeyword,
    loadTasks,
    handleSearch,
  }
}
