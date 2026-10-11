import { computed, ref, watch, type Ref } from 'vue'
import { usePagePagination } from '../../shared'
import { ADMIN_MONITOR_PAGE_SIZE } from '../core/displayPolicy'
import { createDemoTaskDetail, createDemoMonitorList, createDemoMonitorStats } from './demoData'
import type { TimeRangePreset } from '../core/timeRange'
import type { AdminTaskRow } from './useAdminTaskList'
import type { AdminMonitorDetailModel } from '../resource'
import type { TaskStatus } from '../../shared'

type MonitorDemoOptions = {
  filterStatus: Ref<TaskStatus | null>
  appliedSearchKeyword: Ref<string>
  isDemoMode: Ref<boolean>
  selectedPreset: Ref<TimeRangePreset>
}

function filterDemoRows(
  list: AdminTaskRow[],
  filterStatus: TaskStatus | null,
  appliedKeyword: string,
) {
  // 与真实模式保持同一语义：只认已应用的 keyword（Enter 生效）。
  let result = list
  if (filterStatus) {
    result = result.filter((item) => item.status === filterStatus)
  }
  if (appliedKeyword.trim()) {
    const kw = appliedKeyword.trim().toLowerCase()
    result = result.filter(
      (item) =>
        item.prompt.toLowerCase().includes(kw) ||
        item.user_display_name.toLowerCase().includes(kw) ||
        item.task_id.toLowerCase().includes(kw),
    )
  }
  return result
}

export function useMonitorDemo(options: MonitorDemoOptions) {
  const { filterStatus, appliedSearchKeyword, isDemoMode, selectedPreset } = options

  const demoSource = computed(() =>
    isDemoMode.value ? createDemoMonitorList(selectedPreset.value.durationMs) : [],
  )
  const demoFiltered = computed(() =>
    filterDemoRows(demoSource.value, filterStatus.value, appliedSearchKeyword.value),
  )
  const demoPage = ref(1)
  const demoPageSize = computed(() => ADMIN_MONITOR_PAGE_SIZE)
  const demoPagination = usePagePagination({
    currentPage: demoPage,
    totalRecords: computed(() => demoFiltered.value.length),
    pageSize: demoPageSize,
    pending: ref(false),
  })
  const demoTasks = computed(() => {
    const start = (demoPagination.currentPage.value - 1) * demoPageSize.value
    return demoFiltered.value.slice(start, start + demoPageSize.value)
  })
  const demoStats = computed(() => createDemoMonitorStats(demoSource.value))

  const demoDetailTask = ref<AdminMonitorDetailModel | null>(null)

  function resetDemoPage() {
    demoPage.value = 1
  }

  watch([filterStatus, appliedSearchKeyword, isDemoMode], resetDemoPage, { flush: 'sync' })

  // 演示详情用行内数据本地拼装，不请求真实接口。
  function buildDemoDetail(row: AdminTaskRow) {
    demoDetailTask.value = createDemoTaskDetail(row)
  }

  function clearDemoDetail() {
    demoDetailTask.value = null
  }

  return {
    demoTasks,
    demoStats,
    demoFiltered,
    demoSource,
    demoPagination,
    demoDetailTask,
    resetDemoPage,
    buildDemoDetail,
    clearDemoDetail,
  }
}
