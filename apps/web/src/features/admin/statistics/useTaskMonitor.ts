import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import type { DataTableColumns } from 'naive-ui'
import type { Task } from '../../../shared/api/account'
import { fetchAdminTasks, fetchAdminMonitor } from '../../../shared/query/taskQueries'
import { queryKeys } from '../../../shared/query/keys'
import { createMonitorTaskColumns } from '../shared/monitorShared'

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

  const tasksQuery = useQuery({
    queryKey: computed(() => queryKeys.admin.tasks(resolveParams())),
    queryFn: () => fetchAdminTasks(resolveParams()),
    placeholderData: (prev) => prev,
  })

  const monitorQuery = useQuery({
    queryKey: queryKeys.admin.monitor(),
    queryFn: fetchAdminMonitor,
    refetchInterval: 10_000,
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
