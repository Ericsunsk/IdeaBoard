<template>
  <aside
    class="floating-toolbar canvas-chrome canvas-chrome-left canvas-control-group canvas-control-group--vertical"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div
      class="canvas-icon-button"
      :class="{ 'is-active': activeTool === 'select' }"
      title="Select (V)"
      @click="emit('select-tool', 'select')"
    >
      <OneFingerSelectHandGesture class="canvas-control-icon" />
    </div>
    <div
      class="canvas-icon-button"
      :class="{ 'is-active': activeTool === 'hand' }"
      title="Hand (H)"
      @click="emit('select-tool', 'hand')"
    >
      <SpockHandGesture class="canvas-control-icon" />
    </div>
    <div
      class="canvas-icon-button"
      :class="{ 'is-active': activeTool === 'text' }"
      title="Text (T)"
      @click="emit('select-tool', 'text')"
    >
      <Text class="canvas-control-icon" />
    </div>
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
