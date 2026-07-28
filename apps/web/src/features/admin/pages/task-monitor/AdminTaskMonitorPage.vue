<template>
  <div :class="adminDashboardRootClass">
    <Message v-if="taskMonitorSettingsError" severity="warn" :closable="false">
      {{ taskMonitorSettingsErrorMessage }}
    </Message>

    <Message v-if="isMonitorError" severity="error" :closable="false">
      {{ monitorErrorMessage }}
    </Message>

    <Message v-if="isMonitorStale" severity="warn" :closable="false">
      <div class="flex flex-wrap items-center gap-2">
        <span>任务监控连接不稳定，当前显示上次成功数据。</span>
        <Button
          text
          size="small"
          :class="adminTextButtonClass"
          label="重试连接"
          @click="retryMonitorSnapshot"
        />
      </div>
    </Message>

    <template v-if="!isMonitorError">
      <div :class="adminSummaryGridClass">
        <AdminSummaryCard label="执行中" :value="liveRunningCount" :footer="runningFoot" />
        <AdminSummaryCard label="队列等待" :value="liveQueueTotal" :footer="queueFoot" />
        <AdminSummaryCard label="失败" :value="liveFailedCount" :footer="failedFoot" />
        <AdminSummaryCard
          label="账号并发"
          :value="`${peakAccountLoadPercent}%`"
          :footer="accountLoadFoot"
        />
      </div>

      <div :class="adminChartGridClass">
        <AdminChartCard
          :title="taskCards.executionPressure.title"
          span-class="xl:col-span-12"
          chart-size="monitor"
        >
          <template #actions>
            <Tag
              :value="`${activeExecutionPressurePoints.length} 类`"
              severity="secondary"
              rounded
            />
          </template>
          <Chart
            type="bubble"
            :class="adminChartCanvasClass"
            :data="executionPressureChartData"
            :options="executionPressureChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard
          :title="taskCards.taskDistribution.title"
          span-class="xl:col-span-6"
          chart-size="monitor"
        >
          <template #actions>
            <Tag :value="`${taskDistributionRows.length} 类`" severity="secondary" rounded />
          </template>
          <Chart
            type="radar"
            :class="adminChartCanvasClass"
            :data="taskDistributionChartData"
            :options="taskDistributionChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard
          :title="taskCards.accountConcurrency.title"
          span-class="xl:col-span-6"
          chart-size="monitor"
        >
          <template #actions>
            <Tag :value="`${accountLoadRows.length} 账号`" severity="secondary" rounded />
          </template>
          <Chart
            type="bar"
            :class="adminChartCanvasClass"
            :data="accountConcurrencyChartData"
            :options="accountConcurrencyChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>
      </div>
    </template>

    <Message v-if="isTaskListError" severity="error" :closable="false">
      {{ taskListErrorMessage }}
    </Message>

    <AdminTablePanel v-else>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch
              v-model="searchKeyword"
              input-label="搜索任务"
              @enter="handleTaskSearch"
              @clear="clearSearch"
            />
          </template>

          <template #metrics>
            <Tag :value="`${total} 任务`" severity="secondary" rounded />
            <Tag
              v-if="taskStats.pending > 0"
              :value="`${taskStats.pending} 等待中`"
              severity="secondary"
              rounded
            />
            <Tag
              v-if="taskStats.running > 0"
              :value="`${taskStats.running} 执行中`"
              severity="warn"
              rounded
            />
            <Tag
              v-if="taskStats.failed > 0"
              :value="`${taskStats.failed} 失败`"
              severity="danger"
              rounded
            />
            <Tag
              v-if="taskStats.completed > 0"
              :value="`${taskStats.completed} 已完成`"
              severity="success"
              rounded
            />
            <Tag
              v-if="taskStats.cancelled > 0"
              :value="`${taskStats.cancelled} 已取消`"
              severity="secondary"
              rounded
            />
            <Tag v-if="selectedTasks.length > 0" :value="`已选 ${selectedTasks.length}`" rounded />
          </template>

          <template #filters>
            <Button
              text
              :class="adminToolbarIconButtonClass"
              :disabled="!canDeleteSelectedTasks"
              :loading="isDeletingSelectedTasks"
              aria-label="删除"
              title="删除"
              @click="requestDeleteSelectedTasks"
            >
              <template #icon>
                <Trash class="size-4 text-admin-danger" />
              </template>
            </Button>
            <AdminFilterPopover
              :active="hasTaskFilter"
              filter-label="筛选任务"
              @reset="resetTaskFilters"
            >
              <label
                class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>状态</span>
                <Select
                  v-model="filterStatus"
                  :options="taskStatusSelectOptions"
                  option-label="label"
                  option-value="value"
                  size="small"
                  fluid
                />
              </label>

              <label
                class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>账号 ID</span>
                <InputNumber
                  v-model="accountIdFilter"
                  :min="1"
                  :use-grouping="false"
                  placeholder="Account ID"
                  size="small"
                  fluid
                />
              </label>
            </AdminFilterPopover>
          </template>

          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新任务列表"
              :disabled="isDeletingSelectedTasks"
              @click="loadTasks"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
          </template>

          <template #pager>
            <AdminPagerControls
              :current-page="currentPage"
              :total-pages="totalPages"
              :prev-disabled="!canPrevious"
              :next-disabled="!canNext"
              @prev="previous"
              @next="next"
            />
          </template>
        </AdminResourceToolbar>
      </template>

      <DataTableFrame
        v-model:selection="selectedTasks"
        :value="tasks"
        :loading="loading"
        data-key="task_id"
        scrollable
        :table-style="{ minWidth: '1536px' }"
      >
        <template #empty>
          <div class="py-7 text-center text-admin-body text-admin-text-3">暂无任务</div>
        </template>

        <Column selection-mode="multiple" style="width: 3rem" />

        <Column header="创建时间" :style="{ width: '156px' }">
          <template #body="{ data }">
            <span
              class="whitespace-nowrap font-mono text-admin-meta font-semibold text-admin-text-2-strong"
            >
              {{ formatTaskCreatedAt(data) }}
            </span>
          </template>
        </Column>

        <Column header="结果" :style="{ width: '104px', minWidth: '104px' }">
          <template #body="{ data }">
            <div class="flex h-12 w-12 shrink-0 items-center justify-start">
              <PrimeImage
                v-if="resultPreviewEnabled && getTaskResultImageUrl(data)"
                :src="getTaskResultImageUrl(data)"
                preview
                image-class="block h-12 w-12 min-w-12 shrink-0 overflow-hidden rounded-[10px] border border-admin-border-soft bg-admin-surface-muted object-cover"
                :image-style="{
                  width: '48px',
                  height: '48px',
                  minWidth: '48px',
                  objectFit: 'cover',
                }"
              />
              <span v-else class="text-admin-meta text-admin-text-3">-</span>
            </div>
          </template>
        </Column>

        <Column header="用户" :style="{ width: '140px' }">
          <template #body="{ data }">
            <span class="block max-w-[120px] truncate text-admin-meta font-bold text-admin-text-1">
              {{ getTaskUserIdentifier(data) }}
            </span>
          </template>
        </Column>

        <Column header="提示词" :style="{ width: '280px' }">
          <template #body="{ data }">
            <span
              class="block max-w-[252px] truncate text-admin-meta text-admin-text-2-strong"
              :title="data.prompt || '-'"
            >
              {{ data.prompt || '-' }}
            </span>
          </template>
        </Column>

        <Column header="账号" :style="{ width: '88px' }">
          <template #body="{ data }">
            <span class="text-admin-meta text-admin-text-3">{{
              getTaskAccountLabel(data) || '-'
            }}</span>
          </template>
        </Column>

        <Column header="Endpoint" :style="{ width: '184px' }">
          <template #body="{ data }">
            <div class="flex min-w-0 flex-col gap-0.5">
              <span
                class="block max-w-[160px] truncate text-admin-meta font-semibold text-admin-text-2-strong"
                :title="getTaskEndpointLabel(data) || '-'"
              >
                {{ getTaskEndpointLabel(data) || '-' }}
              </span>
              <span class="block max-w-[160px] truncate text-admin-micro text-admin-text-3">
                {{ getTaskAPITypeLabel(data) || '-' }}
              </span>
            </div>
          </template>
        </Column>

        <Column header="模型" :style="{ width: '156px' }">
          <template #body="{ data }">
            <span
              class="block max-w-[132px] truncate text-admin-meta font-semibold text-admin-text-2-strong"
              :title="getTaskModelLabel(data) || '-'"
            >
              {{ getTaskModelLabel(data) || '-' }}
            </span>
          </template>
        </Column>

        <Column header="优先级" :style="{ width: '88px' }">
          <template #body="{ data }">
            <Tag :value="getTaskPriorityLabel(data)" severity="secondary" rounded />
          </template>
        </Column>

        <Column header="API调用" :style="{ width: '144px' }">
          <template #body="{ data }">
            <span v-if="getTaskApiCall(data)" class="inline-flex max-w-32 items-center gap-1.5">
              <Tag :value="getTaskApiCall(data)?.platform" severity="info" rounded />
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="状态" :style="{ width: '104px' }">
          <template #body="{ data }">
            <Tag
              :value="getTaskStatusDefinition(data.status).text"
              :severity="getTaskStatusDefinition(data.status).severity"
              rounded
              class="min-w-16 justify-center"
            />
          </template>
        </Column>

        <Column header="耗时" :style="{ width: '88px' }">
          <template #body="{ data }">
            <span v-if="getTaskDuration(data)" :class="getTaskDurationClass(data)">
              {{ getTaskDuration(data)?.seconds }}s
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="错误诊断" :style="{ width: '240px' }">
          <template #body="{ data }">
            <span
              v-if="getTaskSanitizedError(data)"
              class="block max-w-[212px] truncate text-admin-meta text-admin-danger"
            >
              {{ getTaskSanitizedError(data) }}
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="操作" :style="{ width: '72px' }">
          <template #body="{ data }">
            <Button
              text
              rounded
              size="small"
              severity="secondary"
              aria-label="查看任务详情"
              @click="showDetail(data)"
            >
              <template #icon>
                <Eye class="size-4 text-admin-text-2" />
              </template>
            </Button>
          </template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>

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
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Chart from 'primevue/chart'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import PrimeImage from 'primevue/image'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { Eye, Refresh, Trash } from '@iconoir/vue'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { useAdminTaskDeletionFeedback } from '../../task-monitor/useAdminTaskMonitorFeedback'
import AdminTaskResultDialog from '../../task-monitor/AdminTaskResultDialog.vue'
import AdminChartCard from '../../shared/components/AdminChartCard.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminCards } from '../../shared/lib/adminCardMeta'
import { adminChartPlugins } from '../../shared/lib/adminChart'
import {
  adminChartCanvasClass,
  adminChartGridClass,
  adminDashboardRootClass,
  adminSummaryGridClass,
  adminTextButtonClass,
  adminToolbarIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import {
  formatTaskCreatedAt,
  getTaskAccountLabel,
  getTaskAPITypeLabel,
  getTaskApiCall,
  getTaskDuration,
  getTaskEndpointLabel,
  getTaskModelLabel,
  getTaskPriorityLabel,
  getTaskResultImageUrl,
  getTaskSanitizedError,
  getTaskUserIdentifier,
} from '../../task-monitor/taskMonitorColumns'
import {
  adminTaskStatuses,
  getTaskStatusDefinition,
  taskStatusSelectOptions,
} from '../../task-monitor/taskMonitorStatus'
import { useTaskMonitorCharts } from '../../task-monitor/useTaskMonitorCharts'
import { type AdminTaskRow, useAdminTaskList } from '../../task-monitor/useAdminTaskList'
import { useAdminTaskDetailResource } from '../../task-monitor/taskMonitorResource'
import { useAdminTaskSelectionDelete } from '../../task-monitor/useAdminTaskSelectionDelete'
import { useAdminTaskRuntimeSnapshot } from '../../task-monitor/useAdminTaskRuntimeSnapshot'

const taskDeletionFeedback = useAdminTaskDeletionFeedback()
const adminConfirm = useAdminConfirm()
const taskCards = adminCards.taskMonitor
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
    ? formatDisplayErrorMessage(detailQuery.error.value, '加载任务详情失败')
    : '',
)

