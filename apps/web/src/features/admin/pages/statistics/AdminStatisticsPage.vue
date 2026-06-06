<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0 pb-6">
    <div class="grid grid-cols-1 gap-3.5 md:grid-cols-2 xl:grid-cols-4">
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">今日调用</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ todayCalls }}
          </span>
        </template>
        <template #content>{{ todayCallsFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">今日失败</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ todayFailed }}
          </span>
        </template>
        <template #content>{{ todayFailedFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">今日成功率</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ todaySuccessRate }}
          </span>
        </template>
        <template #content>{{ todaySuccessRateFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">本月调用</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ monthCalls }}
          </span>
        </template>
        <template #content>{{ monthCallsFoot }}</template>
      </Card>
    </div>

    <div class="grid grid-cols-1 gap-3.5 xl:grid-cols-12">
      <Card class="xl:col-span-12">
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

      <Card class="xl:col-span-7">
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
      <Card class="xl:col-span-5">
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
        <Card>
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

        <Card>
          <template #title>
            <AdminCardHeader :title="usageCards.bottleneckSource.title" />
          </template>
          <template #content>
            <AdminChartPanel size="compact">
              <Chart
                v-if="isChartReady"
                type="doughnut"
                class="h-full w-full min-w-0"
                :data="bottleneckSourceChartData"
                :options="bottleneckSourceChartOptions"
                :plugins="adminChartPlugins"
              />
            </AdminChartPanel>
          </template>
        </Card>

        <Card>
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
import Chart from 'primevue/chart'
import AdminCardHeader from '../../shared/components/AdminCardHeader.vue'
import AdminChartPanel from '../../shared/components/AdminChartPanel.vue'
import { adminCards } from '../../shared/lib/adminCardMeta'
import { adminChartPlugins, useAdminChartReady } from '../../shared/lib/adminChart'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'

const usageCards = adminCards.usageStatistics
const {
  adminStatistics,
  qualityStatistics,
  isStatisticsSettled,
  timeRange,
  qualityTimeRange,
  rangeOptions,
  qualityRangeOptions,
} = useUsageStatisticsData()
const isChartReady = useAdminChartReady(isStatisticsSettled)

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
