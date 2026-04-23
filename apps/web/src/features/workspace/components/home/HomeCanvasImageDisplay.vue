<template>
  <transition name="canvas-image-content" mode="out-in">
    <div
      v-if="isPending"
      :key="`pending-${item.id}`"
      class="canvas-image-placeholder"
      :style="{ height: `${displayHeight}px` }"
    >
      <div class="canvas-image-placeholder-shimmer"></div>
      <div class="canvas-image-placeholder-center">
        <span class="canvas-image-placeholder-spinner" aria-hidden="true"></span>
      </div>
    </div>
    <img
      v-else-if="item.type === 'image'"
      :key="`image-${item.id}-${item.url}`"
      :src="item.url"
      class="canvas-img"
      draggable="false"
      :ref="setImageRef"
      @load="emit('image-load', $event)"
    />
  </transition>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import type { CanvasImageItem } from '../../types/canvasNodes'

defineProps<{
  item: CanvasImageItem
  isPending: boolean
  displayHeight: number
  setImageRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (event: 'image-load', value: Event): void
}>()
</script>

<style scoped>
.canvas-img {
  width: 100%;
  display: block;
  border-radius: 8px;
  pointer-events: none;
  user-select: none;
  -webkit-user-select: none;
  -webkit-user-drag: none;
}

.canvas-image-placeholder {
  position: relative;
  width: 100%;
  min-height: 72px;
  overflow: hidden;
  border-radius: 8px;
  background:
    radial-gradient(circle at top, rgba(255, 255, 255, 0.85), rgba(255, 255, 255, 0) 58%),
    linear-gradient(135deg, rgba(240, 239, 248, 0.94), rgba(231, 228, 244, 0.9));
  border: 1px solid rgba(255, 255, 255, 0.75);
}

.canvas-image-placeholder-shimmer {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, rgba(255, 255, 255, 0.16), rgba(123, 103, 209, 0.12));
  animation: canvas-placeholder-shimmer 1.7s ease-in-out infinite;
}

.canvas-image-placeholder-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #7b67d1;
}

.canvas-image-placeholder-spinner {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 2px solid rgba(123, 103, 209, 0.18);
  border-top-color: #7b67d1;
  animation: canvas-placeholder-spin 0.8s linear infinite;
}

.canvas-image-content-enter-active,
.canvas-image-content-leave-active {
  transition: opacity 0.28s ease, transform 0.28s ease;
}

.canvas-image-content-enter-from,
.canvas-image-content-leave-to {
  opacity: 0;
  transform: scale(0.985);
}

@keyframes canvas-placeholder-shimmer {
  0%, 100% { opacity: 0.34; }
  50% { opacity: 0.82; }
}

@keyframes canvas-placeholder-spin {
  100% { transform: rotate(360deg); }
}
</style>
