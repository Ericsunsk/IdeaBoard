<template>
  <ChartCard :title="title" :title-tag="titleTag" :variant="variant">
    <template #actions>
      <div
        role="radiogroup"
        :aria-label="rangeAriaLabel ?? t('common.a11y.chartTimeRange')"
        :class="
          variant === 'admin'
            ? 'inline-flex h-8 items-center rounded-lg border border-admin-border-soft bg-admin-surface-muted p-1 text-xs text-admin-text-3'
            : 'inline-flex items-center rounded-lg border border-ib-border-soft bg-ib-surface-muted p-0.5 text-xs'
        "
      >
        <button
          v-for="range in ranges"
          :key="range.key"
          type="button"
          role="radio"
          :aria-checked="selectedRange === range.key"
          class="cursor-pointer rounded-md px-2.5 py-1 font-medium transition-colors focus-visible:outline-none focus-visible:ring-2"
          :class="
            variant === 'admin'
              ? selectedRange === range.key
                ? 'bg-white font-semibold text-admin-text-1 shadow-xs focus-visible:ring-admin-accent'
                : 'text-admin-text-2 hover:text-admin-text-1 focus-visible:ring-admin-accent'
              : selectedRange === range.key
                ? 'bg-ib-surface font-semibold text-ib-text shadow-xs focus-visible:ring-ib-focus'
                : 'text-ib-text-muted hover:text-ib-text focus-visible:ring-ib-focus'
          "
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

<script setup lang="ts" generic="TRange extends string">
import type { HTMLAttributes } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Plugin } from 'chart.js'
import type { ChartData, ChartOptions } from '../lib/chartPrimitives'
import ChartCard from './ChartCard.vue'
import ChartPanel, { type ChartPanelSize } from './ChartPanel.vue'
import ChartRenderer from './ChartRenderer.vue'

withDefaults(
  defineProps<{
    title: string
    titleTag?: 'h2' | 'h3' | 'span'
    variant?: 'default' | 'admin'
    ranges: Array<{ label: string; key: TRange }>
    rangeAriaLabel?: string | undefined
    data: ChartData
    options: ChartOptions
    plugins?: Plugin[]
    canvasProps?: HTMLAttributes | undefined
    size?: ChartPanelSize
    height?: string
    showChart?: boolean
  }>(),
  {
    titleTag: 'span',
    variant: 'default',
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
