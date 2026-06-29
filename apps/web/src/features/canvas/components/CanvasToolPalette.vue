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
      :class="{ 'is-active': activeTool === 'text' }"
      aria-label="文本工具稍后接入"
      :aria-pressed="activeTool === 'text'"
      title="文本工具稍后接入"
      disabled
      @click="emit('select-tool', 'text')"
    >
      <Text class="canvas-control-icon" />
    </button>
  </aside>
</template>

<script setup lang="ts">
import { OneFingerSelectHandGesture, SpockHandGesture, Text } from '@iconoir/vue'

defineProps<{
  activeTool: 'select' | 'hand' | 'text'
  inert?: boolean
}>()

const emit = defineEmits<{
  (event: 'select-tool', tool: 'select' | 'hand' | 'text'): void
}>()
</script>

<style scoped>
@import './styles/canvas-controls.css';
</style>
