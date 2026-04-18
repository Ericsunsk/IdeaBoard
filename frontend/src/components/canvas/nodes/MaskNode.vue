<template>
  <div
    class="workflow-node mask-node"
    data-node-kind="mask"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Mask"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <path d="M4 7a3 3 0 0 1 3-3h6l7 7v6a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3z" />
          <path d="M13 4v6h6" />
        </svg>
      </template>
    </NodeHeader>

    <div class="mask-node-body" @mousedown.stop @contextmenu.stop>
      <div
        class="mask-stage"
        :data-mask-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
        @contextmenu.prevent.stop="togglePaintPopup"
      >
        <template v-if="effectiveSourceImageUrl">
          <img :src="effectiveSourceImageUrl" class="mask-stage-image" />
          <canvas
            v-if="!paintPopupOpen"
            :ref="onCanvasRef"
            class="mask-stage-canvas"
            :style="maskCanvasCursorStyle"
            :width="canvasWidth"
            :height="stageHeight"
            @mousedown.stop.prevent="onCanvasMouseDown"
            @mouseenter="onCanvasMouseEnter"
            @mousemove="onCanvasMouseMove"
            @mouseleave="hideMaskCursor"
          />
          <div v-if="node.sourceImageUrl && !hasConnectedImageInput" class="mask-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('clear-image')">清空</NodeActionButton>
          </div>
        </template>
        <div v-else class="mask-stage-empty" @click="emit('open-file-picker')">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>上传图片后开始涂抹蒙版</span>
        </div>
      </div>

      <div class="mask-toolbar">
        <NodeIconButton
          :active="node.drawMode === 'paint'"
          title="Brush"
          @click="emit('set-draw-mode', 'paint')"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 19l7-7 3 3-7 7-3-3z"/>
            <path d="M18 13l-1.5-7.5L2 2l3.5 14.5L13 18l5-5z"/>
          </svg>
        </NodeIconButton>
        <NodeIconButton
          :active="node.drawMode === 'erase'"
          title="Eraser"
          @click="emit('set-draw-mode', 'erase')"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
            <path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/>
            <path d="M22 21H7"/>
          </svg>
        </NodeIconButton>
        <input
          :value="node.brushSize"
          class="mask-brush-range node-inline-range"
          :style="{ '--slider-pct': `${((node.brushSize - 4) / 44) * 100}%` }"
          type="range"
          min="4"
          max="48"
          @input="emit('update:brush-size', toNumber(($event.target as HTMLInputElement).value, node.brushSize))"
        />
      </div>

      <div class="mask-refine-row">
        <span class="mask-refine-label">Refine</span>
        <input
          :value="node.refineAmount"
          class="mask-refine-range node-inline-range"
          :style="{ '--slider-pct': `${((node.refineAmount + 100) / 200) * 100}%` }"
          type="range"
          min="-100"
          max="100"
          @input="emit('update:refine-amount', toNumber(($event.target as HTMLInputElement).value, node.refineAmount))"
        />
        <NodeActionButton
          variant="tool"
          :active="node.invertMask"
          @click="emit('toggle-invert')"
        >
          Invert
        </NodeActionButton>
        <NodeActionButton variant="tool" push-right @click="emit('clear-mask')">Clear</NodeActionButton>
      </div>

      <div
        v-if="paintPopupOpen"
        ref="popupRef"
        class="mask-paint-popup"
        @mousedown.stop
        @click.stop
        @contextmenu.prevent.stop="togglePaintPopup"
      >
        <div
          class="mask-paint-popup-stage"
          :style="{ width: `${popupStageWidth}px`, height: `${popupStageHeight}px` }"
        >
          <img :src="effectiveSourceImageUrl" class="mask-stage-image" />
          <canvas
            :ref="onCanvasRef"
            class="mask-paint-popup-canvas"
            :style="maskCanvasCursorStyle"
            :width="canvasWidth"
            :height="stageHeight"
            @mousedown.stop.prevent="onCanvasMouseDown"
            @mouseenter="onCanvasMouseEnter"
            @mousemove="onCanvasMouseMove"
            @mouseleave="hideMaskCursor"
          />
        </div>
      </div>
    </div>
    <NodePorts
      node-kind="mask"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import type { MaskNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeHeader from './NodeHeader.vue'
import NodeIconButton from './NodeIconButton.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, MASK_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: MaskNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  canvasWidth: number
  sourceImageUrl?: string
  hasConnectedImageInput?: boolean
  uiState?: NodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'open-file-picker'): void
  (e: 'clear-image'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'canvas-mousedown', event: MouseEvent): void
  (e: 'set-draw-mode', value: 'paint' | 'erase'): void
  (e: 'update:brush-size', value: number): void
  (e: 'update:refine-amount', value: number): void
  (e: 'toggle-invert'): void
  (e: 'clear-mask'): void
  (e: 'set-canvas-ref', value: Element | null): void
  (e: 'port-mousedown', event: MouseEvent, port: 'mask'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const effectiveSourceImageUrl = computed(() => props.sourceImageUrl ?? props.node.sourceImageUrl)
const ports = computed(() =>
  buildNodeIoFromSlots(MASK_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
    },
  }).ports,
)

