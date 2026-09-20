<template>
  <div :class="adminDashboardRootClass">
    <AdminAlert v-if="hasStatisticsError" severity="error">
      {{ statisticsErrorMessage }}
    </AdminAlert>

    <template v-else>
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
        <RangedLineChartCard
          v-model="timeRange"
          title="调用量与消耗趋势"
          variant="admin"
          class="xl:col-span-12"
          :ranges="rangeOptions"
          :data="trendChartData"
          :options="trendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isChartReady"
          size="hero"
        />

        <AdminChartCard title="模型风险矩阵" span-class="xl:col-span-7">
          <ChartRenderer
            v-if="isChartReady"
            type="scatter"
            :class="adminChartCanvasClass"
            :data="modelRiskChartData"
            :options="modelRiskChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <RangedLineChartCard
          v-model="timeRange"
          title="服务质量趋势"
          variant="admin"
          class="xl:col-span-5"
          :ranges="rangeOptions"
          :data="qualityTrendChartData"
          :options="qualityTrendChartOptions"
          :plugins="adminChartPlugins"
          :show-chart="isChartReady"
        />

        <AdminChartCard title="瓶颈来源" chart-size="compact" span-class="xl:col-span-4">
          <ChartRenderer
            v-if="isChartReady"
            type="polarArea"
            :class="adminChartCanvasClass"
            :data="bottleneckSourceChartData"
            :options="bottleneckSourceChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminChartCard from '../../shared/components/AdminChartCard.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import ChartRenderer from '../../../../shared/components/ChartRenderer.vue'
import RangedLineChartCard from '../../../../shared/components/RangedLineChartCard.vue'
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
  formatDisplayErrorMessage(statisticsError.value, '加载统计数据失败'),
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
  bottleneckSourceChartData,
  bottleneckSourceChartOptions,
} = useUsageStatisticsCharts({
  adminStatistics,
  timeRange,
})
</script>
