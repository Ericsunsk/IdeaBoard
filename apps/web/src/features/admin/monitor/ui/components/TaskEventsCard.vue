<template>
  <div
    class="flex w-full flex-col overflow-hidden rounded-panel border border-border bg-background"
  >
    <!-- 表头：带外层边框，底色同页面背景 (bg-background) -->
    <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5">
      <!-- 标题 Events + ℹ 提示图标 -->
      <div class="flex items-center gap-1.5">
        <span class="text-sm font-bold text-foreground">Events</span>
        <span
          :title="MONITOR_MESSAGES.eventsInfoTitle"
          class="flex size-4 cursor-pointer items-center justify-center rounded-pill text-muted-foreground hover:text-foreground"
        >
          <InfoCircle class="size-3.5" />
        </span>
      </div>

      <!-- 右侧：数据类型指标胶囊 + 折叠按钮 -->
      <div class="flex items-center gap-2">
        <button
          v-for="metric in taskMetricDefinitions"
          :key="metric.key"
          type="button"
          :class="[
            // 20px pills sit in a dense rail; `hit-target-32-inline` grows each
            // target to the repo floor vertically without loosening the row.
            'hit-target-32-inline inline-flex items-center gap-1.5 text-xs font-medium cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] px-2 py-0.5 rounded-control',
            filterStatus === metric.status
              ? metric.activePillClass
              : 'text-muted-foreground hover:text-foreground',
          ]"
          :title="metric.pillTitle"
          @click="emit('toggleFilterStatus', metric.status)"
        >
          <span :class="['size-2 rounded-pill', metric.dotClass]"></span>
          <span>{{ metric.getCount(metricStatsSnapshot) }} {{ metric.label }}</span>
        </button>

        <!-- 折叠按钮: 独立的圆角矩形小按钮 (^) -->
        <button
          type="button"
          :class="adminCompactIconButtonClass"
          :title="MONITOR_MESSAGES.chartsToggle(chartsCollapsed)"
          :aria-label="MONITOR_MESSAGES.chartsToggle(chartsCollapsed)"
          @click="emit('update:chartsCollapsed', !chartsCollapsed)"
        >
          <NavArrowUp v-if="!chartsCollapsed" class="size-4" />
          <NavArrowDown v-else class="size-4" />
        </button>
      </div>
    </div>

    <!-- 表身：白色卡片 (bg-white)，撑满卡片下方，衔接处圆角与边框分界 -->
    <div
      v-show="!chartsCollapsed"
      class="w-full rounded-t-panel border-t border-border bg-card px-4 py-3"
      :style="{ height: `${chartHeightPx}px` }"
    >
      <ChartRenderer
        type="line"
        :data="timelineChartData"
        :options="timelineChartOptions"
        :plugins="adminChartPlugins"
        class="h-full w-full"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { InfoCircle, NavArrowDown, NavArrowUp } from '@iconoir/vue'
import ChartRenderer from '../../../../../core/components/ChartRenderer.vue'
import { adminCompactIconButtonClass } from '../../../shared'
import { taskMetricDefinitions, type TaskMetricStatsSnapshot } from '../../../shared'
import type { useMonitorTimeline } from '../../session/useMonitorTimeline'
import { MONITOR_MESSAGES } from '../pages/messages'

type TimelineState = ReturnType<typeof useMonitorTimeline>

defineProps<{
  filterStatus: string | null
  metricStatsSnapshot: TaskMetricStatsSnapshot
  chartsCollapsed: boolean
  timelineChartData: TimelineState['timelineChartData']['value']
  timelineChartOptions: TimelineState['timelineChartOptions']['value']
  adminChartPlugins: TimelineState['adminChartPlugins']
  chartHeightPx: number
}>()

const emit = defineEmits<{
  'update:chartsCollapsed': [value: boolean]
  toggleFilterStatus: [status: string | null]
}>()
</script>
