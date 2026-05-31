import type { DataTableColumns } from 'naive-ui'
import type { Task } from '../../../../shared/api/task'
import { createMonitorTaskColumns } from '../../shared/lib/monitorShared'
import { useListAdminTasks } from '../../../../shared/api/generated-query/admin-tasks/admin-tasks'
import { calcDurationSeconds } from '../../../../shared/utils/dateTime'

const SLOW_TASK_SECONDS = 60
type SlowTaskRow = { task: Task; seconds: number }

export function useTaskMonitor(options: { onShowDetail: (row: Task) => void }) {
  const page = ref(1)
  const TASK_MONITOR_PAGE_SIZE = 12
  const filterStatus = ref<string | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')

  const columns = computed<DataTableColumns<Task>>(() =>
    createMonitorTaskColumns({ onShowDetail: options.onShowDetail }),
  )

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

  const tasksQuery = useListAdminTasks(computed(resolveParams), {
    query: {
      placeholderData: (prev) => prev,
    },
  })

  const tasks = computed(() => tasksQuery.data.value?.data?.list || [])
  const total = computed(() => tasksQuery.data.value?.data?.total || 0)
  const loading = computed(() => tasksQuery.isLoading.value)
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / TASK_MONITOR_PAGE_SIZE)))
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

  function handleSearch() {
    page.value = 1
    appliedSearchKeyword.value = searchKeyword.value
  }

  watch(filterStatus, () => {
    page.value = 1
  })

  function loadTasks() {
    tasksQuery.refetch()
  }

  return {
    tasks,
    total,
    loading,
    page,
    pageCount,
    pageStats,
    failedTasks,
    slowTasks,
    filterStatus,
    searchKeyword,
    columns,
    loadTasks,
    handleSearch,
  }
}
