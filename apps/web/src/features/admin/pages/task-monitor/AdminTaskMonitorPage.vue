<template>
  <div class="admin-page task-monitor-page">
    <div class="summary-grid task-status-grid">
      <AdminSummaryCard label="执行中" :value="pageStats.running" :foot="runningFoot" />
      <AdminSummaryCard label="排队中" :value="pageStats.pending" :foot="pendingFoot" />
      <AdminSummaryCard label="失败" :value="pageStats.failed" :foot="failedFoot" />
      <AdminSummaryCard label="其他" :value="summaryOtherCount" :foot="otherFoot" />
    </div>

    <div class="monitor-insight-grid">
      <div class="table-card monitor-insight-card slow-task-panel">
        <div class="inline-panel-head">
          <div class="inline-panel-title">慢任务 Top</div>
          <span class="inline-panel-stat">{{ slowTasks.length }} 条</span>
        </div>
        <div class="monitor-insight-body">
          <div v-if="slowTasks.length" class="slow-task-chart-host">
            <VChart
              class="echart-host"
              :option="slowTaskChartOption"
              autoresize
              @click="handleSlowTaskChartClick"
            />
          </div>
          <div v-else class="inline-panel-empty slow-task-empty">-</div>
        </div>
      </div>

      <div class="table-card monitor-insight-card active-task-panel">
        <div class="inline-panel-head">
          <div class="inline-panel-title">活跃任务龄</div>
          <span class="inline-panel-stat">{{ activeTasks.length }} 条</span>
        </div>
        <div class="monitor-insight-body">
          <div v-if="activeTasks.length" class="inline-panel-list">
            <button
              v-for="row in activeTasks"
              :key="row.task.task_id"
              type="button"
              class="inline-task-item"
              @click="showDetail(row.task)"
            >
              <span class="task-id">{{ row.task.task_id }}</span>
              <span class="task-meta">{{ getTaskStatusConfig(row.task.status).text }}</span>
              <span class="task-age">{{ formatSeconds(row.seconds) }}</span>
            </button>
          </div>
          <div v-else class="inline-panel-empty">-</div>
        </div>
      </div>

      <div class="table-card monitor-insight-card failure-task-panel">
        <div class="inline-panel-head">
          <div class="inline-panel-title">失败样本</div>
          <span class="inline-panel-stat">{{ failedTasks.length }} 条</span>
        </div>
        <div class="monitor-insight-body">
          <div v-if="failedTasks.length" class="inline-panel-list">
            <button
              v-for="task in failedTasks.slice(0, 5)"
              :key="task.task_id"
              type="button"
              class="inline-task-item"
              @click="showDetail(task)"
            >
              <span class="task-id">{{ task.task_id }}</span>
              <span class="task-meta">{{ getErrorReason(task) || '-' }}</span>
            </button>
          </div>
          <div v-else class="inline-panel-empty">-</div>
        </div>
      </div>
    </div>

    <div class="table-card admin-table-card tasks-list-card">
      <AdminCardHeader title="任务列表" size="small" variant="table">
        <template #prefix>
          <n-icon size="18" class="tasks-list-title-icon"><List /></n-icon>
        </template>
        <template #extra>
          <div class="tasks-list-actions">
            <n-input
              v-model:value="searchKeyword"
              class="task-search-control"
              placeholder="搜索用户ID或提示词..."
              clearable
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            >
              <template #prefix>
                <n-icon><Search /></n-icon>
              </template>
            </n-input>
            <n-select
              v-model:value="filterStatus"
              class="task-status-control"
              :options="taskStatusOptions"
              placeholder="所有状态"
              clearable
            />
            <n-button secondary class="admin-icon-btn" aria-label="刷新任务列表" @click="loadTasks">
              <template #icon>
                <n-icon><Refresh /></n-icon>
              </template>
            </n-button>
          </div>
        </template>
      </AdminCardHeader>
      <n-data-table
        :columns="columns"
        :data="tasks"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        size="small"
        class="admin-table-dense"
        :scroll-x="1000"
      />
      <div class="pagination-wrapper">
        <n-pagination v-model:page="page" :page-count="pageCount" @update:page="loadTasks" />
      </div>
    </div>

    <n-modal
      v-model:show="detailModalVisible"
      class="premium-detail-modal"
      preset="card"
      style="width: 640px; max-width: 95vw; border-radius: 20px"
      :bordered="false"
      :segmented="{ content: 'soft' }"
    >
      <template #header>
        <div class="premium-drawer-header">
          <div class="h-left">
            <n-icon
              size="22"
              class="h-icon"
              :color="getTaskStatusConfig(detailTask?.status || '').color"
            >
              <CheckCircle v-if="detailTask?.status === 'completed'" />
              <XmarkCircle v-else-if="detailTask?.status === 'failed'" />
              <Clock v-else />
            </n-icon>
            <div class="h-info">
              <span class="h-title">任务详情控制台</span>
              <span class="h-id">ID: {{ detailTask?.task_id }}</span>
            </div>
          </div>
          <div class="h-actions">
            <n-button
              quaternary
              size="small"
              circle
              @click="copyToClipboard(detailTask?.task_id || '')"
            >
              <template #icon>
                <n-icon><Copy /></n-icon>
              </template>
            </n-button>
          </div>
        </div>
      </template>

      <div v-if="detailTask" class="premium-detail-body">
        <div class="status-banner" :class="detailTask.status">
          <div class="banner-main">
            <span class="status-label">当前状态</span>
            <span class="status-value">{{ getTaskStatusConfig(detailTask.status).text }}</span>
          </div>
          <div v-if="detailTask.status === 'failed'" class="banner-error">
            <n-icon class="err-icon"><WarningTriangle /></n-icon>
            <span>{{ getErrorReason(detailTask) || '处理过程中发生未知错误' }}</span>
          </div>
          <div v-else class="banner-metrics">
            <div class="detail-metric">
              <span class="m-label">总耗时</span>
              <span class="m-val">{{ calcDuration(detailTask) }}</span>
            </div>
            <div class="detail-metric">
              <span class="m-label">创建于</span>
              <span class="m-val">{{ formatDateTime(detailTask.created_at).split('-')[1] }}</span>
            </div>
          </div>
        </div>

        <div class="detail-section">
          <div class="section-title">
            <n-icon><Fingerprint /></n-icon>
            <span>身份与归属</span>
          </div>
          <div class="metadata-grid">
            <div class="meta-item">
              <span class="label">用户 ID</span>
              <div class="val-group">
                <span class="value">{{ detailTask.user_id }}</span>
                <n-button
                  quaternary
                  circle
                  size="tiny"
                  @click="copyToClipboard(String(detailTask.user_id))"
                >
                  <template #icon>
                    <n-icon><Copy /></n-icon>
                  </template>
                </n-button>
              </div>
            </div>
            <div class="meta-item">
              <span class="label">账号 ID</span>
              <span class="value">{{ detailTask.account_id || 'N/A' }}</span>
            </div>
            <div class="meta-item">
              <span class="label">创建时间</span>
              <span class="value">{{ formatDateTime(detailTask.created_at) }}</span>
            </div>
            <div class="meta-item">
              <span class="label">完成时间</span>
              <span class="value">{{ formatDateTime(detailTask.completed_at) }}</span>
            </div>
          </div>
        </div>

        <div class="detail-section">
          <div class="section-title">
            <n-icon><ChatBubble /></n-icon>
            <span>任务指令 (Prompt)</span>
            <n-button
              quaternary
              size="tiny"
              class="title-action"
              @click="copyToClipboard(detailTask.prompt || '')"
            >
              复制全文
            </n-button>
          </div>
          <div class="prompt-card">
            <div class="prompt-content">{{ detailTask.prompt || '无提示词' }}</div>
          </div>
        </div>

        <div v-if="resultImageUrl" class="detail-section">
          <div class="section-title">
            <n-icon><MediaImage /></n-icon>
            <span>生成结果</span>
          </div>
          <div class="result-image-box">
            <n-image
              :src="resultImageUrl"
              width="100%"
              object-fit="contain"
              class="standard-image"
            />
            <div class="image-overlay">
              <n-button tertiary size="small" @click="openResultImage(resultImageUrl)">
                在新窗口打开
              </n-button>
            </div>
          </div>
        </div>

        <div class="detail-section">
          <div class="section-title">
            <n-icon><CodeBrackets /></n-icon>
            <span>API 响应摘要</span>
            <n-button
              quaternary
              size="tiny"
              class="title-action"
              @click="copyToClipboard(resultSummaryText)"
            >
              复制 JSON
            </n-button>
          </div>
          <div class="json-card">
            <pre>{{ resultSummaryText }}</pre>
          </div>

          <div v-if="resultDebugText" class="debug-wrapper">
            <div class="debug-header" @click="showDebugResult = !showDebugResult">
              <div class="header-left">
                <n-icon><Terminal /></n-icon>
                <span>原始调试详情 (脱敏)</span>
              </div>
              <n-button text size="tiny">{{ showDebugResult ? '隐藏' : '展开' }}</n-button>
            </div>
            <n-collapse-transition :show="showDebugResult">
              <div class="json-card debug-mode">
                <pre>{{ resultDebugText }}</pre>
              </div>
            </n-collapse-transition>
          </div>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import {
  NButton,
  NCollapseTransition,
  NDataTable,
  NIcon,
  NImage,
  NInput,
  NModal,
  NPagination,
  NSelect,
  useMessage,
} from 'naive-ui'
import {
  ChatBubble,
  CheckCircle,
  Clock,
  CodeBrackets,
  Copy,
  Fingerprint,
  List,
  MediaImage,
  Refresh,
  Search,
  Terminal,
  WarningTriangle,
  XmarkCircle,
} from '@iconoir/vue'
import type { ECElementEvent } from 'echarts/core'
import { taskStatusOptions, getTaskStatusConfig } from '../../shared/lib/monitorShared'
import AdminCardHeader from '../../shared/components/AdminCardHeader.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import { VChart } from '../../shared/components/AdminEChart'
import { useTaskResultDetail } from './useTaskResultDetail'
import { useTaskMonitor } from './useTaskMonitor'
import './AdminTaskMonitorPage.css'