const {
  tasks,
  total,
  loading,
  isError: isTaskListError,
  error: taskListError,
  pagination,
  resultPreviewEnabled,
  taskMonitorSettingsError,
  taskStats,
  filterStatus,
  accountIdFilter,
  searchKeyword,
  loadTasks,
  handleSearch: applyTaskSearch,
} = useAdminTaskList()
const { currentPage, totalPages, canPrevious, canNext, previous, next } = pagination
const {
  monitorData,
  monitorStats,
  monitorStreamStatus,
  isStale: isMonitorStale,
  isError: isMonitorError,
  error: monitorError,
  retry: retryMonitorSnapshot,
} = useAdminTaskRuntimeSnapshot()

const selectedTasks = ref<AdminTaskRow[]>([])
const { canDeleteSelectedTasks, isDeletingSelectedTasks, requestDeleteSelectedTasks } =
  useAdminTaskSelectionDelete({
    selectedTasks,
    confirm: adminConfirm,
    feedback: taskDeletionFeedback,
  })
const hasTaskFilter = computed(() => Boolean(filterStatus.value || accountIdFilter.value))
const taskListErrorMessage = computed(() =>
  formatDisplayErrorMessage(taskListError.value, '加载任务列表失败'),
)
const monitorErrorMessage = computed(() =>
  formatDisplayErrorMessage(monitorError.value, '加载任务运行监控失败'),
)
const taskMonitorSettingsErrorMessage = computed(() =>
  formatDisplayErrorMessage(
    taskMonitorSettingsError.value,
    '加载任务监控设置失败，当前使用默认展示配置',
  ),
)

