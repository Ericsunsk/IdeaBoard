<template>
  <aside
    class="canvas-control-group canvas-control-group--vertical absolute top-1/2 left-4 z-[100] rounded-none border-0 bg-transparent p-0 [transform:translateY(-50%)]"
    :inert="inert"
    @wheel.prevent.stop
  >
    <button
      type="button"
      class="canvas-icon-button"
      :class="{ 'is-active': activeTool === 'select' }"
      aria-label="选择 (V)"
      :aria-pressed="activeTool === 'select'"
      @click="emit('select-tool', 'select')"
    >
      <OneFingerSelectHandGesture class="canvas-control-icon" />
    </button>
    <button
      type="button"
      class="canvas-icon-button"
      :class="{ 'is-active': activeTool === 'hand' }"
      aria-label="抓手 (H)"
      :aria-pressed="activeTool === 'hand'"
      @click="emit('select-tool', 'hand')"
    >
      <SpockHandGesture class="canvas-control-icon" />
    </button>
    <button
      type="button"
      class="canvas-icon-button"
      :class="{ 'is-active': minimapEnabled }"
      aria-label="小地图"
      :aria-pressed="minimapEnabled"
      title="小地图"
      @click="emit('toggle-minimap')"
    >
      <Map class="canvas-control-icon" />
    </button>
  </aside>
</template>

<script setup lang="ts">
import { Map, OneFingerSelectHandGesture, SpockHandGesture } from '@iconoir/vue'

defineProps<{
  activeTool: 'select' | 'hand'
  minimapEnabled: boolean
  inert?: boolean
}>()

const emit = defineEmits<{
  (event: 'select-tool', tool: 'select' | 'hand'): void
  (event: 'toggle-minimap'): void
}>()
</script>
