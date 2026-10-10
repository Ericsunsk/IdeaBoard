<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="isMonitorError" severity="error">
      {{ monitorErrorMessage }}
    </AdminAlert>

    <AdminAlert v-if="isMonitorStale" severity="warning">
      <div class="flex flex-wrap items-center gap-2">
        <span>{{ MONITOR_MESSAGES.staleNotice }}</span>
        <AdminButton
          variant="ghost"
          size="sm"
          :class="adminTextButtonClass"
          :label="MONITOR_MESSAGES.retryConnection"
          @click="retryMonitorSnapshot"
        />
      </div>
    </AdminAlert>

    <template v-if="!isMonitorError">
      <TaskControlToolbar
        v-model:search-keyword="searchKeyword"
        :selected-preset="selectedPreset"
        :selected-time-range-key="selectedTimeRangeKey"
        :time-range-presets="timeRangePresets"
        @search="handleTaskSearch"
        @live-refresh="loadTasks"
        @select-time-range="selectTimeRange"
      />

      <TaskEventsCard
        v-model:charts-collapsed="chartsCollapsed"
        :filter-status="filterStatus"
        :metric-stats-snapshot="metricStatsSnapshot"
        :timeline-chart-data="timelineChartData"
        :timeline-chart-options="timelineChartOptions"
        :admin-chart-plugins="adminChartPlugins"
        :chart-height-px="monitorChartSpecs.heightPx"
        @toggle-filter-status="handleMetricPillClick"
      />
    </template>

    <AdminAlert v-if="isTaskListErrorActive" severity="error">
      {{ taskListErrorMessage }}
    </AdminAlert>

    <TaskActivityTable
      v-else
      :tasks="tasks"
      :loading="loading"
      :total-count="metricStatsSnapshot.total"
      :current-page="currentPage"
      :total-pages="totalPages"
      :can-previous="canPrevious"
      :can-next="canNext"
      :can-cancel-task="canCancelTask"
      :is-cancelling-task="isCancellingTask"
      @cancel-task="requestCancelTask"
      @show-detail="showDetail"
      @prev-page="goPreviousPage"
      @next-page="goNextPage"
    />

    <AdminTaskResultDialog
      v-model:visible="detailModalVisible"
      :task-id="isDemoMode ? (demoDetailTask?.task_id ?? '') : selectedDetailTaskID"
      :task="isDemoMode ? demoDetailTask : detailTask"
      :loading="isDemoMode ? false : isDetailLoading"
      :error-message="isDemoMode ? '' : detailErrorMessage"
      @retry="retryTaskDetail"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { formatDisplayErrorMessage } from '../../../../../shared/utils/errorMessages'
import { useAdminTaskCancellationFeedback } from '../../application/useAdminMonitorFeedback'
import AdminTaskResultDialog from '../AdminTaskResultDialog.vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import { adminDashboardRootClass, adminTextButtonClass } from '../../../shared/lib/uiClasses'
import TaskControlToolbar from '../components/TaskControlToolbar.vue'
import TaskEventsCard from '../components/TaskEventsCard.vue'
import TaskActivityTable from '../components/TaskActivityTable.vue'
import {
  type AdminTaskRow,
  buildAdminTaskListParams,
  useAdminTaskList,
} from '../../application/useAdminTaskList'
import { timeRangePresets } from '../../domain/timeRange'
import { useAdminTaskCancel } from '../../application/useAdminTaskCancel'
import { useAdminTaskDetailResource, useAdminTaskListResource } from '../../infrastructure/resource'
import { useAdminTaskRuntimeSnapshot } from '../../application/useAdminTaskRuntimeSnapshot'
import { useAdminConfirm } from '../../../shared/composables/useAdminConfirm'
import {
  ADMIN_MONITOR_CHART_PAGE_SIZE,
  ADMIN_MONITOR_REFRESH_INTERVAL_MS,
} from '../../domain/displayPolicy'
import { DEMO_MODE_QUERY_VALUE, createDemoTaskDetail } from '../../application/demoData'
import { useMonitorDemo } from '../../application/useMonitorDemo'
import { useMonitorTimeline } from '../../application/useMonitorTimeline'
import {
  monitorChartSpecs,
  type TaskMetricStatsSnapshot,
} from '../../../shared/lib/taskMetricTokens'
import { stringParam } from '../../../../../shared/router/queryParams'
import { MONITOR_MESSAGES } from './messages'

const adminConfirm = useAdminConfirm()
const taskCancellationFeedback = useAdminTaskCancellationFeedback()
const { isCancellableTask, isCancellingTask, requestCancelTask } = useAdminTaskCancel({
  confirm: adminConfirm,
  feedback: taskCancellationFeedback,
})

const route = useRoute()

// 显式演示模式：仅当 URL 携带 ?demo=1 时生效，默认关闭。
// 演示数据只做界面预览，取消等写操作与真实请求一律禁用；详情用本地数据预览。
// 注意：必须位于所有传给 resource/query 的 computed 之前，避免 setup 求值期 TDZ。
const isDemoMode = computed(() => stringParam(route.query.demo) === DEMO_MODE_QUERY_VALUE)

const detailModalVisible = ref(false)
const selectedDetailTaskID = ref('')
const detailQuery = useAdminTaskDetailResource(selectedDetailTaskID, {
  query: {
    enabled: computed(
      () => !isDemoMode.value && detailModalVisible.value && selectedDetailTaskID.value.length > 0,
    ),
  },
})
const detailTask = computed(() => {
  const task = detailQuery.data.value?.task
  return task?.task_id === selectedDetailTaskID.value ? task : null
})
const isDetailLoading = computed(
  () => detailModalVisible.value && detailTask.value === null && !detailQuery.isError.value,
)
const detailErrorMessage = computed(() =>
  detailQuery.isError.value
    ? formatDisplayErrorMessage(detailQuery.error.value, MONITOR_MESSAGES.detailLoadFailed)
    : '',
)

