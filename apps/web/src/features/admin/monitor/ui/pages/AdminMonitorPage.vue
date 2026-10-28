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

    <AdminAlert v-if="isTaskListError" severity="error">
      {{ taskListErrorMessage }}
    </AdminAlert>

    <TaskActivityTable
      v-else
      :tasks="tasks"
      :loading="isTaskListLoading"
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
      :task-id="selectedDetailTaskID"
      :task="detailTask"
      :loading="isDetailLoading"
      :error-message="detailErrorMessage"
      @retry="retryTaskDetail"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { formatDisplayErrorMessage } from '@/core/utils/errorMessages'
import { useAdminTaskCancellationFeedback } from '../../session/useAdminMonitorFeedback'
import AdminTaskResultDialog from '../AdminTaskResultDialog.vue'
import { AdminAlert } from '../../../shared'
import { AdminButton } from '../../../shared'
import { adminDashboardRootClass, adminTextButtonClass } from '../../../shared'
import TaskControlToolbar from '../components/TaskControlToolbar.vue'
import TaskEventsCard from '../components/TaskEventsCard.vue'
import TaskActivityTable from '../components/TaskActivityTable.vue'
import {
  type AdminTaskRow,
  buildAdminTaskListParams,
  useAdminTaskList,
} from '../../session/useAdminTaskList'
import { timeRangePresets } from '../../core/timeRange'
import { useAdminTaskCancel } from '../../session/useAdminTaskCancel'
import { useAdminTaskDetailResource, useAdminTaskListResource } from '../../resource'
import { useAdminTaskRuntimeSnapshot } from '../../session/useAdminTaskRuntimeSnapshot'
import { useAdminConfirm } from '../../../shared'
import {
  ADMIN_MONITOR_CHART_PAGE_SIZE,
  ADMIN_MONITOR_REFRESH_INTERVAL_MS,
} from '../../core/displayPolicy'
import { useMonitorTimeline } from '../../session/useMonitorTimeline'
import { monitorChartSpecs, type TaskMetricStatsSnapshot } from '../../../shared'
import { MONITOR_MESSAGES } from './messages'

const adminConfirm = useAdminConfirm()
const taskCancellationFeedback = useAdminTaskCancellationFeedback()
const {
  isCancellableTask: canCancelTask,
  isCancellingTask,
  requestCancelTask,
} = useAdminTaskCancel({
  confirm: adminConfirm,
  feedback: taskCancellationFeedback,
})

const detailModalVisible = ref(false)
const selectedDetailTaskID = ref('')
const detailQuery = useAdminTaskDetailResource(selectedDetailTaskID, {
  query: {
    enabled: computed(() => detailModalVisible.value && selectedDetailTaskID.value.length > 0),
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
  tasks,
  total,
  loading: isTaskListLoading,
  isError: isTaskListError,
  error: taskListError,
  pagination,
  taskStats,
  filterStatus,
  searchKeyword,
  appliedSearchKeyword,
  loadTasks,
  handleSearch: handleTaskSearch,
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
  chartRows,
  filterStatus,
})

const metricStatsSnapshot = computed<TaskMetricStatsSnapshot>(() => ({
  total: total.value,
  pending: taskStats.value.pending,
  running: taskStats.value.running,
  completed: taskStats.value.completed,
  failed: taskStats.value.failed,
  cancelled: taskStats.value.cancelled,
}))

const {
  currentPage,
  totalPages,
  canPrevious,
  canNext,
  previous: goPreviousPage,
  next: goNextPage,
} = pagination

function handleMetricPillClick(status: string | null) {
  toggleFilterStatus(status as typeof filterStatus.value)
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
const chartsCollapsed = ref(false)

function toggleFilterStatus(status: typeof filterStatus.value) {
  if (filterStatus.value === status) {
    filterStatus.value = null
  } else {
    filterStatus.value = status
  }
}

function showDetail(task: AdminTaskRow) {
  selectedDetailTaskID.value = task.task_id
  detailModalVisible.value = true
}

function retryTaskDetail() {
  void detailQuery.refetch()
}
</script>
