import { computed, ref, watch } from 'vue'
import type { Task } from '../../../../shared/api/task'
import { useListAdminTasks } from '../../../../shared/api/generated-query/admin-tasks/admin-tasks'
import {
  ListAdminTasksStatus,
  type ListAdminTasksParams,
  type ListAdminTasksStatus as ListAdminTasksStatusValue,
} from '../../../../shared/api/generated-query/model'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'
import { createMockAdminTasks, useAdminMockMode } from '../../shared/lib/adminMockData'

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
  const isMockMode = useAdminMockMode()
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
      enabled: computed(() => !isMockMode.value),
      placeholderData: (prev) => prev,
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })

  const mockTasks = computed(() => createMockAdminTasks())
  const filteredMockTasks = computed(() =>
    mockTasks.value.filter((task) =>
      matchesMockTaskFilters(
        task,
        filterStatus.value,
        accountIdFilter.value,
        appliedSearchKeyword.value,
      ),
    ),
  )
  const tasks = computed(() =>
    isMockMode.value
      ? paginateMockTasks(filteredMockTasks.value, page.value)
      : tasksQuery.data.value?.data?.list || [],
  )
  const total = computed(() =>
    isMockMode.value ? filteredMockTasks.value.length : tasksQuery.data.value?.data?.total || 0,
  )
  const loading = computed(() => !isMockMode.value && tasksQuery.isLoading.value)
  const isError = computed(() => !isMockMode.value && tasksQuery.isError.value)
  const error = computed(() => (isError.value ? (tasksQuery.error.value ?? null) : null))
  const taskStats = computed<TaskStatusStats>(() => {
    if (isMockMode.value) return buildTaskStats(filteredMockTasks.value)
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
      if (isMockMode.value) return hasActiveTaskRows(tasks.value)
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
    if (isMockMode.value) return
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

function buildTaskStats(tasks: Task[]): TaskStatusStats {
  return tasks.reduce<TaskStatusStats>(
    (stats, task) => {
      if (task.status === ListAdminTasksStatus.running) stats.running += 1
      else if (task.status === ListAdminTasksStatus.pending) stats.pending += 1
      else if (task.status === ListAdminTasksStatus.completed) stats.completed += 1
      else if (task.status === ListAdminTasksStatus.failed) stats.failed += 1
      else if (task.status === ListAdminTasksStatus.cancelled) stats.cancelled += 1
      else stats.other += 1
      return stats
    },
    { running: 0, pending: 0, completed: 0, failed: 0, cancelled: 0, other: 0 },
  )
}

function paginateMockTasks(tasks: Task[], page: number) {
  const start = Math.max(0, page - 1) * TASK_MONITOR_PAGE_SIZE
  return tasks.slice(start, start + TASK_MONITOR_PAGE_SIZE)
}

function hasActiveTaskRows(tasks: Task[]) {
  return tasks.some(
    (task) =>
      task.status === ListAdminTasksStatus.pending || task.status === ListAdminTasksStatus.running,
  )
}

function matchesMockTaskFilters(
  task: Task,
  status: ListAdminTasksStatusValue | null,
  accountId: number | null,
  keyword: string,
) {
  if (status && task.status !== status) return false
  if (accountId != null && task.account_id !== accountId) return false
  if (!keyword) return true

  const value = keyword.toLowerCase()
  return [
    task.task_id,
    task.prompt,
    task.status,
    String(task.user_id),
    String(task.account_id || ''),
    String(task.assigned_channel_id || ''),
  ].some((field) => field.toLowerCase().includes(value))
}
