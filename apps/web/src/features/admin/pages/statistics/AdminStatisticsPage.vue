<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0 pb-6">
    <div class="grid grid-cols-1 gap-3.5 md:grid-cols-2 xl:grid-cols-4">
      <AdminSummaryCard label="今日调用" :value="todayCalls" :footer="todayCallsFoot" />
      <AdminSummaryCard label="今日失败" :value="todayFailed" :footer="todayFailedFoot" />
      <AdminSummaryCard
        label="今日成功率"
        :value="todaySuccessRate"
        :footer="todaySuccessRateFoot"
      />
      <AdminSummaryCard label="本月调用" :value="monthCalls" :footer="monthCallsFoot" />
    </div>

    <div class="grid grid-cols-1 gap-3.5 xl:grid-cols-12">
      <Card class="xl:col-span-12" :pt="adminCardPt">
        <template #title>
          <AdminCardHeader :title="usageCards.trend.title">
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
          </AdminCardHeader>
        </template>
        <template #content>
          <AdminChartPanel size="hero">
            <Chart
              v-if="isChartReady"
              type="line"
              class="h-full w-full min-w-0"
              :data="trendChartData"
              :options="trendChartOptions"
              :plugins="adminChartPlugins"
            />
          </AdminChartPanel>
        </template>
      </Card>

      <Card class="xl:col-span-7" :pt="adminCardPt">
        <template #title>
          <AdminCardHeader :title="usageCards.modelRisk.title" />
        </template>
        <template #content>
          <AdminChartPanel>
            <Chart
              v-if="isChartReady"
              type="scatter"
              class="h-full w-full min-w-0"
              :data="modelRiskChartData"
              :options="modelRiskChartOptions"
              :plugins="adminChartPlugins"
            />
          </AdminChartPanel>
        </template>
      </Card>
      <Card class="xl:col-span-5" :pt="adminCardPt">
        <template #title>
          <AdminCardHeader :title="usageCards.qualityTrend.title">
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
          </AdminCardHeader>
        </template>
        <template #content>
          <AdminChartPanel>
            <Chart
              v-if="isChartReady"
              type="line"
              class="h-full w-full min-w-0"
              :data="qualityTrendChartData"
              :options="qualityTrendChartOptions"
              :plugins="adminChartPlugins"
            />
          </AdminChartPanel>
        </template>
      </Card>

      <div class="grid grid-cols-1 gap-3.5 xl:col-span-12 xl:grid-cols-3">
        <Card :pt="adminCardPt">
          <template #title>
            <AdminCardHeader :title="usageCards.slowModels.title" />
          </template>
          <template #content>
            <AdminChartPanel size="compact">
              <Chart
                v-if="isChartReady"
                type="bubble"
                class="h-full w-full min-w-0"
                :data="slowModelsChartData"
                :options="slowModelsChartOptions"
                :plugins="adminChartPlugins"
              />
            </AdminChartPanel>
          </template>
        </Card>

        <Card :pt="adminCardPt">
          <template #title>
            <AdminCardHeader :title="usageCards.bottleneckSource.title" />
          </template>
          <template #content>
            <AdminChartPanel size="compact">
              <Chart
                v-if="isChartReady"
                type="polarArea"
                class="h-full w-full min-w-0"
                :data="bottleneckSourceChartData"
                :options="bottleneckSourceChartOptions"
                :plugins="adminChartPlugins"
              />
            </AdminChartPanel>
          </template>
        </Card>

        <Card :pt="adminCardPt">
          <template #title>
            <AdminCardHeader :title="usageCards.failureCategories.title" />
          </template>
          <template #content>
            <AdminChartPanel size="compact">
              <Chart
                v-if="isChartReady"
                type="polarArea"
                class="h-full w-full min-w-0"
                :data="failureCategoryChartData"
                :options="failureCategoryChartOptions"
                :plugins="adminChartPlugins"
              />
            </AdminChartPanel>
          </template>
        </Card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Chart from 'primevue/chart'
import AdminCardHeader from '../../shared/components/AdminCardHeader.vue'
import AdminChartPanel from '../../shared/components/AdminChartPanel.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminCards } from '../../shared/lib/adminCardMeta'
import { adminChartPlugins, useAdminChartReady } from '../../shared/lib/adminChart'
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
