import { computed, ref, type Ref } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import accountApi from '../../../shared/api/account'
import type { Task } from '../../../shared/api/account'
import { createMonitorTaskColumns } from '../shared/monitorShared'

export function useTaskMonitor(options: {
  onShowDetail: (row: Task) => void
}) {
  const tasks = ref<Task[]>([])
  const loading = ref(false)
  const page = ref(1)
  const pageCount = ref(1)
  const TASK_MONITOR_PAGE_SIZE = 12
  const filterStatus = ref<string | null>(null)
  const searchKeyword = ref('')
  const monitorData = ref<{
    global_queue_len: number
    wait_queue_len: number
    running_tasks: number
    account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
  } | null>(null)

  const columns = computed<DataTableColumns<Task>>(() =>
    createMonitorTaskColumns({ onShowDetail: options.onShowDetail }),
  )

  function handleSearch() {
    page.value = 1
    void loadTasks()
  }

  async function loadTasks() {
    loading.value = true
    try {
      const params: { status?: string; page: number; page_size: number; user_id?: number; keyword?: string } = {
        status: filterStatus.value || undefined,
        page: page.value,
        page_size: TASK_MONITOR_PAGE_SIZE,
      }

      if (searchKeyword.value) {
        if (/^\d+$/.test(searchKeyword.value)) params.user_id = parseInt(searchKeyword.value)
        else params.keyword = searchKeyword.value
      }

      const res = await accountApi.listAdminTasks(params)
      if (res.code === 0) {
        tasks.value = res.data?.list || []
        pageCount.value = Math.max(1, Math.ceil((res.data?.total || 0) / TASK_MONITOR_PAGE_SIZE))
      }

      const monitorRes = await accountApi.getMonitor()
      if (monitorRes.code === 0) monitorData.value = monitorRes.data?.monitor || null
      else monitorData.value = null
    } catch {
      console.error('加载任务列表失败')
    } finally {
      loading.value = false
    }
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
