<template>
  <div :class="adminDashboardRootClass">
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

    <TaskActivityTable
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
      @retry="retryTaskDetail"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  useAdminTaskCancellationFeedback,
  useAdminTaskLoadFeedback,
} from '../../session/useAdminMonitorFeedback'
import { useFailureToast } from '@/core/composables/useFailureToast'
import AdminTaskResultDialog from '../AdminTaskResultDialog.vue'
import { AdminAlert } from '../../../shared'
import { AdminButton } from '../../../shared'
import { adminDashboardRootClass, adminTextButtonClass } from '../../../shared'
import TaskControlToolbar from '../components/TaskControlToolbar.vue'
import TaskEventsCard from '../components/TaskEventsCard.vue'
import TaskActivityTable from '../components/TaskActivityTable.vue'
import { type AdminTaskRow, useAdminTaskList } from '../../session/useAdminTaskList'
import { timeRangePresets } from '../../core/timeRange'
import { useAdminTaskCancel } from '../../session/useAdminTaskCancel'
import { useAdminTaskDetailResource } from '../../resource'
import { useAdminTaskRuntimeSnapshot } from '../../session/useAdminTaskRuntimeSnapshot'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import { useMonitorTimeline } from '../../session/useMonitorTimeline'
import { monitorChartSpecs, type TaskMetricStatsSnapshot } from '../../../shared'
import { MONITOR_MESSAGES } from './messages'

const adminConfirm = useConfirm()
const taskCancellationFeedback = useAdminTaskCancellationFeedback()
const taskLoadFeedback = useAdminTaskLoadFeedback()
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

const {
  tasks,
  chartRows,
  total,
  loading: isTaskListLoading,
  isError: isTaskListError,
  error: taskListError,
  pagination,
  taskStats,
  filterStatus,
  searchKeyword,
  loadTasks,
  handleSearch: handleTaskSearch,
} = useAdminTaskList()

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

// 页级/表级/详情行内错误文案已删除：失败只走 toast，表格与工具栏即恢复入口；
// stale 提示保留（无 toast 覆盖的新鲜度信息）。
useFailureToast(isMonitorError, taskLoadFeedback.snapshotLoadFailed, {
  shouldFireOnMount: false,
  resolveError: () => monitorError.value,
})
useFailureToast(isTaskListError, taskLoadFeedback.listLoadFailed, {
  shouldFireOnMount: false,
  resolveError: () => taskListError.value,
})
useFailureToast(() => detailQuery.isError.value, taskLoadFeedback.detailLoadFailed, {
  shouldFireOnMount: false,
  resolveError: () => detailQuery.error.value,
})
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
