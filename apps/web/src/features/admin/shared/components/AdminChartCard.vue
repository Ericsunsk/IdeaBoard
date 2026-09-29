<template>
  <div :class="spanClass">
    <ChartCard
      :title="title"
      class="shadow-none"
      header-class="p-6 pb-4"
      title-class="text-base font-bold text-admin-text-1"
    >
      <template v-if="$slots.actions" #actions>
        <slot name="actions" />
      </template>
      <ChartPanel :size="chartSize" :height="height">
        <slot />
      </ChartPanel>
    </ChartCard>
  </div>
</template>

<script setup lang="ts">
import ChartCard from '../../../../shared/components/ChartCard.vue'
import ChartPanel, { type ChartPanelSize } from '../../../../shared/components/ChartPanel.vue'

withDefaults(
  defineProps<{
    title: string
    spanClass?: string
    chartSize?: ChartPanelSize
    height?: string
  }>(),
  {
    spanClass: '',
    chartSize: 'standard',
    height: '',
  },
)
</script>
