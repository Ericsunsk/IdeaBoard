import { computed, ref, watch } from 'vue'
import type { Task } from '../../../../shared/api/task'
import { useListAdminTasks } from '../../../../shared/api/generated-query/admin-tasks/admin-tasks'
import { calcDurationSeconds } from '../../../../shared/utils/dateTime'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'
import { createMockAdminTasks, useAdminMockMode } from '../../shared/lib/adminMockData'

export const TASK_MONITOR_PAGE_SIZE = 12

type TimedTaskRow = { task: Task; seconds: number }
type PageTaskStats = {
  running: number
  pending: number
  completed: number
  failed: number
  other: number
}

export function useTaskMonitor() {
  const isMockMode = useAdminMockMode()
  const page = ref(1)
  const filterStatus = ref<string | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const hasActiveTasks = ref(true)

  function resolveParams() {
    const params: {
      status?: string
      page: number
      page_size: number
      user_id?: number
      keyword?: string
    } = {
      status: filterStatus.value || undefined,
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
      matchesMockTaskFilters(task, filterStatus.value, appliedSearchKeyword.value),
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
  const pageStats = computed(() => buildPageTaskStats(tasks.value))
  const taskStats = computed<PageTaskStats>(() => {
    if (isMockMode.value) return buildPageTaskStats(filteredMockTasks.value)
    const data = tasksQuery.data.value?.data
    return {
      pending: data?.pending_count ?? 0,
      running: data?.running_count ?? 0,
      completed: data?.completed_count ?? 0,
      failed: data?.failed_count ?? 0,
      other: data?.other_count ?? 0,
    }
  })

  const activeTasks = computed<TimedTaskRow[]>(() =>
    tasks.value
      .map((task) => {
        if (task.status !== 'running' && task.status !== 'pending') return null
        if (!task.created_at) return null
        const seconds = calcDurationSeconds(task.created_at, new Date())
        if (seconds == null) return null
        return { task, seconds }
      })
      .filter((row): row is TimedTaskRow => !!row)
      .sort((a, b) => b.seconds - a.seconds)
      .slice(0, 5),
  )

  watch(
    tasks,
    (rows) => {
      hasActiveTasks.value = rows.some(
        (task) => task.status === 'pending' || task.status === 'running',
      )
    },
    { immediate: true },
  )

  function handleSearch() {
    page.value = 1
    appliedSearchKeyword.value = searchKeyword.value.trim()
  }

  watch(filterStatus, () => {
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
    page,
    taskStats,
    pageStats,
    activeTasks,
    filterStatus,
    searchKeyword,
    loadTasks,
    handleSearch,
  }
}

function buildPageTaskStats(tasks: Task[]): PageTaskStats {
  return tasks.reduce<PageTaskStats>(
    (stats, task) => {
      if (task.status === 'running') stats.running += 1
      else if (task.status === 'pending') stats.pending += 1
      else if (task.status === 'completed') stats.completed += 1
      else if (task.status === 'failed') stats.failed += 1
      else stats.other += 1
      return stats
    },
    { running: 0, pending: 0, completed: 0, failed: 0, other: 0 },
  )
}

function paginateMockTasks(tasks: Task[], page: number) {
  const start = Math.max(0, page - 1) * TASK_MONITOR_PAGE_SIZE
  return tasks.slice(start, start + TASK_MONITOR_PAGE_SIZE)
}

function matchesMockTaskFilters(task: Task, status: string | null, keyword: string) {
  if (status && task.status !== status) return false
  if (!keyword) return true

  const value = keyword.toLowerCase()
  return [
    task.task_id,
    task.prompt,
    task.status,
    String(task.user_id),
    String(task.account_id || ''),
    String(task.channel_id || ''),
  ].some((field) => field.toLowerCase().includes(value))
}