const message = useMessage()

const {
  detailModalVisible,
  detailTask,
  showDebugResult,
  resultImageUrl,
  resultSummaryText,
  resultDebugText,
  openResultImage,
  copyToClipboard,
  formatDateTime,
  showDetail,
  getErrorReason,
  calcDuration,
} = useTaskResultDetail({ message })

const {
  tasks,
  total,
  loading,
  page,
  pageCount,
  pageStats,
  failedTasks,
  activeTasks,
  slowTasks,
  filterStatus,
  searchKeyword,
  columns,
  loadTasks,
  handleSearch,
} = useTaskMonitor({ onShowDetail: showDetail })

const summaryOtherCount = computed(() =>
  Math.max(
    0,
    tasks.value.length - pageStats.value.running - pageStats.value.pending - pageStats.value.failed,
  ),
)

const longestRunning = computed(() =>
  activeTasks.value.find((row) => row.task.status === 'running'),
)
const longestPending = computed(() =>
  activeTasks.value.find((row) => row.task.status === 'pending'),
)
const currentPageCount = computed(() => tasks.value.length)
const runningFoot = computed(() =>
  longestRunning.value ? `最长 ${formatSeconds(longestRunning.value.seconds)}` : '当前页无执行任务',
)
const pendingFoot = computed(() =>
  longestPending.value ? `最长 ${formatSeconds(longestPending.value.seconds)}` : '当前页无排队任务',
)
const failedFoot = computed(
  () => `当前页失败率 ${formatPercent(pageStats.value.failed, currentPageCount.value)}`,
)
const otherFoot = computed(() => `当前页 ${currentPageCount.value} / 查询 ${total.value}`)

