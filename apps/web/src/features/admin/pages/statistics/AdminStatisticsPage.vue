<template>
  <div :class="[adminDashboardRootClass, 'pb-6']">
    <div :class="adminSummaryGridClass">
      <AdminSummaryCard label="今日调用" :value="todayCalls" :footer="todayCallsFoot" />
      <AdminSummaryCard label="今日失败" :value="todayFailed" :footer="todayFailedFoot" />
      <AdminSummaryCard
        label="今日成功率"
        :value="todaySuccessRate"
        :footer="todaySuccessRateFoot"
      />
      <AdminSummaryCard label="本月调用" :value="monthCalls" :footer="monthCallsFoot" />
    </div>

    <div :class="adminChartGridClass">
      <AdminChartCard :title="usageCards.trend.title" span-class="xl:col-span-12" chart-size="hero">
        <template #actions>
          <SelectButton
            v-model="timeRange"
            :options="rangeOptions"
            option-label="label"
            option-value="key"
            :allow-empty="false"
            size="small"
          />
        </template>
        <Chart
          v-if="isChartReady"
          type="line"
          :class="adminChartCanvasClass"
          :data="trendChartData"
          :options="trendChartOptions"
          :plugins="adminChartPlugins"
        />
      </AdminChartCard>

      <AdminChartCard :title="usageCards.modelRisk.title" span-class="xl:col-span-7">
        <Chart
          v-if="isChartReady"
          type="scatter"
          :class="adminChartCanvasClass"
          :data="modelRiskChartData"
          :options="modelRiskChartOptions"
          :plugins="adminChartPlugins"
        />
      </AdminChartCard>

      <AdminChartCard :title="usageCards.qualityTrend.title" span-class="xl:col-span-5">
        <template #actions>
          <SelectButton
            v-model="qualityTimeRange"
            :options="qualityRangeOptions"
            option-label="label"
            option-value="key"
            :allow-empty="false"
            size="small"
          />
        </template>
        <Chart
          v-if="isChartReady"
          type="line"
          :class="adminChartCanvasClass"
          :data="qualityTrendChartData"
          :options="qualityTrendChartOptions"
          :plugins="adminChartPlugins"
        />
      </AdminChartCard>

      <div class="grid grid-cols-1 gap-3.5 xl:col-span-12 xl:grid-cols-3">
        <AdminChartCard :title="usageCards.slowModels.title" chart-size="compact">
          <Chart
            v-if="isChartReady"
            type="bubble"
            :class="adminChartCanvasClass"
            :data="slowModelsChartData"
            :options="slowModelsChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard :title="usageCards.bottleneckSource.title" chart-size="compact">
          <Chart
            v-if="isChartReady"
            type="polarArea"
            :class="adminChartCanvasClass"
            :data="bottleneckSourceChartData"
            :options="bottleneckSourceChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard :title="usageCards.failureCategories.title" chart-size="compact">
          <Chart
            v-if="isChartReady"
            type="polarArea"
            :class="adminChartCanvasClass"
            :data="failureCategoryChartData"
            :options="failureCategoryChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Chart from 'primevue/chart'
import AdminChartCard from '../../shared/components/AdminChartCard.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import { adminCards } from '../../shared/lib/adminCardMeta'
import { adminChartPlugins, useAdminChartReady } from '../../shared/lib/adminChart'
import {
  adminChartCanvasClass,
  adminChartGridClass,
  adminDashboardRootClass,
  adminSummaryGridClass,
} from '../../shared/lib/adminUiClasses'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'

const usageCards = adminCards.usageStatistics
const {
  adminStatistics,
  qualityStatistics,
  isStatisticsSettled,
  isQualityStatisticsSettled,
  timeRange,
  qualityTimeRange,
  rangeOptions,
  qualityRangeOptions,
} = useUsageStatisticsData()
const areStatisticsChartsSettled = computed(
  () => isStatisticsSettled.value && isQualityStatisticsSettled.value,
)
const isChartReady = useAdminChartReady(areStatisticsChartsSettled)

const {
  trendChartData,
  trendChartOptions,
  todayCalls,
  todayFailed,
  todayCallsFoot,
  todayFailedFoot,
  todaySuccessRate,
  todaySuccessRateFoot,
  monthCalls,
  monthCallsFoot,
  modelRiskChartData,
  modelRiskChartOptions,
  qualityTrendChartData,
  qualityTrendChartOptions,
  slowModelsChartData,
  slowModelsChartOptions,
  bottleneckSourceChartData,
  bottleneckSourceChartOptions,
  failureCategoryChartData,
  failureCategoryChartOptions,
} = useUsageStatisticsCharts({
  adminStatistics,
  qualityStatistics,
  timeRange,
  qualityTimeRange,
})
</script>
