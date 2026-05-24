<template>
  <aside
    class="floating-toolbar canvas-chrome canvas-chrome-left canvas-control-group canvas-control-group--vertical"
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
      aria-label="文本 (T)"
      :aria-pressed="activeTool === 'text'"
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

.floating-toolbar {
  position: absolute;
  left: 16px;
  top: 50%;
  transform: translateY(-50%);
  background: transparent;
  padding: 0;
  border-radius: 0;
  border: none;
  z-index: 100;
}
</style>