const popupRef = ref<HTMLElement | null>(null)
const paintPopupOpen = ref(false)
const maskCursor = reactive({
  visible: false,
  scale: 1,
})

const BRUSH_SIZE_MIN = 4
const BRUSH_SIZE_MAX = 48
const BRUSH_SIZE_STEP = 2

const popupStageHeight = computed(() =>
  Math.max(280, Math.min(560, Math.round(props.stageHeight * 2))),
)

const popupStageWidth = computed(() => {
  const scale = props.stageHeight > 0 ? popupStageHeight.value / props.stageHeight : 1
  return Math.round(props.canvasWidth * scale)
})

const maskCanvasCursorStyle = computed(() => {
  const diameter = Math.max(10, Math.round(props.node.brushSize * maskCursor.scale))
  const hotspot = Math.floor(diameter / 2)
  const stroke = props.node.drawMode === 'erase' ? '#5f6877' : '#6757c8'
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="${diameter}" height="${diameter}" viewBox="0 0 ${diameter} ${diameter}">
      <circle
        cx="${diameter / 2}"
        cy="${diameter / 2}"
        r="${Math.max(2, diameter / 2 - 1.25)}"
        fill="none"
        stroke="${stroke}"
        stroke-width="1.5"
      />
    </svg>
  `.trim()
  const cursor = `url("data:image/svg+xml;utf8,${encodeURIComponent(svg)}") ${hotspot} ${hotspot}, none`
  return {
    cursor,
  }
})

function toNumber(rawValue: string, fallback: number): number {
  const value = Number(rawValue)
  return Number.isFinite(value) ? value : fallback
}

function clampBrushSize(value: number): number {
  if (!Number.isFinite(value)) return props.node.brushSize
  return Math.max(BRUSH_SIZE_MIN, Math.min(BRUSH_SIZE_MAX, Math.round(value)))
}

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'mask')
}

function updateBrushSize(value: number) {
  emit('update:brush-size', clampBrushSize(value))
}

function togglePaintPopup() {
  if (!effectiveSourceImageUrl.value) return
  hideMaskCursor()
  paintPopupOpen.value = !paintPopupOpen.value
}

function onCanvasRef(el: Element | ComponentPublicInstance | null) {
  emit('set-canvas-ref', el instanceof HTMLCanvasElement ? el : null)
}

function syncMaskCursor(event: MouseEvent) {
  const canvas = event.currentTarget instanceof HTMLCanvasElement
    ? event.currentTarget
    : null

  if (!canvas) return

  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return

  maskCursor.visible = true
  maskCursor.scale = rect.width / props.canvasWidth
}

function onCanvasMouseEnter(event: MouseEvent) {
  syncMaskCursor(event)
}

function onCanvasMouseMove(event: MouseEvent) {
  syncMaskCursor(event)
}

function hideMaskCursor() {
  maskCursor.visible = false
}

function onCanvasMouseDown(event: MouseEvent) {
  syncMaskCursor(event)
  emit('canvas-mousedown', event)
}

function isTextEditableTarget(target: EventTarget | null): boolean {
  const element = target instanceof HTMLElement ? target : null
  if (!element) return false
  if (element instanceof HTMLTextAreaElement) return true
  if (element instanceof HTMLInputElement) {
    return !['range', 'button', 'checkbox', 'radio', 'file', 'color'].includes(element.type)
  }
  return element.isContentEditable
}

function onGlobalPointerDown(event: Event) {
  if (!paintPopupOpen.value) return
  const target = event.target as Node | null
  const popupEl = popupRef.value
  if (!target || !popupEl || !popupEl.contains(target)) {
    paintPopupOpen.value = false
    hideMaskCursor()
  }
}

function onGlobalKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    paintPopupOpen.value = false
    hideMaskCursor()
    return
  }

  if (!paintPopupOpen.value && !maskCursor.visible) return
  if (isTextEditableTarget(event.target)) return

  const isDecrease =
    event.key === '[' ||
    event.key === '【' ||
    event.code === 'BracketLeft'

  const isIncrease =
    event.key === ']' ||
    event.key === '】' ||
    event.code === 'BracketRight'

  if (!isDecrease && !isIncrease) return

  event.preventDefault()
  updateBrushSize(props.node.brushSize + (isIncrease ? BRUSH_SIZE_STEP : -BRUSH_SIZE_STEP))
}

window.addEventListener('pointerdown', onGlobalPointerDown, true)
window.addEventListener('keydown', onGlobalKeyDown, true)

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onGlobalPointerDown, true)
  window.removeEventListener('keydown', onGlobalKeyDown, true)
})
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.workflow-node.mask-node {
  overflow: visible;
}

.mask-node-body {
  position: relative;
  padding: 10px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mask-stage-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  pointer-events: none;
}

.mask-stage-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0.5;
}

.mask-paint-popup {
  z-index: 30;
  width: max-content;
}

.mask-paint-popup-stage {
  position: relative;
  border: 1px solid #d8ddea;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.mask-paint-popup-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0.5;
}

.mask-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
}

.mask-refine-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.mask-refine-label {
  font-size: 11px;
  font-weight: 700;
  color: #7b8798;
  letter-spacing: 0.02em;
}

.mask-brush-range,
.mask-refine-range {
  flex: 1;
  min-width: 80px;
}

</style>
