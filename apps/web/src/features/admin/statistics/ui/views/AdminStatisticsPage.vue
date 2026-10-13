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
          v-model="trend.timeRange.value"
          :title="STATISTICS_MESSAGES.trendTitle"
          class="xl:col-span-12"
          :ranges="trend.rangeOptions"
          :range-aria-label="STATISTICS_MESSAGES.trendTitle"
          :data="trendChartData"
          :options="trendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isTrendChartReady && !trend.isStatisticsError.value"
          size="hero"
        >
          <template #before-chart>
            <AdminAlert v-if="trend.isStatisticsError.value" severity="error">
              {{ trendErrorMessage }}
            </AdminAlert>
          </template>
        </AdminRangedLineChartCard>

        <AdminChartCard :title="STATISTICS_MESSAGES.riskMatrixTitle" span-class="xl:col-span-7">
          <ChartRenderer
            v-if="isOverviewChartReady"
            type="scatter"
            :class="adminChartCanvasClass"
            :data="modelRiskChartData"
            :options="modelRiskChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminRangedLineChartCard
          v-model="quality.timeRange.value"
          :title="STATISTICS_MESSAGES.qualityTrendTitle"
          class="xl:col-span-5"
          :ranges="quality.rangeOptions"
          :range-aria-label="STATISTICS_MESSAGES.qualityTrendTitle"
          :data="qualityTrendChartData"
          :options="qualityTrendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isQualityChartReady && !quality.isStatisticsError.value"
        >
          <template #before-chart>
            <AdminAlert v-if="quality.isStatisticsError.value" severity="error">
              {{ qualityErrorMessage }}
            </AdminAlert>
          </template>
        </AdminRangedLineChartCard>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { AdminAlert } from '../../../shared'
import { AdminChartCard } from '../../../shared'
import { AdminRangedLineChartCard } from '../../../shared'
import { AdminSummaryCard } from '../../../shared'
import ChartRenderer from '../../../../../core/components/ChartRenderer.vue'
import { formatDisplayErrorMessage } from '../../../../../core/utils/errorMessages'
import { adminChartPlugins, useAdminChartReady } from '../../../shared'
import {
  adminChartCanvasClass,
  adminChartGridClass,
  adminDashboardRootClass,
  adminSummaryGridClass,
} from '../../../shared'
import { useUsageStatisticsCharts } from '../charts/useUsageStatisticsCharts'
import { useUsageStatisticsData } from '../../session/useUsageStatisticsData'
import { STATISTICS_MESSAGES } from './messages'

const overview = useUsageStatisticsData()
const trend = useUsageStatisticsData()
const quality = useUsageStatisticsData()
const isOverviewChartReady = useAdminChartReady(overview.isStatisticsSettled)
const isTrendChartReady = useAdminChartReady(trend.isStatisticsSettled)
const isQualityChartReady = useAdminChartReady(quality.isStatisticsSettled)
const hasStatisticsError = computed(() => overview.isStatisticsError.value)
const statisticsErrorMessage = computed(() =>
  formatDisplayErrorMessage(overview.statisticsError.value, STATISTICS_MESSAGES.loadFailed),
)
const trendErrorMessage = computed(() =>
  formatDisplayErrorMessage(trend.statisticsError.value, STATISTICS_MESSAGES.loadFailed),
)
const qualityErrorMessage = computed(() =>
  formatDisplayErrorMessage(quality.statisticsError.value, STATISTICS_MESSAGES.loadFailed),
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
  overviewStatistics: overview.adminStatistics,
  trend,
  quality,
})
</script>
