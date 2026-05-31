<template>
  <div class="admin-page statistics-page">
    <div class="summary-grid">
      <AdminSummaryCard label="今日调用" :value="todayCalls" :foot="todayCallsFoot" />
      <AdminSummaryCard label="今日失败" :value="todayFailed" :foot="todayFailedFoot" />
      <AdminSummaryCard label="今日成功率" :value="todaySuccessRate" :foot="todaySuccessRateFoot" />
      <AdminSummaryCard label="队列压力" :value="queuePressure" :foot="queuePressureFoot" />
    </div>

    <div class="charts-grid">
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
        <div class="chart-body area-chart-container">
          <VChart class="echart-host trend-echart" :option="trendChartOption" autoresize />
        </div>
      </div>

      <div class="chart-card pie-chart-card">
        <div class="chart-header-row">
          <h3 class="chart-title">模型用量占比</h3>
          <n-popover
            trigger="click"
            placement="bottom-end"
            :show-arrow="false"
            raw
            :theme-overrides="adminPopoverPanelThemeOverrides"
          >
            <template #trigger>
              <n-button quaternary circle size="small">
                <template #icon
                  ><n-icon><Settings /></n-icon
                ></template>
              </n-button>
            </template>
            <div class="price-card-popover">
              <div class="popover-body">
                <n-config-provider :theme-overrides="popoverFormThemeOverrides">
                  <n-form
                    size="small"
                    :show-feedback="false"
                    label-placement="left"
                    label-width="150"
                    label-align="left"
                  >
                    <n-form-item
                      v-for="m in modelSettingCandidates"
                      :key="m.name"
                      :label="m.name"
                      class="modal-style-item"
                    >
                      <n-input-number
                        placeholder="0.00"
                        :precision="4"
                        :min="0"
                        style="width: 100%"
                      >
                        <template #prefix>$</template>
                      </n-input-number>
                    </n-form-item>
                  </n-form>
                </n-config-provider>
              </div>
            </div>
          </n-popover>
        </div>
        <div class="chart-body consumption-dashboard">
          <div class="donut-section-large">
            <div class="donut-wrapper-large">
              <VChart
                class="echart-host donut-echart"
                :option="consumptionChartOption"
                autoresize
              />
            </div>
          </div>

          <div class="consumption-legend">
            <div v-for="item in modelRanking" :key="item.name" class="legend-row">
              <div class="legend-left">
                <span class="legend-marker" :style="{ backgroundColor: item.color }"></span>
                <span class="legend-name">
                  <span class="legend-model-name" :title="item.name">{{ item.name }}</span>
                  <span class="legend-provider">({{ item.provider }})</span>
                </span>
              </div>
              <div class="legend-metric">
                <span>{{ item.value.toLocaleString() }}</span>
                <span class="legend-percent">{{ item.percent }}%</span>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="chart-card health-card">
        <div class="health-header">
          <h3 class="chart-title">服务健康监测</h3>
          <div class="health-stats">
            <span class="health-label">{{ healthRangeLabel }}</span>
            <span class="health-value">{{ healthRate }}</span>
          </div>
        </div>
        <div class="chart-body health-chart-container">
          <VChart class="echart-host health-echart" :option="healthHeatmapOption" autoresize />
        </div>
        <div class="health-legend">
          <div class="legend-item">
            <span class="legend-dot is-success"></span><span>正常</span>
          </div>
          <div class="legend-item">
            <span class="legend-dot is-warning"></span><span>波动</span>
          </div>
          <div class="legend-item"><span class="legend-dot is-error"></span><span>故障</span></div>
          <div class="legend-spacer"></div>
          <div class="legend-item">
            <span class="legend-dot is-empty"></span><span>无数据</span>
          </div>
        </div>
      </div>
      <div class="metrics-row">
        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">模型响应延迟 (TTFT)</span>
          </div>
          <div class="chart-body metric-chart-container">
            <VChart class="echart-host mini-bar-echart" :option="latencyChartOption" autoresize />
          </div>
        </div>

        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">渠道并发负载</span>
            <span class="metric-value"
              >{{ saturatedAccountCount }} <span class="unit">个饱和</span></span
            >
          </div>
          <div class="chart-body metric-chart-container">
            <VChart
              class="echart-host mini-bar-echart"
              :option="accountLoadChartOption"
              autoresize
            />
          </div>
        </div>

        <div class="chart-card metric-mini-card">
          <div class="metric-header">
            <span class="metric-title">高耗用户排行</span>
          </div>
          <div class="chart-body metric-chart-container">
            <VChart class="echart-host mini-bar-echart" :option="userRankChartOption" autoresize />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  NButton,
  NForm,
  NFormItem,
  NConfigProvider,
  NIcon,
  NInputNumber,
  NPopover,
  type GlobalThemeOverrides,
} from 'naive-ui'
import { Settings } from '@iconoir/vue'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'
import { VChart } from '../shared/components/AdminEChart'
import { adminPopoverPanelThemeOverrides } from '../shared/adminThemeOverrides'
import { useAdminMonitorData } from './useAdminMonitorData'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'
import './UsageStatisticsScreen.css'

const popoverFormThemeOverrides: GlobalThemeOverrides = {
  Form: {
    labelFontSizeLeftSmall: '13px',
    labelTextColor: 'var(--admin-text-3)',
    labelFontWeight: '400',
    labelHeightSmall: '32px',
  },
  Input: {
    borderRadius: '10px',
    color: 'var(--admin-card)',
    colorFocus: 'var(--admin-card)',
    heightSmall: '32px',
    border: '1px solid var(--admin-border)',
    borderHover: '1px solid var(--admin-border)',
    borderFocus: '1px solid var(--admin-border)',
    boxShadowFocus: 'none',
  },
}
const { adminStatistics, configuredAccounts, configuredChannels, timeRange, rangeOptions } =
  useUsageStatisticsData()
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
  modelRanking,
  modelSettingCandidates,
  consumptionChartOption,
  healthHeatmapOption,
  latencyChartOption,
  accountLoadChartOption,
  userRankChartOption,
  healthRangeLabel,
  healthRate,
  saturatedAccountCount,
} = useUsageStatisticsCharts({
  adminStatistics,
  configuredAccounts,
  configuredChannels,
  timeRange,
  monitorData,
})
</script>