const slowTaskChartOption = computed(() => ({
  animation: false,
  tooltip: {
    trigger: 'item',
    formatter: '{b}<br/>耗时: {c}s',
  },
  grid: {
    top: 6,
    left: 8,
    right: 18,
    bottom: 8,
    containLabel: true,
  },
  xAxis: {
    type: 'value',
    axisLine: { show: false },
    axisTick: { show: false },
    splitLine: { lineStyle: { color: '#e7edf5' } },
    axisLabel: {
      color: 'var(--admin-text-3)',
      fontSize: 11,
    },
  },
  yAxis: {
    type: 'category',
    data: slowTasks.value.map((row) => row.task.task_id),
    axisTick: { show: false },
    axisLine: { show: false },
    axisLabel: {
      color: 'var(--admin-text-2)',
      fontSize: 11,
      width: 140,
      overflow: 'truncate',
    },
  },
  series: [
    {
      name: '耗时',
      type: 'bar',
      data: slowTasks.value.map((row) => row.seconds),
      barWidth: 12,
      itemStyle: {
        color: '#7e9eff',
        borderRadius: [0, 5, 5, 0],
      },
      emphasis: {
        itemStyle: {
          color: '#5e84ff',
        },
      },
    },
  ],
}))

function handleSlowTaskChartClick(event: ECElementEvent) {
  if (event.componentType !== 'series' || typeof event.dataIndex !== 'number') return
  const row = slowTasks.value[event.dataIndex]
  if (!row) return
  showDetail(row.task)
}

function formatSeconds(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  if (minutes < 60) return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
  const hours = Math.floor(minutes / 60)
  const minuteRest = minutes % 60
  return minuteRest === 0 ? `${hours}h` : `${hours}h ${minuteRest}m`
}

function formatPercent(value: number, totalValue: number) {
  if (totalValue <= 0) return '0.0%'
  return `${((value / totalValue) * 100).toFixed(1)}%`
}
</script>
