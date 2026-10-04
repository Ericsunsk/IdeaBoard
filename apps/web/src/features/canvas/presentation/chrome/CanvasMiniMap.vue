<template>
  <div class="canvas-minimap-shell" :style="minimapShellStyle">
    <MiniMap
      class="ib-page-chrome-surface canvas-minimap"
      position="bottom-left"
      :width="CANVAS_MINIMAP_WIDTH"
      :height="CANVAS_MINIMAP_HEIGHT"
      node-color="var(--canvas-minimap-node)"
      node-stroke-color="transparent"
      :node-stroke-width="0"
      :node-border-radius="1.5"
      mask-color="transparent"
      mask-stroke-color="transparent"
      :mask-stroke-width="0"
      :mask-border-radius="CANVAS_MINIMAP_VIEWPORT_RADIUS"
      :offset-scale="CANVAS_MINIMAP_OFFSET_SCALE"
      :pannable="true"
      :zoomable="true"
      :inverse-pan="false"
      :zoom-step="1"
      :aria-label="t('canvas.minimap.label')"
      @click="onMiniMapClick"
    />
    <svg
      v-if="viewportOverlay"
      ref="viewportOverlaySvg"
      class="canvas-minimap-viewport-overlay"
      :width="CANVAS_MINIMAP_WIDTH"
      :height="CANVAS_MINIMAP_HEIGHT"
      :viewBox="viewportOverlay.viewBox"
      aria-hidden="true"
    >
      <rect
        class="canvas-minimap-viewport-fill"
        :class="{ 'is-dragging': isViewportOverlayDragging }"
        :x="viewportOverlay.x"
        :y="viewportOverlay.y"
        :width="viewportOverlay.width"
        :height="viewportOverlay.height"
        :rx="CANVAS_MINIMAP_VIEWPORT_RADIUS"
        :ry="CANVAS_MINIMAP_VIEWPORT_RADIUS"
        @click.stop="onViewportOverlayClick"
        @pointerdown.stop="onViewportOverlayPointerDown"
      />
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getBoundsofRects, getRectOfNodes, useVueFlow } from '@vue-flow/core'
import { MiniMap } from '@vue-flow/minimap'
import { usePointerDragSession } from '../../infrastructure/browser/interaction/usePointerDragSession.ts'

const emit = defineEmits<{
  'center-requested': [payload: { position: { x: number; y: number } }]
  'drag-start': []
  'drag-move': [payload: { delta: MiniMapPoint }]
  'drag-end': []
}>()

const CANVAS_MINIMAP_WIDTH = 188
const CANVAS_MINIMAP_HEIGHT = 124
const CANVAS_MINIMAP_OFFSET_SCALE = 4
const CANVAS_MINIMAP_VIEWPORT_RADIUS = 3
const CANVAS_MINIMAP_FRAME_INSET = 5
const CANVAS_MINIMAP_DRAG_START_DISTANCE = 3

const minimapShellStyle = {
  width: `${CANVAS_MINIMAP_WIDTH + CANVAS_MINIMAP_FRAME_INSET * 2}px`,
  height: `${CANVAS_MINIMAP_HEIGHT + CANVAS_MINIMAP_FRAME_INSET * 2}px`,
}

type MiniMapPoint = {
  x: number
  y: number
}

type MiniMapRect = MiniMapPoint & {
  width: number
  height: number
}

type MiniMapViewportOverlay = MiniMapRect & {
  viewBox: string
  viewBoxRect: MiniMapRect
}

type MiniMapOverlayDragPayload = {
  startClientX: number
  startClientY: number
  lastClientX: number
  lastClientY: number
  deltaScaleX: number
  deltaScaleY: number
  started: boolean
}