const {
  tasks: rawTasks,
  total,
  loading: rawLoading,
  isError: isTaskListError,
  error: taskListError,
  pagination,
  taskStats: rawTaskStats,
  filterStatus,
  searchKeyword,
  appliedSearchKeyword,
  loadTasks,
  handleSearch: applyTaskSearch,
} = useAdminTaskList()

// 图表专用全量行：与表格同过滤语义（status + 已应用 keyword），
// 取最近 100 行做区间密度，表格分页行代表不了区间分布。
const chartListParams = computed(() =>
  buildAdminTaskListParams({
    status: filterStatus.value,
    keyword: appliedSearchKeyword.value,
    page: 1,
    pageSize: ADMIN_MONITOR_CHART_PAGE_SIZE,
  }),
)
const chartListQuery = useAdminTaskListResource(chartListParams, {
  query: {
    enabled: computed(() => !isDemoMode.value),
    refetchInterval: ADMIN_MONITOR_REFRESH_INTERVAL_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
  },
})
const chartRows = computed(() => chartListQuery.data.value?.list ?? [])

const {
  selectedTimeRangeKey,
  selectedPreset,
  selectTimeRange,
  timelineChartData,
  timelineChartOptions,
  adminChartPlugins,
} = useMonitorTimeline({
  isDemoMode,
  demoRows: () => demoFiltered.value,
  chartRows,
  filterStatus,
})

const {
  demoTasks,
  demoStats,
  demoFiltered,
  demoSource,
  demoPagination,
  demoDetailTask,
  resetDemoPage,
} = useMonitorDemo({
  filterStatus,
  appliedSearchKeyword,
  isDemoMode,
  selectedPreset,
})

// 真实模式以服务端筛选为准（status/keyword 经 query key 下发），
// 本地不再二次过滤，避免“翻页丢结果”与 pill 计数对不上。
const tasks = computed(() => (isDemoMode.value ? demoTasks.value : rawTasks.value))

const loading = computed(() => (isDemoMode.value ? false : rawLoading.value))

const isTaskListErrorActive = computed(() => !isDemoMode.value && isTaskListError.value)

const taskStats = computed(() => (isDemoMode.value ? demoStats.value : rawTaskStats.value))

const metricStatsSnapshot = computed<TaskMetricStatsSnapshot>(() => ({
  total: isDemoMode.value ? demoSource.value.length : total.value,
  pending: taskStats.value.pending,
  running: taskStats.value.running,
  completed: taskStats.value.completed,
  failed: taskStats.value.failed,
  cancelled: taskStats.value.cancelled,
}))

const activePagination = computed(() =>
  isDemoMode.value
    ? {
        currentPage: demoPagination.currentPage.value,
        totalPages: demoPagination.totalPages.value,
        canPrevious: demoPagination.canPrevious.value,
        canNext: demoPagination.canNext.value,
        previous: demoPagination.previous,
        next: demoPagination.next,
      }
    : {
        currentPage: pagination.currentPage.value,
        totalPages: pagination.totalPages.value,
        canPrevious: pagination.canPrevious.value,
        canNext: pagination.canNext.value,
        previous: pagination.previous,
        next: pagination.next,
      },
)

// 演示模式禁用触达真实后端的行级写操作（取消）；详情弹窗用本地数据预览。
const canCancelTask = (row: AdminTaskRow) => !isDemoMode.value && isCancellableTask(row)

function handleMetricPillClick(status: string | null) {
  toggleFilterStatus(status as typeof filterStatus.value)
}

const currentPage = computed(() => activePagination.value.currentPage)
const totalPages = computed(() => activePagination.value.totalPages)
const canPrevious = computed(() => activePagination.value.canPrevious)
const canNext = computed(() => activePagination.value.canNext)
function goPreviousPage() {
  activePagination.value.previous()
}
function goNextPage() {
  activePagination.value.next()
}
const {
  isStale: isMonitorStale,
  isError: isMonitorError,
  error: monitorError,
  retry: retryMonitorSnapshot,
} = useAdminTaskRuntimeSnapshot()

const taskListErrorMessage = computed(() =>
  formatDisplayErrorMessage(taskListError.value, MONITOR_MESSAGES.listLoadFailed),
)
const monitorErrorMessage = computed(() =>
  formatDisplayErrorMessage(monitorError.value, MONITOR_MESSAGES.snapshotLoadFailed),
)
function handleTaskSearch() {
  resetDemoPage()
  applyTaskSearch()
}

const chartsCollapsed = ref(false)

function toggleFilterStatus(status: typeof filterStatus.value) {
  if (filterStatus.value === status) {
    filterStatus.value = null
  } else {
    filterStatus.value = status
  }
}

function showDetail(task: AdminTaskRow) {
  if (isDemoMode.value) {
    // 演示模式用行内数据本地拼装详情，不请求真实接口；
    // 同时清空残留真实 ID，避免后台误触发真实详情请求。
    selectedDetailTaskID.value = ''
    demoDetailTask.value = createDemoTaskDetail(task)
  } else {
    demoDetailTask.value = null
    selectedDetailTaskID.value = task.task_id
  }
  detailModalVisible.value = true
}

function retryTaskDetail() {
  void detailQuery.refetch()
}
</script>
