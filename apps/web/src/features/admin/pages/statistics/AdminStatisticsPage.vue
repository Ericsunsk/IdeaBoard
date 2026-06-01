<template>
  <div class="admin-page statistics-page">
    <div class="summary-grid">
      <AdminSummaryCard label="今日调用" :value="todayCalls" :foot="todayCallsFoot" />
      <AdminSummaryCard label="今日失败" :value="todayFailed" :foot="todayFailedFoot" />
      <AdminSummaryCard label="今日成功率" :value="todaySuccessRate" :foot="todaySuccessRateFoot" />
      <AdminSummaryCard label="队列压力" :value="queuePressure" :foot="queuePressureFoot" />
    </div>

    <div class="charts-grid">
      <div class="chart-card trend-card">
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
        <div class="chart-body area-chart-container">
          <VChart class="echart-host trend-echart" :option="trendChartOption" autoresize />
        </div>
      </div>

      <div class="chart-card risk-chart-card">
        <div class="chart-header-row">
          <h3 class="chart-title">模型风险矩阵</h3>
        </div>
        <div class="chart-body risk-dashboard">
          <VChart class="echart-host risk-echart" :option="modelRiskChartOption" autoresize />
        </div>
      </div>
      <div class="chart-card quality-card">
        <div class="chart-header-row">
          <h3 class="chart-title">服务质量趋势</h3>
        </div>
        <div class="chart-body quality-chart-container">
          <VChart
            class="echart-host quality-trend-echart"
            :option="qualityTrendChartOption"
            autoresize
          />
        </div>
      </div>

      <div class="metrics-row">
        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">慢模型 P95</span>
          </div>
          <div class="chart-body metric-chart-container">
            <VChart
              class="echart-host mini-bar-echart"
              :option="slowModelsChartOption"
              autoresize
            />
          </div>
        </div>

        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">渠道并发负载</span>
            <span class="metric-value"
              >{{ peakAccountLoadPercent }}<span class="unit">% 峰值</span></span
            >
          </div>
          <div class="chart-body metric-chart-container">
            <VChart
              class="echart-host capacity-bar-echart"
              :option="accountLoadChartOption"
              autoresize
            />
          </div>
        </div>

        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">异常用户排行</span>
          </div>
          <div class="chart-body metric-chart-container">
            <VChart class="echart-host mini-bar-echart" :option="userRiskChartOption" autoresize />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import { VChart } from '../../shared/components/AdminEChart'
import { useAdminMonitorData } from './useAdminMonitorData'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'
import './AdminStatisticsPage.css'

const { adminStatistics, timeRange, rangeOptions } = useUsageStatisticsData()
const { monitorData } = useAdminMonitorData()

const {
  totalCallsInRange,
  trendChartOption,
  todayCalls,
  todayFailed,
  todayCallsFoot,
  todayFailedFoot,
  todaySuccessRate,
  todaySuccessRateFoot,
  queuePressure,
  queuePressureFoot,
  modelRiskChartOption,
  qualityTrendChartOption,
  slowModelsChartOption,
  accountLoadChartOption,
  peakAccountLoadPercent,
  userRiskChartOption,
} = useUsageStatisticsCharts({
  adminStatistics,
  timeRange,
  monitorData,
})
</script>
