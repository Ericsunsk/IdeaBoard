import type { DataTableColumns } from 'naive-ui'
import type { Task } from '../../../shared/api/task'
import { createMonitorTaskColumns } from '../shared/monitorShared'
import { useGetAdminMonitor } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import { useListAdminTasks } from '../../../shared/api/generated-query/admin-tasks/admin-tasks'

export function useTaskMonitor(options: {
  onShowDetail: (row: Task) => void
}) {
  const page = ref(1)
  const TASK_MONITOR_PAGE_SIZE = 12
  const filterStatus = ref<string | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')

  const columns = computed<DataTableColumns<Task>>(() =>
    createMonitorTaskColumns({ onShowDetail: options.onShowDetail }),
  )

  function resolveParams() {
    const params: { status?: string; page: number; page_size: number; user_id?: number; keyword?: string } = {
      status: filterStatus.value || undefined,
      page: page.value,
      page_size: TASK_MONITOR_PAGE_SIZE,
    }
    if (appliedSearchKeyword.value) {
      if (/^\d+$/.test(appliedSearchKeyword.value)) params.user_id = parseInt(appliedSearchKeyword.value)
      else params.keyword = appliedSearchKeyword.value
    }
    return params
  }

  const tasksQuery = useListAdminTasks(computed(resolveParams), {
    query: {
      placeholderData: (prev) => prev,
    },
  })

  const monitorQuery = useGetAdminMonitor({
    query: {
      refetchInterval: 10_000,
    },
  })

  const tasks = computed(() => tasksQuery.data.value?.data?.list || [])
  const loading = computed(() => tasksQuery.isLoading.value || monitorQuery.isLoading.value)
  const pageCount = computed(() =>
    Math.max(1, Math.ceil((tasksQuery.data.value?.data?.total || 0) / TASK_MONITOR_PAGE_SIZE)),
  )
  const monitorData = computed(() => monitorQuery.data.value?.data?.monitor || null)

  function handleSearch() {
    page.value = 1
    appliedSearchKeyword.value = searchKeyword.value
  }

  watch(filterStatus, () => {
    page.value = 1
  })

  function loadTasks() {
    tasksQuery.refetch()
    monitorQuery.refetch()
  }

  return {
    tasks,
    loading,
    page,
    pageCount,
    filterStatus,
    searchKeyword,
    monitorData,
    columns,
    loadTasks,
    handleSearch,
  }
}
