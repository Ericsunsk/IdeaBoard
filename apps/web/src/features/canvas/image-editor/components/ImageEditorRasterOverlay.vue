<template>
  <div
    class="image-editor-raster-overlay"
    :class="{ 'is-enabled': rasterToolEnabled }"
    data-image-editor
    data-image-editor-raster-overlay
  >
    <canvas
      ref="canvasRef"
      class="image-editor-raster-overlay__canvas"
      :width="canvasWidth"
      :height="canvasHeight"
      @pointerenter="onCanvasPointerMove"
      @pointermove="onCanvasPointerMove"
      @pointerleave="hideBrushCursor"
      @pointercancel="hideBrushCursor"
      @pointerdown="onCanvasPointerDown"
    />
    <span
      class="image-editor-raster-overlay__cursor"
      :class="{ 'is-visible': brushCursorVisible }"
      :style="brushCursorStyle"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  canvasWidth: number
  canvasHeight: number
  brushSize: number
  rasterToolEnabled: boolean
  setCanvasRef: (element: Element | null) => void
}>()

const emit = defineEmits<{
  (e: 'canvas-pointerdown', event: PointerEvent): void
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const brushCursorVisible = ref(false)
const brushCursorX = ref(0)
const brushCursorY = ref(0)

const brushCursorDiameter = computed(() => {
  const canvas = canvasRef.value
  const localWidth = canvas?.offsetWidth || props.canvasWidth
  const scale = props.canvasWidth > 0 ? localWidth / props.canvasWidth : 1
  return Math.max(3, props.brushSize * scale)
})
const brushCursorStyle = computed(() => ({
  width: `${brushCursorDiameter.value}px`,
  height: `${brushCursorDiameter.value}px`,
  transform: `translate3d(${brushCursorX.value}px, ${brushCursorY.value}px, 0) translate(-50%, -50%)`,
}))

function updateBrushCursor(event: PointerEvent) {
  if (!props.rasterToolEnabled || event.pointerType === 'touch') {
    hideBrushCursor()
    return
  }

  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return
  const x = event.clientX - rect.left
  const y = event.clientY - rect.top
  brushCursorX.value = x
  brushCursorY.value = y
  brushCursorVisible.value = x >= 0 && y >= 0 && x <= rect.width && y <= rect.height
}

function hideBrushCursor() {
  brushCursorVisible.value = false
}

function onCanvasPointerMove(event: PointerEvent) {
  updateBrushCursor(event)
}

function onCanvasPointerDown(event: PointerEvent) {
  updateBrushCursor(event)
  emit('canvas-pointerdown', event)
}

watch(
  canvasRef,
  (canvas, prev) => {
    if (prev) props.setCanvasRef(null)
    props.setCanvasRef(canvas)
  },
  { immediate: true },
)

watch(
  () => [props.canvasWidth, props.canvasHeight] as const,
  async () => {
    await nextTick()
    if (canvasRef.value) props.setCanvasRef(canvasRef.value)
  },
)

watch(
  () => props.rasterToolEnabled,
  (enabled) => {
    if (!enabled) hideBrushCursor()
  },
)

onBeforeUnmount(() => {
  props.setCanvasRef(null)
})
</script>

<style scoped>
.image-editor-raster-overlay {
  position: absolute;
  z-index: 3;
  overflow: visible;
  pointer-events: none;
}

.image-editor-raster-overlay__canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border-radius: 4px;
  pointer-events: none;
  touch-action: none;
}

.image-editor-raster-overlay.is-enabled,
.image-editor-raster-overlay.is-enabled .image-editor-raster-overlay__canvas {
  pointer-events: auto;
}

.image-editor-raster-overlay.is-enabled .image-editor-raster-overlay__canvas {
  cursor: none;
}

.image-editor-raster-overlay__cursor {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 4;
  border: 1px solid rgba(18, 24, 38, 0.88);
  border-radius: 999px;
  box-shadow:
    0 0 0 1px rgba(255, 255, 255, 0.78),
    0 1px 4px rgba(18, 24, 38, 0.22);
  opacity: 0;
  pointer-events: none;
  transition: opacity 80ms ease;
  will-change: transform, width, height;
}

.image-editor-raster-overlay__cursor.is-visible {
  opacity: 1;
}
</style>
