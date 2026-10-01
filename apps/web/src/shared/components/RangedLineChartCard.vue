<template>
  <ChartCard
    :title="title"
    :title-tag="titleTag"
    :header-class="headerClass"
    :title-class="titleClass"
    :class="cardClass"
    :elevated="elevated"
  >
    <template #actions>
      <div
        role="radiogroup"
        :aria-label="rangeAriaLabel ?? t('common.a11y.chartTimeRange')"
        :class="rangeGroupClass"
      >
        <button
          v-for="range in ranges"
          :key="range.key"
          type="button"
          role="radio"
          :aria-checked="selectedRange === range.key"
          class="ib-hit-target-32-inline cursor-pointer rounded-md px-2.5 py-1 font-medium transition-colors focus-visible:outline-none focus-visible:ring-2"
          :class="selectedRange === range.key ? rangeSelectedClass : rangeUnselectedClass"
          @click="selectedRange = range.key"
        >
          {{ range.label }}
        </button>
      </div>
    </template>

    <slot name="before-chart" />
    <ChartPanel :size="size" :height="height">
      <ChartRenderer
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

<script lang="ts">
import type { HTMLAttributes } from 'vue'
import type { Plugin } from 'chart.js'
import type { ChartData, ChartOptions } from '../lib/chartPrimitives'
import type { ChartPanelSize } from './ChartPanel.vue'

export interface RangedLineChartProps<TRange extends string> {
  title: string
  titleTag?: 'h2' | 'h3' | 'span' | undefined
  headerClass?: string | undefined
  titleClass?: string | undefined
  cardClass?: string | undefined
  /** Forwarded to `Card`. Flat surfaces pass `false` instead of cancelling a default shadow. */
  elevated?: boolean | undefined
  rangeGroupClass?: string | undefined
  rangeSelectedClass?: string | undefined
  rangeUnselectedClass?: string | undefined
  ranges: Array<{ label: string; key: TRange }>
  rangeAriaLabel?: string | undefined
  data: ChartData
  options: ChartOptions
  plugins?: Plugin[] | undefined
  canvasProps?: HTMLAttributes | undefined
  size?: ChartPanelSize | undefined
  height?: string | undefined
  showChart?: boolean | undefined
}
</script>

<script setup lang="ts" generic="TRange extends string">
import { useI18n } from 'vue-i18n'
import ChartCard from './ChartCard.vue'
import ChartPanel from './ChartPanel.vue'
import ChartRenderer from './ChartRenderer.vue'

// Neutral ranged chart primitive (#863): range-switch and card skin ride
// explicit class props with user-surface defaults. Admin pages use the
// `features/admin/shared` `AdminRangedLineChartCard` wrapper to supply
// the Admin skin instead of a feature-specific `variant` flag.
withDefaults(
  defineProps<RangedLineChartProps<TRange>>(),
  {
    titleTag: 'span',
    headerClass: undefined,
    titleClass: undefined,
    cardClass: undefined,
    elevated: undefined,
    rangeGroupClass:
      'inline-flex items-center rounded-lg border border-ib-border-soft bg-ib-surface-muted p-0.5 text-xs',
    rangeSelectedClass:
      'bg-ib-surface font-semibold text-ib-text shadow-xs focus-visible:ring-ib-focus',
    rangeUnselectedClass: 'text-ib-text-muted hover:text-ib-text focus-visible:ring-ib-focus',
    rangeAriaLabel: undefined,
    plugins: () => [],
    canvasProps: undefined,
    size: 'standard',
    height: '',
    showChart: true,
  },
)

const { t } = useI18n()

const selectedRange = defineModel<TRange>({ required: true })
</script>
