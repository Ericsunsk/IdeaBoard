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
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">队列压力</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ queuePressure }}
          </span>
        </template>
        <template #content>{{ queuePressureFoot }}</template>
      </Card>
    </div>

    <div class="grid grid-cols-1 gap-3.5 xl:grid-cols-12">
      <Card class="xl:col-span-12">
        <template #title>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <span>调用量趋势</span>
              <p class="mt-1 text-sm font-normal text-admin-text-3">
                <span v-if="timeRange === '7d'">最近 7 天总计</span>
                <span v-else-if="timeRange === '30d'">最近 30 天总计</span>
                <span v-else>过去一年总计</span>
                <span class="font-semibold text-admin-text-1">
                  {{ totalCallsInRange.toLocaleString() }}
                </span>
                次
              </p>
            </div>
            <SelectButton
              v-model="timeRange"
              :options="rangeOptions"
              option-label="label"
              option-value="key"
              :allow-empty="false"
              size="small"
            />
          </div>
        </template>
        <template #content>
          <div class="h-[240px] min-w-0 md:h-[278px]">
            <Chart
              v-if="isChartReady"
              type="line"
              class="h-full w-full min-w-0"
              :data="trendChartData"
              :options="trendChartOptions"
              :plugins="adminChartPlugins"
            />
          </div>
        </template>
      </Card>

      <Card class="xl:col-span-7">
        <template #title>模型风险矩阵</template>
        <template #content>
          <div class="h-[300px] min-w-0 md:h-[320px] xl:h-[360px]">
            <Chart
              v-if="isChartReady"
              type="scatter"
              class="h-full w-full min-w-0"
              :data="modelRiskChartData"
              :options="modelRiskChartOptions"
              :plugins="adminChartPlugins"
            />
          </div>
        </template>
      </Card>
      <Card class="xl:col-span-5">
        <template #title>服务质量趋势</template>
        <template #content>
          <div class="h-[240px] min-w-0 md:h-[300px] xl:h-[360px]">
            <Chart
              v-if="isChartReady"
              type="bar"
              class="h-full w-full min-w-0"
              :data="qualityTrendChartData"
              :options="qualityTrendChartOptions"
              :plugins="adminChartPlugins"
            />
          </div>
        </template>
      </Card>

      <div class="grid grid-cols-1 gap-3.5 xl:col-span-12 xl:grid-cols-3">
        <Card>
          <template #title>慢模型 P95</template>
          <template #content>
            <div class="h-[178px] min-w-0 md:h-[164px]">
              <Chart
                v-if="isChartReady"
                type="bar"
                class="h-full w-full min-w-0"
                :data="slowModelsChartData"
                :options="slowModelsChartOptions"
                :plugins="adminChartPlugins"
              />
            </div>
          </template>
        </Card>

        <Card>
          <template #title>
            <div class="flex items-start justify-between gap-3">
              <span>渠道并发负载</span>
              <span class="text-base font-semibold text-admin-text-1">
                {{ peakAccountLoadPercent }}<span class="text-xs text-admin-text-3">% 峰值</span>
              </span>
            </div>
          </template>
          <template #content>
            <div class="h-[178px] min-w-0 md:h-[164px]">
              <Chart
                v-if="isChartReady"
                type="bar"
                class="h-full w-full min-w-0"
                :data="accountLoadChartData"
                :options="accountLoadChartOptions"
                :plugins="adminChartPlugins"
              />
            </div>
          </template>
        </Card>

        <Card>
          <template #title>异常用户排行</template>
          <template #content>
            <div class="h-[178px] min-w-0 md:h-[164px]">
              <Chart
                v-if="isChartReady"
                type="bar"
                class="h-full w-full min-w-0"
                :data="userRiskChartData"
                :options="userRiskChartOptions"
                :plugins="adminChartPlugins"
              />
            </div>
          </template>
        </Card>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Chart from 'primevue/chart'
import { adminChartPlugins, useAdminChartReady } from '../../shared/lib/adminChart'
import { useAdminMonitorData } from './useAdminMonitorData'
import { useUsageStatisticsData } from './useUsageStatisticsData'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'

const { adminStatistics, isStatisticsSettled, timeRange, rangeOptions } = useUsageStatisticsData()
const { monitorData } = useAdminMonitorData()
const isChartReady = useAdminChartReady(isStatisticsSettled)

const {
  totalCallsInRange,
  trendChartData,
  trendChartOptions,
  todayCalls,
  todayFailed,
  todayCallsFoot,
  todayFailedFoot,
  todaySuccessRate,
  todaySuccessRateFoot,
  queuePressure,
  queuePressureFoot,
  modelRiskChartData,
  modelRiskChartOptions,
  qualityTrendChartData,
  qualityTrendChartOptions,
  slowModelsChartData,
  slowModelsChartOptions,
  accountLoadChartData,
  accountLoadChartOptions,
  peakAccountLoadPercent,
  userRiskChartData,
  userRiskChartOptions,
} = useUsageStatisticsCharts({
  adminStatistics,
  timeRange,
  monitorData,
})
</script>