const { t } = useI18n()
const { dimensions, getNodesInitialized, viewport } = useVueFlow()
const viewportOverlaySvg = ref<SVGSVGElement | null>(null)
const dragViewBoxOverride = ref<MiniMapRect | null>(null)
let activeOverlayDragPayload: MiniMapOverlayDragPayload | null = null
let suppressNextOverlayClick = false
// This component only runs in the browser, where timer handles are numeric.
let resetClickSuppressionId: number | null = null

const viewportOverlay = computed<MiniMapViewportOverlay | null>(() => {
  const zoom = viewport.value.zoom
  if (!Number.isFinite(zoom) || zoom <= 0) return null

  const viewBounds = {
    x: -viewport.value.x / zoom,
    y: -viewport.value.y / zoom,
    width: dimensions.value.width / zoom,
    height: dimensions.value.height / zoom,
  }
  const initializedNodes = getNodesInitialized.value.filter((node) => !node.hidden)
  const nodeBounds = initializedNodes.length ? getRectOfNodes(initializedNodes) : viewBounds
  const visibleBounds = initializedNodes.length
    ? getBoundsofRects(nodeBounds, viewBounds)
    : viewBounds
  const viewBoxRect = dragViewBoxOverride.value ?? resolveMiniMapViewBoxRect(visibleBounds)

  if (!viewBoxRect) return null

  return {
    x: viewBounds.x,
    y: viewBounds.y,
    width: Math.max(0, viewBounds.width),
    height: Math.max(0, viewBounds.height),
    viewBox: formatMiniMapViewBox(viewBoxRect),
    viewBoxRect,
  }
})

const viewportOverlayDrag = usePointerDragSession<MiniMapOverlayDragPayload>({
  onMove: ({ event, payload }) => {
    const dragDistance = Math.hypot(
      event.clientX - payload.startClientX,
      event.clientY - payload.startClientY,
    )
    if (!payload.started && dragDistance < CANVAS_MINIMAP_DRAG_START_DISTANCE) return

    event.preventDefault()
    if (!payload.started) {
      payload.started = true
      emit('drag-start')
    }

    const delta = {
      x: (event.clientX - payload.lastClientX) * payload.deltaScaleX,
      y: (event.clientY - payload.lastClientY) * payload.deltaScaleY,
    }
    payload.lastClientX = event.clientX
    payload.lastClientY = event.clientY

    if (!delta.x && !delta.y) return
    emit('drag-move', { delta })
  },
  onEnd: ({ payload }) => {
    finishViewportOverlayDrag(payload)
  },
  onCancel: ({ payload }) => {
    finishViewportOverlayDrag(payload)
  },
})
const isViewportOverlayDragging = viewportOverlayDrag.isDragging

onBeforeUnmount(() => {
  const activePayload = activeOverlayDragPayload
  viewportOverlayDrag.cancelDrag()
  if (activePayload) finishViewportOverlayDrag(activePayload, { releaseViewBox: false })
  if (resetClickSuppressionId !== null) window.clearTimeout(resetClickSuppressionId)
})

function resolveMiniMapViewBoxRect(bounds: MiniMapRect): MiniMapRect | null {
  const scaledWidth = bounds.width / CANVAS_MINIMAP_WIDTH
  const scaledHeight = bounds.height / CANVAS_MINIMAP_HEIGHT
  const viewScale = Math.max(scaledWidth, scaledHeight)
  const viewWidth = viewScale * CANVAS_MINIMAP_WIDTH
  const viewHeight = viewScale * CANVAS_MINIMAP_HEIGHT
  const offset = CANVAS_MINIMAP_OFFSET_SCALE * viewScale
  const rect = {
    x: bounds.x - (viewWidth - bounds.width) / 2 - offset,
    y: bounds.y - (viewHeight - bounds.height) / 2 - offset,
    width: viewWidth + offset * 2,
    height: viewHeight + offset * 2,
  }

  if (![rect.x, rect.y, rect.width, rect.height].every(Number.isFinite)) return null
  return rect
}

