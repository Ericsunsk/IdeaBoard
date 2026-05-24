<template>
  <div
    class="floating-status canvas-chrome canvas-chrome-status"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div class="zoom-controls canvas-control-group">
      <button
        type="button"
        class="zoom-btn canvas-icon-button"
        aria-label="缩小"
        @click="emit('zoom-out')"
      >
        <Minus class="zoom-btn-icon" />
      </button>
      <span class="zoom-level">{{ zoomLevel }}%</span>
      <button
        type="button"
        class="zoom-btn canvas-icon-button"
        aria-label="放大"
        @click="emit('zoom-in')"
      >
        <Plus class="zoom-btn-icon" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Minus, Plus } from '@iconoir/vue'

defineProps<{
  zoomLevel: number
  inert?: boolean
}>()

const emit = defineEmits<{
  (event: 'zoom-in'): void
  (event: 'zoom-out'): void
}>()
</script>

<style scoped>
@import './styles/canvas-controls.css';

.floating-status {
  position: absolute;
  right: 24px;
  bottom: 24px;
  z-index: 100;
  pointer-events: auto;
}

.zoom-controls {
  box-sizing: border-box;
  align-items: center;
  gap: 9px;
  height: 33px;
  padding: 4px;
  border: var(--canvas-control-border-width) solid #d8ddea;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.94);
  color: #202124;
}

.zoom-btn {
  width: 23px;
  height: 23px;
  margin: 0;
  padding: 0;
  border-radius: 999px;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--ib-control-icon);
  display: flex;
  align-items: center;
  justify-content: center;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}

.zoom-btn-icon {
  width: 16px;
  height: 16px;
  stroke-width: var(--canvas-control-icon-stroke);
}

.zoom-btn:hover {
  background: #f1f3f9;
  color: var(--ib-control-icon-hover);
}

.zoom-level {
  font-size: 13px;
  font-weight: 400;
  line-height: 1.2;
  color: #202124;
  min-width: 33px;
  text-align: center;
}
</style>
