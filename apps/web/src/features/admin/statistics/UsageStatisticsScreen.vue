<template>
  <div class="admin-page statistics-page">
    <div class="summary-grid">
      <AdminSummaryCard label="今日调用" :value="todayCalls" :foot="todayCallsFoot" />
      <AdminSummaryCard label="今日成功" :value="todaySuccess" :foot="todaySuccessFoot" />
      <AdminSummaryCard label="今日失败" :value="todayFailed" :foot="todayFailedFoot" />
      <AdminSummaryCard label="累计总调用" :value="totalCalls" :foot="totalCallsFoot" />
    </div>

    <div v-if="needsBackfill" class="backfill-banner">
      <div class="backfill-banner-content">
        <n-icon size="18" class="backfill-banner-icon"><WarningOutline /></n-icon>
        <span>缺失 <strong>{{ backfillStatus?.missing_days }}</strong> 天统计数据，部分历史数据不完整</span>
      </div>
      <n-button
        size="small"
        type="warning"
        :loading="backfillLoading"
        @click="handleBackfill"
      >
        立即补录
      </n-button>
    </div>

    <div class="charts-grid">
      <!-- 调用量趋势 -->
      <div class="chart-card">
        <div class="chart-header-row">
          <div class="chart-header-left">
            <h3 class="chart-title">调用量趋势</h3>
            <div class="chart-subtitle">
              <span v-if="timeRange === '7d'">最近 7 天总计</span>
              <span v-else-if="timeRange === '30d'">最近 30 天总计</span>
              <span v-else>过去一年总计</span>
              <span class="highlight"> {{ totalCallsInRange.toLocaleString() }}</span> 次
            </div>
          </div>
          <div class="chart-controls">
            <div class="range-picker">
              <button 
                v-for="r in rangeOptions" 
                :key="r.key"
                class="range-btn"
                :class="{ active: timeRange === r.key }"
                @click="timeRange = r.key"
              >
                {{ r.label }}
              </button>
            </div>
          </div>
        </div>
        <div class="chart-body area-chart-container" @mousemove="handleChartHover" @mouseleave="activeDayIndex = null">
          <div class="area-chart-clipped">
            <div class="area-chart-wrapper">
              <svg viewBox="0 0 700 200" preserveAspectRatio="none" class="area-svg">
                <defs>
                  <linearGradient v-for="(m, idx) in areaSeriesMeta" :key="'grad-' + idx" :id="'grad-' + idx" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" :stop-color="m.color" stop-opacity="0.15" />
                    <stop offset="100%" :stop-color="m.color" stop-opacity="0" />
                  </linearGradient>
                </defs>
                <path v-for="(path, index) in areaPaths" :key="'path-' + index" :d="path.d" :fill="`url(#grad-${index})`" class="area-path" />
                <path v-for="(path, index) in areaPaths" :key="'line-' + index" :d="path.lineD" :stroke="path.color" stroke-width="1.5" fill="none" stroke-linecap="round" class="area-line" />
                <line v-if="activeDayIndex !== null" :x1="activeDayIndex * (700 / Math.max(1, trendData.length - 1))" y1="0" :x2="activeDayIndex * (700 / Math.max(1, trendData.length - 1))" y2="200" stroke="rgba(255,255,255,0.1)" stroke-width="1" stroke-dasharray="4,4" />
                <template v-if="activeDayIndex !== null">
                  <circle v-for="(pt, idx) in activeDayPoints" :key="'pt-' + idx" :cx="activeDayIndex * (700 / Math.max(1, trendData.length - 1))" :cy="pt.y" r="3" :fill="pt.color" stroke="#141416" stroke-width="1.5" />
                </template>
              </svg>
            </div>
            <div class="chart-x-axis-refined-fixed">
              <template v-for="(day, i) in trendData" :key="day.date">
                <div v-if="shouldShowLabel(i)" class="axis-item-absolute" :style="{ left: (i * (100 / Math.max(1, trendData.length - 1))) + '%' }">
                  <span class="axis-date">{{ day.date }}</span>
                </div>
              </template>
            </div>
          </div>
          <div
            v-if="activeDayIndex !== null"
            class="chart-float-tooltip"
            :class="{
              'is-left-edge': activeDayIndex <= 1,
              'is-right-edge': activeDayIndex >= Math.max(0, trendData.length - 2),
            }"
            :style="{ left: (activeDayIndex * (100 / Math.max(1, trendData.length - 1))) + '%' }"
          >
            <div class="tooltip-header">{{ trendData[activeDayIndex].date }}</div>
            <div class="tooltip-list">
              <div v-for="seg in trendData[activeDayIndex].segments" :key="seg.label" class="tooltip-item">
                <span class="dot" :style="{ background: seg.color }"></span>
                <span class="label">{{ seg.label }}</span>
                <span class="val">{{ seg.value }}</span>
              </div>
              <div class="tooltip-divider"></div>
              <div class="tooltip-total"><span>总计</span><span>{{ trendData[activeDayIndex].total }}</span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 模型消耗占比 (Step 14: Channel -> Model) -->
      <div class="chart-card pie-chart-card">
        <div class="chart-header-row">
          <h3 class="chart-title">模型消耗占比</h3>
          <n-popover trigger="click" placement="bottom-end" :show-arrow="false" raw class="admin-popover-panel">
            <template #trigger>
              <n-button quaternary circle size="small">
                <template #icon><n-icon><SettingsOutline /></n-icon></template>
              </n-button>
            </template>
            <div class="price-card-popover">
              <div class="popover-body">
                <n-form size="small" :show-feedback="false" label-placement="left" label-width="150" label-align="left">
                  <n-form-item v-for="m in modelSettingCandidates" :key="m.name" :label="m.name" class="modal-style-item">
                    <n-input-number placeholder="0.00" :precision="4" :min="0" style="width: 100%" class="pill-input">
                      <template #prefix>$</template>
                    </n-input-number>
                  </n-form-item>
                </n-form>
              </div>
            </div>
          </n-popover>
        </div>
        <div class="chart-body consumption-dashboard center-mode">
          <div class="donut-section-large">
            <div class="donut-wrapper-large">
              <svg viewBox="0 0 100 100" class="donut-svg">
                <circle cx="50" cy="50" r="40" fill="transparent" stroke="rgba(255,255,255,0.03)" stroke-width="8" />
                <template v-for="(item, index) in chartSegments" :key="'ring-' + index">
                  <circle 
                    cx="50" 
                    cy="50" 
                    r="40" 
                    fill="transparent" 
                    :stroke="item.color" 
                    stroke-width="8" 
                    stroke-linecap="round" 
                    :stroke-dasharray="item.dashArray" 
                    :stroke-dashoffset="item.dashOffset" 
                    class="donut-segment-hardware"
                  />
                </template>
              </svg>
              <div class="donut-center-interactive">
                <span class="center-label">总计消耗</span>
                <span class="center-value">{{ totalCalls.toLocaleString() }}</span>
              </div>
            </div>
          </div>
          
          <!-- 新增渠道列表 legend -->
          <div class="consumption-legend">
            <div v-for="item in modelRanking" :key="item.name" class="legend-row">
              <div class="legend-left">
                <span class="legend-marker" :style="{ backgroundColor: item.color }"></span>
                <span class="legend-name">
                  {{ item.name }}
                  <span class="legend-provider">({{ item.provider }})</span>
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- 服务健康监测 (Step 1: Align with Trend Chart) -->
      <div class="chart-card health-card">
        <div class="health-header">
          <h3 class="chart-title">服务健康监测</h3>
          <div class="health-stats">
            <span class="health-label">最近 7 天</span>
            <span class="health-value">{{ healthRate }}</span>
          </div>
        </div>
        <div class="health-grid">
          <n-tooltip v-for="(cell, i) in healthGridFlat" :key="i" trigger="hover" placement="top" :show-arrow="true" class="health-tooltip">
            <template #trigger>
              <div class="health-cell" :class="{ 'is-error': cell.status === 1, 'is-warning': cell.status === 2, 'is-success': cell.status === 3, 'is-empty': cell.status === 0 }" :style="{ background: cell.status === 1 ? 'var(--admin-danger)' : cell.status === 2 ? 'var(--admin-warning)' : cell.status === 3 ? 'var(--admin-success)' : 'rgba(255, 255, 255, 0.05)' }"></div>
            </template>
            <div class="tooltip-content">
              <div class="tooltip-time">{{ getCellTimeRange(i) }}</div>
              <div class="tooltip-stats">
                <span class="stat-success"><span class="icon">✓</span> {{ cell.success }}</span>
                <span class="stat-fail"><span class="icon Discovery-failed">×</span> {{ cell.fail }}</span>
                <span class="stat-percent">({{ calculatePercent(cell) }}%)</span>
              </div>
            </div>
          </n-tooltip>
        </div>
        <div class="health-legend">
          <div class="legend-item"><span class="legend-dot is-success"></span><span>正常</span></div>
          <div class="legend-item"><span class="legend-dot is-warning"></span><span>波动</span></div>
          <div class="legend-item"><span class="legend-dot is-error"></span><span>故障</span></div>
          <div class="legend-spacer"></div>
          <div class="legend-item"><span class="legend-dot is-empty"></span><span>无数据</span></div>
        </div>
      </div>
      <!-- 第三行：延迟、错误分布与用户排行 -->
      <div class="metrics-row">
        <!-- 响应延迟 (Step 2 Redesign: Per-Model Latency) -->
        <div class="chart-card metric-mini-card latency-split-card">
          <div class="metric-header">
            <span class="metric-title">模型响应延迟 (TTFT)</span>
          </div>
          <div class="latency-list">
            <div class="latency-item" v-for="item in latencyList" :key="item.name">
              <span class="model-tag">{{ item.name }}</span>
              <div class="latency-bar-wrapper">
                <div class="latency-bar" :style="{ width: `${item.percent}%`, background: item.color }"></div>
              </div>
              <span class="latency-val">{{ item.val }}s</span>
            </div>
          </div>
        </div>

        <!-- 错误分布 (Step 4) -->
        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">错误分布</span>
          </div>
          <div class="error-tags">
            <div class="error-tag-item"><span class="tag-dot" style="background: #ef4444"></span><span>失败任务</span><span class="tag-val">{{ failureRate }}</span></div>
            <div class="error-tag-item"><span class="tag-dot" style="background: #f59e0b"></span><span>运行中任务</span><span class="tag-val">{{ runningRate }}</span></div>
            <div class="error-tag-item"><span class="tag-dot" style="background: #6366f1"></span><span>已完成任务</span><span class="tag-val">{{ successRate }}</span></div>
          </div>
        </div>

        <!-- 系统实时负载 (Migrated from MonitorScreen) -->
        <div class="chart-card metric-mini-card load-monitor-card">
          <div class="metric-header">
            <span class="metric-title">系统实时负载</span>
          </div>
          <div class="load-bars">
            <div class="load-bar-item">
              <div class="load-bar-info">
                <span>全局队列</span>
                <span class="load-count">{{ monitorQueueText }}</span>
              </div>
              <n-progress type="line" :percentage="monitorQueuePercent" :show-indicator="false" :height="6" :border-radius="3" color="var(--admin-primary)" />
            </div>
            <div class="load-bar-item">
              <div class="load-bar-info">
                <span>运行中任务</span>
                <span class="load-count">{{ monitorRunningText }}</span>
              </div>
              <n-progress type="line" :percentage="monitorRunningPercent" :show-indicator="false" :height="6" :border-radius="3" status="warning" />
            </div>
          </div>
        </div>

        <!-- 渠道负载监控 (Migrated from MonitorScreen) -->
        <div class="chart-card metric-mini-card channel-load-card">
          <div class="metric-header">
            <span class="metric-title">渠道并发负载</span>
            <span class="metric-value">{{ saturatedAccountCount }} <span class="unit">个饱和</span></span>
          </div>
          <div class="load-items-mini">
            <div class="load-item-row" v-for="item in accountLoadRows" :key="item.id">
              <span class="acc-id">#{{ item.id }}</span>
              <n-progress type="line" :percentage="item.percent" :show-indicator="false" :height="4" :color="item.color" />
              <span class="acc-num">{{ item.current }}/{{ item.max }}</span>
            </div>
          </div>
        </div>

        <!-- 用户消耗排行 (Step 1) -->
        <div class="chart-card metric-mini-card user-rank-card">
          <div class="metric-header">
            <span class="metric-title">活跃用户排行</span>
          </div>
          <div class="user-rank-list">
            <div v-for="(user, idx) in userRank" :key="user.name" class="user-rank-item">
              <span class="rank-num">{{ idx + 1 }}</span>
              <span class="rank-name">{{ user.name }}</span>
              <span class="rank-val">{{ user.value }}</span>
            </div>
          </div>
        </div>
      </div>

    </div>
    <!-- 底部：任务监控 (Migrated from MonitorScreen) -->
    <div class="chart-card admin-table-card tasks-audit-log">
      <div class="admin-card-header">
        <div class="admin-card-title-wrapper">
          <n-icon size="18" class="admin-card-title-icon"><ListOutline /></n-icon>
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
              <n-icon><SearchOutline /></n-icon>
            </template>
          </n-input>
          <n-select v-model:value="filterStatus" :options="taskStatusOptions" placeholder="所有状态" clearable style="width: 140px" />
          <n-button secondary class="admin-icon-btn" @click="loadTasks">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
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
        <n-pagination
          v-model:page="page"
          :page-count="pageCount"
          @update:page="loadTasks"
        />
      </div>
    </div>

    <!-- 详情弹窗 (Redesigned: High Density & Premium Modal) -->
    <n-modal 
      v-model:show="detailModalVisible" 
      class="premium-detail-modal"
      preset="card"
      style="width: 640px; max-width: 95vw; border-radius: 20px;"
      :bordered="false"
      :segmented="{ content: 'soft' }"
    >
      <template #header>
        <div class="premium-drawer-header">
          <div class="h-left">
            <n-icon size="22" class="h-icon" :color="getTaskStatusConfig(detailTask?.status || '').color">
              <CheckmarkCircleOutline v-if="detailTask?.status === 'completed'" />
              <CloseCircleOutline v-else-if="detailTask?.status === 'failed'" />
              <TimeOutline v-else />
            </n-icon>
            <div class="h-info">
              <span class="h-title">任务详情控制台</span>
              <span class="h-id">ID: {{ detailTask?.task_id }}</span>
            </div>
          </div>
          <div class="h-actions">
            <n-button quaternary size="small" circle @click="copyToClipboard(detailTask?.task_id || '')">
              <template #icon><n-icon><CopyOutline /></n-icon></template>
            </n-button>
          </div>
        </div>
      </template>

      <div v-if="detailTask" class="premium-detail-body">
        <!-- 状态横幅 -->
        <div class="status-banner" :class="detailTask.status">
          <div class="banner-main">
            <span class="status-label">当前状态</span>
            <span class="status-value">{{ getTaskStatusConfig(detailTask.status).text }}</span>
          </div>
          <div v-if="detailTask.status === 'failed'" class="banner-error">
             <n-icon class="err-icon"><WarningOutline /></n-icon>
             <span>{{ getErrorReason(detailTask) || '处理过程中发生未知错误' }}</span>
          </div>
          <div v-else class="banner-metrics">
            <div class="metric">
              <span class="m-label">总耗时</span>
              <span class="m-val">{{ calcDuration(detailTask) }}</span>
            </div>
            <div class="metric">
              <span class="m-label">创建于</span>
              <span class="m-val">{{ formatDateTime(detailTask.created_at).split('-')[1] }}</span>
            </div>
          </div>
        </div>

        <!-- 核心元数据卡片 -->
        <div class="detail-section">
          <div class="section-title">
            <n-icon><FingerPrintOutline /></n-icon>
            <span>身份与归属</span>
          </div>
          <div class="metadata-grid">
            <div class="meta-item">
              <span class="label">用户 ID</span>
              <div class="val-group">
                <span class="value">{{ detailTask.user_id }}</span>
                <n-button quaternary circle size="tiny" @click="copyToClipboard(String(detailTask.user_id))">
                  <template #icon><n-icon><CopyOutline /></n-icon></template>
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

        <!-- 提示词与内容 -->
        <div class="detail-section">
          <div class="section-title">
            <n-icon><ChatbubbleEllipsesOutline /></n-icon>
            <span>任务指令 (Prompt)</span>
            <n-button quaternary size="tiny" class="title-action" @click="copyToClipboard(detailTask.prompt || '')">
              复制全文
            </n-button>
          </div>
          <div class="prompt-card">
            <div class="prompt-content">{{ detailTask.prompt || '无提示词' }}</div>
          </div>
        </div>

        <!-- 产出物预览 -->
        <div v-if="resultImageUrl" class="detail-section">
          <div class="section-title">
            <n-icon><ImageOutline /></n-icon>
            <span>生成结果</span>
          </div>
          <div class="result-image-box">
            <n-image :src="resultImageUrl" width="100%" object-fit="contain" class="standard-image" />
            <div class="image-overlay">
              <n-button tertiary size="small" @click="openResultImage(resultImageUrl)">
                 在新窗口打开
              </n-button>
            </div>
          </div>
        </div>

        <!-- API 调用摘要 -->
        <div class="detail-section">
          <div class="section-title">
            <n-icon><CodeWorkingOutline /></n-icon>
            <span>API 响应摘要</span>
            <n-button quaternary size="tiny" class="title-action" @click="copyToClipboard(resultSummaryText)">
              复制 JSON
            </n-button>
          </div>
          <div class="json-card">
            <pre>{{ resultSummaryText }}</pre>
          </div>
          
          <div v-if="resultDebugText" class="debug-wrapper">
            <div class="debug-header" @click="showDebugResult = !showDebugResult">
               <div class="header-left">
                 <n-icon><TerminalOutline /></n-icon>
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

        <div class="premium-drawer-footer">
           &copy; Feng-AI Operations Audit System
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { 
  NDataTable, NProgress, NTooltip, NButton, NIcon, NPopover, NInputNumber, 
  NForm, NFormItem, NModal, NSelect, NPagination, NImage, NInput, 
  NCollapseTransition, useMessage 
} from 'naive-ui'
import { 
  SettingsOutline, ListOutline, RefreshOutline, SearchOutline, 
  ChatbubbleEllipsesOutline, ImageOutline, WarningOutline, TerminalOutline,
  CopyOutline, CheckmarkCircleOutline, CloseCircleOutline, TimeOutline,
  FingerPrintOutline,
  CodeWorkingOutline
} from '@vicons/ionicons5'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'
import {
  taskStatusOptions,
  getTaskStatusConfig,
} from '../shared/monitorShared'
import { useTaskResultDetail } from './useTaskResultDetail'
import { useTaskMonitor } from './useTaskMonitor'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'
import './UsageStatisticsScreen.css'

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
  loading,
  page,
  pageCount,
  filterStatus,
  searchKeyword,
  monitorData,
  columns,
  loadTasks,
  handleSearch,
} = useTaskMonitor({ onShowDetail: showDetail })

