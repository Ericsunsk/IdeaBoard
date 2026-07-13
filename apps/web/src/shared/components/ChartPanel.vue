<template>
  <div class="min-w-0" :class="sizeClass" :style="panelStyle">
    <slot />
  </div>
</template>

<script lang="ts">
export type ChartPanelSize = 'hero' | 'standard' | 'compact' | 'monitor'
</script>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: ChartPanelSize
    height?: string
  }>(),
  { size: 'standard', height: '' },
)

const heightClassBySize: Record<ChartPanelSize, string> = {
  hero: 'h-[240px] md:h-[278px]',
  standard: 'h-[300px] md:h-[320px] xl:h-[360px]',
  compact: 'h-[220px]',
  monitor: 'h-[380px]',
}

const sizeClass = computed(() => (props.height ? '' : heightClassBySize[props.size]))
const panelStyle = computed(() => (props.height ? { height: props.height } : undefined))
</script>
