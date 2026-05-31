<template>
  <div class="admin-page task-monitor-page">
    <div class="table-card admin-table-card tasks-audit-log">
      <div class="monitor-summary-chart-panel">
        <div class="monitor-summary-chart-head">
          <div class="monitor-summary-chart-copy">
            <span class="monitor-summary-chart-title">当前页状态构成</span>
            <div class="monitor-summary-legend">
              <span class="monitor-summary-legend-item">
                <span class="monitor-summary-dot is-running"></span>执行中 {{ pageStats.running }}
              </span>
              <span class="monitor-summary-legend-item">
                <span class="monitor-summary-dot is-pending"></span>排队中 {{ pageStats.pending }}
              </span>
              <span class="monitor-summary-legend-item">
                <span class="monitor-summary-dot is-failed"></span>失败 {{ pageStats.failed }}
              </span>
              <span class="monitor-summary-legend-item">
                <span class="monitor-summary-dot is-other"></span>其他 {{ summaryOtherCount }}
              </span>
            </div>
          </div>
          <div class="monitor-summary-total">
            <span class="monitor-summary-total-label">查询结果总数</span>
            <span class="monitor-summary-total-value">{{ total }}</span>
          </div>
        </div>
        <div v-if="tasks.length" class="monitor-summary-chart-host">
          <VChart class="echart-host" :option="summaryChartOption" autoresize />
        </div>
        <div v-else class="monitor-chart-empty">-</div>
      </div>

      <div class="monitor-inline-panels">
        <div class="inline-panel slow-task-panel">
          <div class="inline-panel-title">当前页慢任务 (≥60s)</div>
          <div v-if="slowTasks.length" class="slow-task-chart-host">
            <VChart
              class="echart-host"
              :option="slowTaskChartOption"
              autoresize
              @click="handleSlowTaskChartClick"
            />
          </div>
          <div v-if="slowTasks.length" class="slow-task-links">
            <button
              v-for="row in slowTasks"
              :key="row.task.task_id"
              type="button"
              class="slow-task-link"
              @click="showDetail(row.task)"
            >
              <span>{{ row.task.task_id }}</span>
              <span>{{ row.seconds }}s</span>
            </button>
          </div>
          <div v-else class="inline-panel-empty slow-task-empty">-</div>
        </div>
        <div class="inline-panel">
          <div class="inline-panel-title">当前页失败任务</div>
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

      <div class="admin-card-header">
        <div class="admin-card-title-wrapper">
          <n-icon size="18" class="admin-card-title-icon"><List /></n-icon>
          <h3 class="admin-card-title">任务监控</h3>
        </div>
        <div class="admin-card-actions">
          <n-input
            v-model:value="searchKeyword"
            placeholder="搜索用户ID或提示词..."
            clearable
            style="width: 240px"
            @keyup.enter="handleSearch"
            @clear="handleSearch"
          >
            <template #prefix>
              <n-icon><Search /></n-icon>
            </template>
          </n-input>
          <n-select
            v-model:value="filterStatus"
            :options="taskStatusOptions"
            placeholder="所有状态"
            clearable
            style="width: 140px"
          />
          <n-button secondary class="admin-icon-btn" @click="loadTasks">
            <template #icon>
              <n-icon><Refresh /></n-icon>
            </template>
          </n-button>
        </div>
      </div>
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

const summaryChartOption = computed(() => {
  return {
    animation: false,
    tooltip: {
      trigger: 'item',
      formatter: '{a}<br/>{b}: {c}',
    },
    grid: {
      top: 8,
      left: 8,
      right: 8,
      bottom: 8,
      containLabel: true,
    },
    xAxis: {
      type: 'value',
      show: false,
      max: tasks.value.length || 1,
    },
    yAxis: {
      type: 'category',
      data: ['当前页任务'],
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: {
        color: 'var(--admin-text-3)',
        fontSize: 11,
      },
    },
    series: [
      {
        name: '执行中',
        type: 'bar',
        stack: 'task-state',
        barWidth: 16,
        itemStyle: { color: '#f5b042', borderRadius: [6, 0, 0, 6] },
        label: {
          show: pageStats.value.running > 0,
          position: 'inside',
          formatter: String(pageStats.value.running),
          color: '#2b2f36',
          fontWeight: 600,
        },
        data: [pageStats.value.running],
      },
      {
        name: '排队中',
        type: 'bar',
        stack: 'task-state',
        itemStyle: { color: '#9aa7ba' },
        label: {
          show: pageStats.value.pending > 0,
          position: 'inside',
          formatter: String(pageStats.value.pending),
          color: '#2b2f36',
          fontWeight: 600,
        },
        data: [pageStats.value.pending],
      },
      {
        name: '失败',
        type: 'bar',
        stack: 'task-state',
        itemStyle: { color: '#e35d6a' },
        label: {
          show: pageStats.value.failed > 0,
          position: 'inside',
          formatter: String(pageStats.value.failed),
          color: '#2b2f36',
          fontWeight: 600,
        },
        data: [pageStats.value.failed],
      },
      {
        name: '其他',
        type: 'bar',
        stack: 'task-state',
        itemStyle: { color: '#dce3ee', borderRadius: [0, 6, 6, 0] },
        label: {
          show: summaryOtherCount.value > 0,
          position: 'inside',
          formatter: String(summaryOtherCount.value),
          color: '#2b2f36',
          fontWeight: 600,
        },
        data: [summaryOtherCount.value],
      },
    ],
  }
})

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
</script>
