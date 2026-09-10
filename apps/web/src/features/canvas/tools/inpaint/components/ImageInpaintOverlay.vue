<template>
  <div
    class="inpaint-overlay"
    :class="{ 'is-image-layer-hidden': !imageLayerVisible }"
    data-inpaint
  >
    <canvas
      ref="canvasRef"
      class="inpaint-canvas"
      :style="{ opacity: maskLayerOpacity / 100 }"
      :width="canvasWidth"
      :height="canvasHeight"
      @pointerenter="onCanvasPointerMove"
      @pointermove="onCanvasPointerMove"
      @pointerleave="hideBrushCursor"
      @pointercancel="hideBrushCursor"
      @pointerdown="onCanvasPointerDown"
    />
    <Circle
      class="inpaint-brush-cursor"
      :class="{ 'is-visible': brushCursorVisible }"
      :style="brushCursorStyle"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Circle } from '@iconoir/vue'
import { clampMaskBrushSize } from '../config/constants'

const props = defineProps<{
  canvasWidth: number
  canvasHeight: number
  maskLayerOpacity: number
  imageLayerVisible: boolean
  brushSize: number
  setCanvasRef: (element: Element | null) => void
}>()

const emit = defineEmits<{
  (e: 'canvas-pointerdown', event: PointerEvent): void
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const brushCursorVisible = ref(false)
const brushCursorX = ref(0)
const brushCursorY = ref(0)

const ICONOIR_CIRCLE_PATH_DIAMETER_RATIO = 20 / 24
const brushCursorDiameter = computed(() => {
  const brushSize = clampMaskBrushSize(props.brushSize)
  return brushSize / ICONOIR_CIRCLE_PATH_DIAMETER_RATIO
})
const brushCursorStyle = computed(() => ({
  width: `${brushCursorDiameter.value}px`,
  height: `${brushCursorDiameter.value}px`,
  transform: `translate3d(${brushCursorX.value}px, ${brushCursorY.value}px, 0) translate(-50%, -50%)`,
}))

function updateBrushCursor(event: PointerEvent) {
  if (event.pointerType === 'touch') {
    hideBrushCursor()
    return
  }

  const canvas = canvasRef.value
  if (!canvas) return
  const localWidth = canvas.offsetWidth || props.canvasWidth
  const localHeight = canvas.offsetHeight || props.canvasHeight
  const cursorPoint = resolveBrushCursorPoint(event, canvas, localWidth, localHeight)
  if (!cursorPoint) return

  brushCursorX.value = cursorPoint.x
  brushCursorY.value = cursorPoint.y
  brushCursorVisible.value =
    brushCursorX.value >= 0 &&
    brushCursorY.value >= 0 &&
    brushCursorX.value <= localWidth &&
    brushCursorY.value <= localHeight
}

function resolveBrushCursorPoint(
  event: PointerEvent,
  canvas: HTMLCanvasElement,
  localWidth: number,
  localHeight: number,
) {
  if (Number.isFinite(event.offsetX) && Number.isFinite(event.offsetY)) {
    return {
      x: event.offsetX,
      y: event.offsetY,
    }
  }

  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return null
  return {
    x: ((event.clientX - rect.left) / rect.width) * localWidth,
    y: ((event.clientY - rect.top) / rect.height) * localHeight,
  }
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

onBeforeUnmount(() => {
  props.setCanvasRef(null)
})
</script>

<style scoped>
.inpaint-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: auto;
  overflow: visible;
}

.inpaint-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border-radius: 4px;
  touch-action: none;
  cursor: none;
}

.inpaint-brush-cursor {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 4;
  color: rgb(122, 107, 213);
  opacity: 0;
  pointer-events: none;
  stroke-width: 0.8;
  transition: opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
  will-change: transform, width, height;
}

.inpaint-brush-cursor.is-visible {
  opacity: 1;
}
</style>