function formatMiniMapViewBox(rect: MiniMapRect) {
  return `${rect.x} ${rect.y} ${rect.width} ${rect.height}`
}

function onMiniMapClick(params: { event: MouseEvent; position: { x: number; y: number } }) {
  if (params.event.defaultPrevented) return
  emit('center-requested', { position: params.position })
}

function onViewportOverlayPointerDown(event: PointerEvent) {
  if (event.button !== 0) return

  const overlay = viewportOverlay.value
  if (!overlay) return

  const deltaScale = resolveOverlayDeltaScale(overlay.viewBoxRect)
  if (!deltaScale) return

  dragViewBoxOverride.value = { ...overlay.viewBoxRect }
  const payload = {
    startClientX: event.clientX,
    startClientY: event.clientY,
    lastClientX: event.clientX,
    lastClientY: event.clientY,
    deltaScaleX: deltaScale.x,
    deltaScaleY: deltaScale.y,
    started: false,
  }
  activeOverlayDragPayload = payload
  viewportOverlayDrag.beginDrag(event, payload, { target: event.currentTarget })
}

function onViewportOverlayClick(event: MouseEvent) {
  if (event.defaultPrevented) return
  if (suppressNextOverlayClick) {
    suppressNextOverlayClick = false
    return
  }

  const position = resolveOverlayPoint(event)
  if (!position) return
  emit('center-requested', { position })
}

function finishViewportOverlayDrag(
  payload: MiniMapOverlayDragPayload,
  options: { releaseViewBox?: boolean } = {},
) {
  if (activeOverlayDragPayload === payload) activeOverlayDragPayload = null
  if (!payload.started) {
    dragViewBoxOverride.value = null
    return
  }

  suppressOverlayClick()
  emit('drag-end')
  if (options.releaseViewBox !== false) releaseDragViewBoxOverride()
}

function suppressOverlayClick() {
  suppressNextOverlayClick = true
  if (resetClickSuppressionId !== null) window.clearTimeout(resetClickSuppressionId)
  resetClickSuppressionId = window.setTimeout(() => {
    suppressNextOverlayClick = false
    resetClickSuppressionId = null
  })
}

function releaseDragViewBoxOverride() {
  window.requestAnimationFrame(() => {
    dragViewBoxOverride.value = null
  })
}

function resolveOverlayPoint(event: MouseEvent | PointerEvent): MiniMapPoint | null {
  const overlay = viewportOverlay.value
  if (!overlay) return null

  const svgRect = viewportOverlaySvg.value?.getBoundingClientRect()
  if (!svgRect || svgRect.width <= 0 || svgRect.height <= 0) return null

  return {
    x:
      overlay.viewBoxRect.x +
      ((event.clientX - svgRect.left) / svgRect.width) * overlay.viewBoxRect.width,
    y:
      overlay.viewBoxRect.y +
      ((event.clientY - svgRect.top) / svgRect.height) * overlay.viewBoxRect.height,
  }
}

function resolveOverlayDeltaScale(viewBoxRect: MiniMapRect): MiniMapPoint | null {
  const svgRect = viewportOverlaySvg.value?.getBoundingClientRect()
  if (!svgRect || svgRect.width <= 0 || svgRect.height <= 0) return null

  return {
    x: viewBoxRect.width / svgRect.width,
    y: viewBoxRect.height / svgRect.height,
  }
}
</script>

<style scoped>
.canvas-minimap-shell {
  pointer-events: auto;
}

.canvas-minimap-viewport-overlay {
  position: absolute;
  inset: 5px;
  z-index: 6;
  pointer-events: none;
}

.canvas-minimap-viewport-fill {
  fill: var(--canvas-minimap-mask);
  stroke: var(--canvas-minimap-viewport);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
  cursor: grab;
  pointer-events: auto;
  touch-action: none;
}

.canvas-minimap-viewport-fill.is-dragging {
  cursor: grabbing;
}
</style>
