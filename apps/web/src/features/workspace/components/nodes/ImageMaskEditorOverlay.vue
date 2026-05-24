<template>
  <div
    class="image-mask-editor-overlay"
    :class="{ 'is-image-layer-hidden': !imageLayerVisible }"
    data-image-mask-editor
  >
    <canvas
      ref="canvasRef"
      class="image-mask-editor-canvas"
      :style="{ opacity: maskLayerOpacity / 100 }"
      :width="canvasWidth"
      :height="canvasHeight"
      @pointerdown="emit('canvas-pointerdown', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  canvasWidth: number
  canvasHeight: number
  maskLayerOpacity: number
  imageLayerVisible: boolean
  setCanvasRef: (element: Element | null) => void
}>()

const emit = defineEmits<{
  (e: 'canvas-pointerdown', event: PointerEvent): void
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)

watch(
  canvasRef,
  (canvas, prev) => {
    if (prev) props.setCanvasRef(null)
    props.setCanvasRef(canvas)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  props.setCanvasRef(null)
})
</script>

<style scoped>
.image-mask-editor-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: auto;
  overflow: visible;
}

.image-mask-editor-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border-radius: 4px;
  touch-action: none;
  cursor: crosshair;
}

:global(.image-node-stage:has(.image-mask-editor-overlay.is-image-layer-hidden)
    .image-node-stage-image) {
  opacity: 0;
}
</style>
