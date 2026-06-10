<template>
  <Card :class="spanClass" :pt="adminCardPt">
    <template #title>
      <AdminCardHeader :title="title">
        <template v-if="$slots.actions" #actions>
          <slot name="actions" />
        </template>
      </AdminCardHeader>
    </template>
    <template #content>
      <AdminChartPanel :size="chartSize" :height="height">
        <slot />
      </AdminChartPanel>
    </template>
  </Card>
</template>

<script setup lang="ts">
import AdminCardHeader from './AdminCardHeader.vue'
import AdminChartPanel from './AdminChartPanel.vue'
import { adminCardPt } from '../lib/adminCardPt'

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