function handleTaskSearch() {
  selectedTasks.value = []
  applyTaskSearch()
}

function resetTaskFilters() {
  filterStatus.value = null
  accountIdFilter.value = null
}

watch([currentPage, filterStatus, accountIdFilter], () => {
  selectedTasks.value = []
})

const currentPageCount = computed(() => tasks.value.length)
const {
  taskStatusSnapshot,
  statusTotalCount,
  accountLoadRows,
  accountLoadCurrent,
  accountLoadCapacity,
  peakAccountLoadPercent,
  taskDistributionRows,
  activeExecutionPressurePoints,
  accountConcurrencyChartData,
  accountConcurrencyChartOptions,
  taskDistributionChartData,
  taskDistributionChartOptions,
  executionPressureChartData,
  executionPressureChartOptions,
} = useTaskMonitorCharts({ monitorData, monitorStats })
const liveRunningCount = computed(() => monitorData.value?.running_attempts ?? 0)
const liveFailedCount = computed(() => taskStatusSnapshot.value[adminTaskStatuses.failed])
const globalQueueLength = computed(() => monitorData.value?.global_queue_len || 0)
const waitQueueLength = computed(() => monitorData.value?.wait_queue_len || 0)
const liveQueueTotal = computed(() => globalQueueLength.value + waitQueueLength.value)
const runningFoot = computed(() => formatMonitorStreamStatus(monitorStreamStatus.value))
const queueFoot = computed(() =>
  waitQueueLength.value > 0
    ? `全局 ${globalQueueLength.value} / 等待 ${waitQueueLength.value}`
    : `全局 ${globalQueueLength.value}`,
)
const failedFoot = computed(
  () => `状态占比 ${formatPercent(liveFailedCount.value, statusTotalCount.value)}`,
)
const accountLoadFoot = computed(() =>
  accountLoadRows.value.length > 0
    ? `${accountLoadCurrent.value}/${accountLoadCapacity.value} 并发占用`
    : `当前页 ${currentPageCount.value} / 查询 ${total.value}`,
)

function clearSearch() {
  searchKeyword.value = ''
  handleTaskSearch()
}

function showDetail(task: AdminTaskRow) {
  selectedDetailTaskID.value = task.task_id
  detailModalVisible.value = true
}

function retryTaskDetail() {
  void detailQuery.refetch()
}

function getTaskDurationClass(task: Parameters<typeof getTaskDuration>[0]) {
  const tone = getTaskDuration(task)?.tone
  return [
    'font-mono text-admin-meta font-bold',
    tone === 'success' && 'text-admin-success',
    tone === 'warning' && 'text-admin-warning',
    tone === 'danger' && 'text-admin-danger',
  ]
}

function formatPercent(value: number, totalValue: number) {
  if (totalValue <= 0) return '0.0%'
  return `${((value / totalValue) * 100).toFixed(1)}%`
}

function formatMonitorStreamStatus(status: string) {
  const textByStatus: Record<string, string> = {
    idle: '实时流待命',
    unsupported: '实时流不可用',
    connecting: '实时流连接中',
    connected: '实时流已连接',
    degraded: '实时流降级',
  }
  return textByStatus[status] || '实时流未知'
}
</script>
