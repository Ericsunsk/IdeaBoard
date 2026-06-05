import type { Task } from '../../../../shared/api/task'
import { useListAdminTasks } from '../../../../shared/api/generated-query/admin-tasks/admin-tasks'
import { calcDurationSeconds } from '../../../../shared/utils/dateTime'
import { useAdminQueryPolling } from '../../shared/composables/useAdminQueryPolling'

const SLOW_TASK_SECONDS = 60
export const TASK_MONITOR_PAGE_SIZE = 12
type SlowTaskRow = { task: Task; seconds: number }

export function useTaskMonitor() {
  const page = ref(1)
  const filterStatus = ref<string | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')

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
        params.user_id = parseInt(appliedSearchKeyword.value)
      else params.keyword = appliedSearchKeyword.value
    }
    return params
  }

  const hasActiveTasks = ref(true)

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
  const pageStats = computed(() => {
    const stats = {
      running: 0,
      pending: 0,
      failed: 0,
    }
    tasks.value.forEach((task) => {
      if (task.status === 'running') stats.running += 1
      else if (task.status === 'pending') stats.pending += 1
      else if (task.status === 'failed') stats.failed += 1
    })
    return stats
  })

  const failedTasks = computed(() => tasks.value.filter((task) => task.status === 'failed'))
  const activeTasks = computed<SlowTaskRow[]>(() =>
    tasks.value
      .map((task) => {
        if (task.status !== 'running' && task.status !== 'pending') return null
        if (!task.created_at) return null
        const seconds = calcDurationSeconds(task.created_at, new Date())
        if (seconds == null) return null
        return { task, seconds }
      })
      .filter((row): row is SlowTaskRow => !!row)
      .sort((a, b) => b.seconds - a.seconds)
      .slice(0, 5),
  )
  const slowTasks = computed<SlowTaskRow[]>(() =>
    tasks.value
      .map((task) => {
        if (!task.created_at) return null
        const end = task.completed_at || new Date()
        const seconds = calcDurationSeconds(task.created_at, end)
        if (seconds == null || seconds < SLOW_TASK_SECONDS) return null
        return { task, seconds }
      })
      .filter((row): row is SlowTaskRow => !!row)
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
    tasksQuery.refetch()
  }

  return {
    tasks,
    total,
    loading,
    page,
    pageStats,
    failedTasks,
    activeTasks,
    slowTasks,
    filterStatus,
    searchKeyword,
    loadTasks,
    handleSearch,
  }
}
