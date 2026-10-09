<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="hasStatisticsError" severity="error">
      {{ statisticsErrorMessage }}
    </AdminAlert>

    <template v-else>
      <div :class="adminSummaryGridClass">
        <AdminSummaryCard
          :label="STATISTICS_MESSAGES.todayCalls"
          :value="todayCalls"
          :footer="todayCallsFoot"
        />
        <AdminSummaryCard
          :label="STATISTICS_MESSAGES.todayFailed"
          :value="todayFailed"
          :footer="todayFailedFoot"
        />
        <AdminSummaryCard
          :label="STATISTICS_MESSAGES.todaySuccessRate"
          :value="todaySuccessRate"
          :footer="todaySuccessRateFoot"
        />
        <AdminSummaryCard
          :label="STATISTICS_MESSAGES.monthCalls"
          :value="monthCalls"
          :footer="monthCallsFoot"
        />
      </div>

      <div :class="adminChartGridClass">
        <AdminRangedLineChartCard
          v-model="timeRange"
          :title="STATISTICS_MESSAGES.trendTitle"
          class="xl:col-span-12"
          :ranges="rangeOptions"
          :data="trendChartData"
          :options="trendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isChartReady"
          size="hero"
        />

        <AdminChartCard :title="STATISTICS_MESSAGES.riskMatrixTitle" span-class="xl:col-span-7">
          <ChartRenderer
            v-if="isChartReady"
            type="scatter"
            :class="adminChartCanvasClass"
            :data="modelRiskChartData"
            :options="modelRiskChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminRangedLineChartCard
          v-model="timeRange"
          :title="STATISTICS_MESSAGES.qualityTrendTitle"
          class="xl:col-span-5"
          :ranges="rangeOptions"
          :data="qualityTrendChartData"
          :options="qualityTrendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isChartReady"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminChartCard from '../../shared/components/AdminChartCard.vue'
import AdminRangedLineChartCard from '../../shared/components/AdminRangedLineChartCard.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import ChartRenderer from '../../../../shared/components/ChartRenderer.vue'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { adminChartPlugins, useAdminChartReady } from '../../shared/lib/chart'
import {
  adminChartCanvasClass,
  adminChartGridClass,
  adminDashboardRootClass,
  adminSummaryGridClass,
} from '../../shared/lib/uiClasses'
import { useUsageStatisticsCharts } from '../useUsageStatisticsCharts'
import { useUsageStatisticsData } from '../useUsageStatisticsData'
import { STATISTICS_MESSAGES } from './messages'

const {
  adminStatistics,
  isStatisticsSettled,
  isStatisticsError,
  statisticsError,
  timeRange,
  rangeOptions,
} = useUsageStatisticsData()
const areStatisticsChartsSettled = computed(() => isStatisticsSettled.value)
const isChartReady = useAdminChartReady(areStatisticsChartsSettled)
const hasStatisticsError = computed(() => isStatisticsError.value)
const statisticsErrorMessage = computed(() =>
  formatDisplayErrorMessage(statisticsError.value, STATISTICS_MESSAGES.loadFailed),
)

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
} = useUsageStatisticsCharts({
  adminStatistics,
  timeRange,
})
</script>