const {
  adminStatistics,
  configuredAccounts,
  configuredChannels,
  timeRange,
  rangeOptions,
  loadStatistics,
  backfillStatus,
  needsBackfill,
  backfillLoading,
  triggerBackfill,
} = useUsageStatisticsData()

const {
  areaSeriesMeta,
  trendData,
  totalCallsInRange,
  activeDayIndex,
  handleChartHover,
  areaPaths,
  activeDayPoints,
  todayCalls,
  todaySuccess,
  todayFailed,
  totalCalls,
  todayCallsFoot,
  todaySuccessFoot,
  todayFailedFoot,
  totalCallsFoot,
  modelRanking,
  modelSettingCandidates,
  chartSegments,
  healthGridFlat,
  userRank,
  latencyList,
  healthRate,
  failureRate,
  runningRate,
  successRate,
  monitorQueueText,
  monitorRunningText,
  monitorQueuePercent,
  monitorRunningPercent,
  accountLoadRows,
  saturatedAccountCount,
  calculatePercent,
  getCellTimeRange,
  shouldShowLabel,
} = useUsageStatisticsCharts({
  adminStatistics,
  configuredAccounts,
  configuredChannels,
  timeRange,
  monitorData,
})

async function handleBackfill() {
  const result = await triggerBackfill()
  if (result.ok) {
    message.success('统计补录完成')
  } else {
    message.error(result.message || '统计补录失败')
  }
}

watch(filterStatus, () => {
  page.value = 1
  void loadTasks()
})
watch(timeRange, () => {
  void loadStatistics()
})

onMounted(() => {
  void loadTasks()
  void loadStatistics()
})

</script>
