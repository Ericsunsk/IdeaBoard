<template>
  <ChartCard :title="title" :class="spanClass" :pt="adminCardPt">
    <template v-if="$slots.actions" #actions>
      <slot name="actions" />
    </template>
    <ChartPanel :size="chartSize" :height="height">
      <slot />
    </ChartPanel>
  </ChartCard>
</template>

<script setup lang="ts">
import ChartCard from '../../../../shared/components/ChartCard.vue'
import ChartPanel from '../../../../shared/components/ChartPanel.vue'
import { adminCardPt } from '../lib/cardPt'

type AdminChartPanelSize = 'hero' | 'standard' | 'compact' | 'monitor'

withDefaults(
  defineProps<{
    title: string
    spanClass?: string
    chartSize?: AdminChartPanelSize
    height?: string
  }>(),
  {
    spanClass: '',
    chartSize: 'standard',
    height: '',
  },
)
</script>
