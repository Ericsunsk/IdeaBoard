<template>
  <ChartCard :title="title" :title-tag="titleTag" :pt="panelCardPt">
    <template #actions>
      <SelectButton
        v-model="selectedRange"
        :options="ranges"
        option-label="label"
        option-value="key"
        :allow-empty="false"
        size="small"
        :aria-label="rangeAriaLabel"
      />
    </template>

    <slot name="before-chart" />
    <ChartPanel :size="size" :height="height">
      <Chart
        v-if="showChart"
        type="line"
        class="h-full w-full min-w-0"
        :data="data"
        :options="options"
        :plugins="plugins"
        :canvas-props="canvasProps"
      />
      <slot v-else name="chart-fallback" />
    </ChartPanel>
    <slot name="after-chart" />
  </ChartCard>
</template>

<script setup lang="ts" generic="TRange extends string">
import type { HTMLAttributes } from 'vue'
import type { Plugin } from 'chart.js'
import Chart from 'primevue/chart'
import SelectButton from 'primevue/selectbutton'
import type { ChartData, ChartOptions } from '../lib/chartPrimitives'
import { panelCardPt } from '../theme/panelCardPt'
import ChartCard from './ChartCard.vue'
import ChartPanel, { type ChartPanelSize } from './ChartPanel.vue'

withDefaults(
  defineProps<{
    title: string
    titleTag?: 'h2' | 'h3' | 'span'
    ranges: Array<{ label: string; key: TRange }>
    rangeAriaLabel?: string
    data: ChartData
    options: ChartOptions
    plugins?: Plugin[]
    canvasProps?: HTMLAttributes
    size?: ChartPanelSize
    height?: string
    showChart?: boolean
  }>(),
  {
    titleTag: 'span',
    rangeAriaLabel: '图表时间范围',
    plugins: () => [],
    canvasProps: undefined,
    size: 'standard',
    height: '',
    showChart: true,
  },
)

const selectedRange = defineModel<TRange>({ required: true })
</script>
