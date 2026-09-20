<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="isMonitorError" severity="error">
      {{ monitorErrorMessage }}
    </AdminAlert>

    <AdminAlert v-if="isMonitorStale" severity="warning">
      <div class="flex flex-wrap items-center gap-2">
        <span>任务监控连接不稳定，当前显示上次成功数据。</span>
        <AdminButton
          variant="ghost"
          size="sm"
          :class="adminTextButtonClass"
          label="重试连接"
          @click="retryMonitorSnapshot"
        />
      </div>
    </AdminAlert>

    <template v-if="!isMonitorError">
      <!-- 1. 最上方：顶栏控制工具栏 (与页面背景同色，1:1 对齐样板) -->
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：侧边栏抽屉按钮 + Events 选择器 + 搜索输入框 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <!-- 侧边栏按钮 (样式占位，后续接功能) -->
          <button
            type="button"
            aria-disabled="true"
            tabindex="-1"
            class="relative flex size-9 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-white text-admin-text-2"
            title="切换侧边导航"
          >
            <SidebarCollapse class="size-4" />
            <span class="absolute top-1.5 right-1.5 size-1.5 rounded-full bg-blue-500"></span>
          </button>

          <!-- 数据集选择器 (样式占位，后续接功能) -->
          <div class="relative inline-flex items-center">
            <button
              type="button"
              aria-disabled="true"
              tabindex="-1"
              class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-semibold text-admin-text-1"
            >
              <Table class="size-3.5 text-admin-text-2" />
              <span>Events</span>
              <NavArrowDown class="size-3.5 text-admin-text-3" />
            </button>
          </div>

          <!-- 搜索输入框: 🔍 使用查询语言搜索... ❓ -->
          <div class="relative flex min-w-[200px] flex-1 items-center">
            <div class="pointer-events-none absolute left-3 flex items-center text-admin-text-3">
              <Search class="size-4" />
            </div>
            <input
              v-model="searchKeyword"
              type="text"
              aria-label="使用查询语言搜索"
              class="h-9 w-full rounded-lg border border-admin-border-soft bg-white pr-9 pl-9 text-xs text-admin-text-1 placeholder:text-admin-text-3 transition-colors focus-visible:border-admin-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20"
              placeholder="使用查询语言搜索..."
              @keydown.enter="handleTaskSearch"
            />
            <div class="pointer-events-none absolute right-3 flex items-center text-admin-text-3">
              <HelpCircle class="size-4" />
            </div>
          </div>
        </div>

        <!-- 右侧：▶ 实时 + 📅 过去 1 小时 + ··· 更多 + 分页器 -->
        <div class="flex shrink-0 items-center gap-2">
          <!-- 实时按钮 -->
          <button
            type="button"
            class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
            @click="loadTasks"
          >
            <Play class="size-3 fill-admin-text-1 text-admin-text-1" />
            <span>实时</span>
          </button>

          <!-- 时间范围下拉按钮与弹出层 (shadcn-vue 原生原子组件) -->
          <AdminPopover v-model:open="timeRangePopoverOpen">
            <AdminPopoverTrigger as-child>
              <button
                type="button"
                class="flex h-9 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
                :aria-expanded="timeRangePopoverOpen"
                aria-haspopup="listbox"
                aria-label="选择时间范围"
              >
                <Calendar class="size-3.5 text-admin-text-2" />
                <span>{{ selectedPreset.label }}</span>
                <NavArrowDown
                  class="size-3.5 text-admin-text-3 transition-transform"
                  :class="timeRangePopoverOpen ? 'rotate-180' : ''"
                />
              </button>
            </AdminPopoverTrigger>

            <AdminPopoverContent align="end" :side-offset="4">
              <div class="flex flex-col gap-0.5" role="listbox">
                <button
                  v-for="preset in timeRangePresets"
                  :key="preset.key"
                  type="button"
                  role="option"
                  :aria-selected="selectedTimeRangeKey === preset.key"
                  :class="[
                    'flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors cursor-pointer',
                    selectedTimeRangeKey === preset.key
                      ? 'bg-blue-50 text-blue-600 font-semibold'
                      : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
                  ]"
                  @click="selectTimeRange(preset.key)"
                >
                  <span>{{ preset.label }}</span>
                  <Check v-if="selectedTimeRangeKey === preset.key" class="size-3.5 text-blue-600 shrink-0" />
                </button>
              </div>
            </AdminPopoverContent>
          </AdminPopover>

          <!-- 更多按钮 (样式占位，后续接功能) -->
          <AdminButton
            variant="ghost"
            class="size-9 rounded-lg border border-admin-border-soft bg-white p-0 text-admin-text-2"
            aria-label="更多操作"
            aria-disabled="true"
            tabindex="-1"
          >
            <MoreHoriz class="size-4 text-admin-text-2" />
          </AdminButton>
        </div>
      </div>

      <!-- 2. 中间：Events 卡片 (表头底色采用 bg-admin-bg，带外层边框；表身白色卡片撑满下方) -->
      <div class="flex w-full flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-admin-bg">
        <!-- 表头：带外层边框，底色同页面背景 (bg-transparent) -->
        <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5">
          <!-- 标题 Events + ℹ 提示图标 -->
          <div class="flex items-center gap-1.5">
            <span class="text-sm font-semibold text-admin-text-1">Events</span>
            <span title="查看事件时序概览" class="flex size-4 cursor-pointer items-center justify-center rounded-full text-admin-text-3 hover:text-admin-text-1">
              <InfoCircle class="size-3.5" />
            </span>
          </div>

          <!-- 右侧：数据类型指标胶囊 + 折叠按钮 -->
          <div class="flex items-center gap-2">
            <button
              v-for="metric in taskMetricDefinitions"
              :key="metric.key"
              type="button"
              :class="[
                'inline-flex items-center gap-1.5 text-xs font-medium cursor-pointer transition-colors px-2 py-0.5 rounded',
                filterStatus === metric.status
                  ? metric.activePillClass
                  : 'text-admin-text-2 hover:text-admin-text-1',
              ]"
              :title="metric.pillTitle"
              @click="handleMetricPillClick(metric.status)"
            >
              <span :class="['size-2 rounded-full', metric.dotClass]"></span>
              <span>{{ metric.getCount(metricStatsSnapshot) }} {{ metric.label }}</span>
            </button>

            <!-- 折叠按钮: 独立的圆角矩形小按钮 (^) -->
            <button
              type="button"
              :class="adminCompactIconButtonClass"
              :title="chartsCollapsed ? '展开图表' : '折叠图表'"
              :aria-label="chartsCollapsed ? '展开图表' : '折叠图表'"
              @click="chartsCollapsed = !chartsCollapsed"
            >
              <NavArrowUp v-if="!chartsCollapsed" class="size-4" />
              <NavArrowDown v-else class="size-4" />
            </button>
          </div>
        </div>

        <!-- 表身：白色卡片 (bg-white)，撑满卡片下方，衔接处圆角与边框分界 -->
        <div
          v-show="!chartsCollapsed"
          class="w-full rounded-t-xl border-t border-admin-border-soft bg-white px-4 py-3"
          :style="{ height: `${monitorChartSpecs.heightPx}px` }"
        >
          <ChartRenderer
            type="line"
            :data="timelineChartData"
            :options="timelineChartOptions"
            :plugins="adminChartPlugins"
            class="h-full w-full"
          />
        </div>
      </div>
    </template>

    <AdminAlert v-if="isTaskListErrorActive" severity="error">
      {{ taskListErrorMessage }}
    </AdminAlert>

    <!-- 3. 下方：事件表格卡片 (灰底容器 + 灰底表头，白底数据行，灰底底栏分页) -->
    <div
      v-else
      :class="adminTableCardClass"
    >
      <AdminTable
        :value="tasks"
        :loading="loading"
        data-key="task_id"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
      >
          <template #empty>
            <AdminEmptyState title="暂无事件日志" />
          </template>

          <!-- 第 1 列：时间戳 GMT+8 ↓ [✏ 字段] -->
          <AdminColumn :style="{ width: adminMonitorColumnWidths.timestamp }">
            <template #header>
              <div class="flex items-center gap-1.5 font-sans">
                <span class="font-semibold text-admin-text-2 text-xs">时间戳</span>
                <span class="font-mono text-[11px] text-admin-text-3 font-normal">GMT+8</span>
              </div>
            </template>
            <template #body="{ data }">
              <div class="flex items-center gap-2 font-mono text-xs">
                <!-- 竖直状态指示条 (复用 Events 表的状态 Token 颜色) -->
                <span
                  :class="[
                    'h-3.5 w-[3px] shrink-0 rounded-full',
                    getTaskRowView(data).metric?.dotClass || 'bg-slate-400',
                  ]"
                />
                <!-- 精确时间戳 2026-09-17 12:18:35 -->
                <span class="font-medium text-admin-text-1">
                  {{ formatTaskExactTimestamp(data) }}
                </span>
              </div>
            </template>
          </AdminColumn>

          <!-- 耗时列：完成时间 - 创建时间，未完成展示占位 -->
          <AdminColumn header="Duration" :style="{ width: adminMonitorColumnWidths.duration }">
            <template #body="{ data }">
              <span class="font-mono text-xs text-admin-text-1">
                {{ formatTaskDuration(data) }}
              </span>
            </template>
          </AdminColumn>

          <!-- 第 2 列：Status (复用 Events 表状态 Token 胶囊与点) -->
          <AdminColumn header="Status" :style="{ width: adminMonitorColumnWidths.status }">
            <template #body="{ data }">
              <TaskStatusCell :row="data" />
            </template>
          </AdminColumn>

          <!-- 第 3 列：Service (feature 图标 + slug) -->
          <AdminColumn header="Service" :style="{ width: adminMonitorColumnWidths.service }">
            <template #body="{ data }">
              <div class="flex items-center gap-1.5 font-mono text-xs text-admin-text-1">
                <component
                  :is="getTaskRowView(data).featureIcon"
                  v-if="getTaskRowView(data).featureIcon"
                  class="size-4 shrink-0 text-admin-text-2"
                />
                <FastArrowRight v-else class="size-3.5 text-orange-500 shrink-0" />
                <span class="truncate text-slate-700">{{ getTaskRowView(data).featureSlug || '-' }}</span>
              </div>
            </template>
          </AdminColumn>

          <!-- 模型列：实际解析模型 -->
          <AdminColumn header="Model" :style="{ width: adminMonitorColumnWidths.model }">
            <template #body="{ data }">
              <span class="truncate font-mono text-xs text-admin-text-1" :title="getTaskModelLabel(data) || '-'">
                {{ getTaskModelLabel(data) || '-' }}
              </span>
            </template>
          </AdminColumn>

          <!-- 第 4 列：Message (事件 Prompt / 诊断内容，服务名前缀已由 Service 列承载) -->
          <AdminColumn header="Message" :style="{ width: adminMonitorColumnWidths.message }">
            <template #body="{ data }">
              <div class="max-w-[160px] truncate font-mono text-xs text-slate-800" :title="data.prompt || '-'">
                {{ getTaskRowView(data).message || '-' }}
              </div>
            </template>
          </AdminColumn>

          <!-- 花费列：实际扣账 Idea 数 -->
          <AdminColumn header="Credits" :style="{ width: adminMonitorColumnWidths.credits }">
            <template #body="{ data }">
              <span class="font-mono text-xs text-admin-text-1">
                {{ data.credits || 0 }} <span class="text-admin-text-3">Idea</span>
              </span>
            </template>
          </AdminColumn>

          <!-- 结果列：最多 3 张缩略图，超出 +N，点击进详情 -->
          <AdminColumn header="Result" :style="{ width: adminMonitorColumnWidths.result }">
            <template #body="{ data }">
              <TaskResultCell :row="data" @inspect="showDetail" />
            </template>
          </AdminColumn>

          <!-- 第 7 列：操作 (仅取消活跃任务) -->
          <AdminColumn :style="{ width: adminMonitorColumnWidths.actions }">
            <template #body="{ data }">
              <div class="flex items-center justify-end gap-1">
                <AdminButton
                  v-if="canCancelTask(data)"
                  variant="ghost"
                  size="sm"
                  severity="warn"
                  :class="adminTextButtonClass"
                  label="取消"
                  :loading="isCancellingTask(data.task_id)"
                  aria-label="取消任务"
                  @click="requestCancelTask(data)"
                />
              </div>
            </template>
          </AdminColumn>

          <!-- 第 8 列：More (更多图标，点击进详情弹窗；演示模式用本地数据预览) -->
          <AdminColumn header="More" :style="{ width: adminMonitorColumnWidths.more }">
            <template #body="{ data }">
              <div class="flex items-center justify-start">
                <button
                  type="button"
                  class="flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-admin-border-soft bg-white text-admin-text-2 transition-colors hover:bg-admin-surface-muted"
                  aria-label="更多任务详情"
                  @click="showDetail(data)"
                >
                  <MoreHoriz class="size-4" />
                </button>
              </div>
            </template>
          </AdminColumn>
        </AdminTable>

      <!-- 表格底栏 (Card Footer)：灰底带上边框，左侧记录总数，右侧分页控制器 -->
      <div class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg px-4 py-2.5">
        <span class="text-xs font-normal text-admin-text-3">
          共 {{ metricStatsSnapshot.total }} 条记录
        </span>

        <div class="flex items-center gap-2">
          <AdminPagerControls
            :current-page="currentPage"
            :total-pages="totalPages"
            :prev-disabled="!canPrevious"
            :next-disabled="!canNext"
            @prev="goPreviousPage"
            @next="goNextPage"
          />
        </div>
      </div>
    </div>

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
import {
  AdminPopover,
  AdminPopoverContent,
  AdminPopoverTrigger,
} from '../../../shared/components/adminPopoverAtoms'
import AdminColumn from '../../../shared/components/AdminColumn.vue'
import {
  Calendar,
  Check,
  FastArrowRight,
  HelpCircle,
  InfoCircle,
  MoreHoriz,
  NavArrowDown,
  NavArrowUp,
  Play,
  Search,
  SidebarCollapse,
  Table,
} from '@iconoir/vue'
import { formatDisplayErrorMessage } from '../../../../../shared/utils/errorMessages'
import { useAdminTaskCancellationFeedback } from '../../application/useAdminMonitorFeedback'
import AdminTaskResultDialog from '../AdminTaskResultDialog.vue'
import TaskResultCell from '../TaskResultCell.vue'
import TaskStatusCell from '../TaskStatusCell.vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminEmptyState from '../../../shared/components/AdminEmptyState.vue'
import AdminPagerControls from '../../../shared/components/AdminPagerControls.vue'
import {
  adminCompactIconButtonClass,
  adminDashboardRootClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
  adminTextButtonClass,
} from '../../../shared/lib/uiClasses'
import AdminTable from '../../../shared/components/AdminTable.vue'
import ChartRenderer from '../../../../../shared/components/ChartRenderer.vue'
import {
  adminMonitorColumnWidths,
  formatTaskExactTimestamp,
  getTaskModelLabel,
  getTaskRowView,
} from '../columns'
import { formatTaskDuration } from '../../domain/taskResultPresentation'
import { type AdminTaskRow, buildAdminTaskListParams, useAdminTaskList } from '../../application/useAdminTaskList'
import {
  timeRangePresets,
} from '../../domain/timeRange'
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
  taskMetricDefinitions,
  monitorChartSpecs,
  type TaskMetricStatsSnapshot,
} from '../../../../../shared/domain/taskMetricTokens'

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
const isDemoMode = computed(() => route.query.demo === DEMO_MODE_QUERY_VALUE)

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
    ? formatDisplayErrorMessage(detailQuery.error.value, '加载任务详情失败')
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
    placeholderData: (previous) => previous,
    refetchInterval: ADMIN_MONITOR_REFRESH_INTERVAL_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
  },
})
const chartRows = computed(() => chartListQuery.data.value?.list ?? [])

const {
  selectedTimeRangeKey,
  timeRangePopoverOpen,
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

function handleMetricPillClick(status: typeof filterStatus.value) {
  toggleFilterStatus(status)
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
  formatDisplayErrorMessage(taskListError.value, '加载任务列表失败'),
)
const monitorErrorMessage = computed(() =>
  formatDisplayErrorMessage(monitorError.value, '加载任务运行监控失败'),
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
