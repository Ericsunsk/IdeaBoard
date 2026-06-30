<template>
  <MiniMap
    class="canvas-minimap"
    position="bottom-left"
    :style="minimapSizeStyle"
    :width="CANVAS_MINIMAP_WIDTH"
    :height="CANVAS_MINIMAP_HEIGHT"
    node-color="var(--canvas-minimap-node)"
    node-stroke-color="transparent"
    :node-stroke-width="0"
    :node-border-radius="1.5"
    mask-color="transparent"
    mask-stroke-color="var(--canvas-minimap-viewport)"
    :mask-stroke-width="1.2"
    :mask-border-radius="3"
    :offset-scale="CANVAS_MINIMAP_OFFSET_SCALE"
    :pannable="false"
    :zoomable="false"
    aria-label="画布小地图"
  />
  <Panel
    v-if="viewportFillStyle"
    position="bottom-left"
    class="canvas-minimap-viewport-panel"
    :style="minimapSizeStyle"
    aria-hidden="true"
  >
    <div class="canvas-minimap-viewport-plane">
      <div class="canvas-minimap-viewport-fill" :style="viewportFillStyle" />
    </div>
  </Panel>
  <Panel
    position="bottom-left"
    class="canvas-minimap-interaction-panel"
    :style="minimapSizeStyle"
  >
    <div
      class="canvas-minimap-interaction-plane"
      role="application"
      aria-label="拖拽小地图视口"
      @pointerdown="onMinimapPointerDown"
      @pointermove="onMinimapPointerMove"
      @pointerup="onMinimapPointerUp"
      @pointercancel="onMinimapPointerUp"
    />
  </Panel>
</template>

<script setup lang="ts">
import type { Rect } from '@vue-flow/core'
import { getRectOfNodes, Panel, useVueFlow } from '@vue-flow/core'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/minimap/dist/style.css'
import {
  resolveCanvasMiniMapGeometry,
  resolveCanvasMiniMapViewportForPoint,
} from '../utils/canvasMiniMapGeometry'

type CanvasMiniMapViewport = { x: number; y: number; zoom: number }

const emit = defineEmits<{
  'viewport-drag-start': []
  'viewport-drag': [viewport: CanvasMiniMapViewport]
  'viewport-drag-end': []
}>()

const CANVAS_MINIMAP_WIDTH = 188
const CANVAS_MINIMAP_HEIGHT = 124
const CANVAS_MINIMAP_OFFSET_SCALE = 4

const minimapSizeStyle = {
  '--canvas-minimap-width': `${CANVAS_MINIMAP_WIDTH}px`,
  '--canvas-minimap-height': `${CANVAS_MINIMAP_HEIGHT}px`,
}

const {
  viewport: flowViewport,
  dimensions: flowDimensions,
  getNodesInitialized,
  setViewport,
} = useVueFlow()

const minimapGeometry = computed(() => {
  const viewportBounds = resolveViewportBounds()
  if (!viewportBounds) return null

  const visibleNodes = getNodesInitialized.value.filter((node) => !node.hidden)
  return resolveCanvasMiniMapGeometry({
    viewportBounds,
    nodeBounds: visibleNodes.length ? getRectOfNodes(visibleNodes) : null,
    width: CANVAS_MINIMAP_WIDTH,
    height: CANVAS_MINIMAP_HEIGHT,
    offsetScale: CANVAS_MINIMAP_OFFSET_SCALE,
  })
})

const viewportFillStyle = computed(() => {
  const viewportRect = minimapGeometry.value?.viewportRect
  if (!viewportRect) return null

  return {
    width: `${viewportRect.width}px`,
    height: `${viewportRect.height}px`,
    transform: `translate3d(${viewportRect.x}px, ${viewportRect.y}px, 0)`,
  }
})

let activePointerId: number | null = null
let dragCenterOffset = { x: 0, y: 0 }
let pendingViewport: CanvasMiniMapViewport | null = null
let viewportFrameId: number | null = null

function resolveViewportBounds(): Rect | null {
  const viewport = flowViewport.value
  const dimensions = flowDimensions.value

  if (
    !Number.isFinite(viewport.x) ||
    !Number.isFinite(viewport.y) ||
    !Number.isFinite(viewport.zoom) ||
    !Number.isFinite(dimensions.width) ||
    !Number.isFinite(dimensions.height) ||
    dimensions.width <= 0 ||
    dimensions.height <= 0 ||
    viewport.zoom <= 0
  ) {
    return null
  }

  return {
    x: -viewport.x / viewport.zoom,
    y: -viewport.y / viewport.zoom,
    width: dimensions.width / viewport.zoom,
    height: dimensions.height / viewport.zoom,
  }
}

function onMinimapPointerDown(event: PointerEvent) {
  const target = event.currentTarget
  if (!(target instanceof HTMLElement) || activePointerId !== null) return

  activePointerId = event.pointerId
  dragCenterOffset = resolveDragCenterOffset(event, target)
  target.setPointerCapture(event.pointerId)
  emit('viewport-drag-start')
  queueViewportForPointer(event, target)
}

function onMinimapPointerMove(event: PointerEvent) {
  if (event.pointerId !== activePointerId) return
  const target = event.currentTarget
  if (!(target instanceof HTMLElement)) return

  queueViewportForPointer(event, target)
}

function onMinimapPointerUp(event: PointerEvent) {
  if (event.pointerId !== activePointerId) return
  const target = event.currentTarget
  if (target instanceof HTMLElement && target.hasPointerCapture(event.pointerId)) {
    target.releasePointerCapture(event.pointerId)
  }
  activePointerId = null
  dragCenterOffset = { x: 0, y: 0 }
  flushPendingViewport()
  emit('viewport-drag-end')
}

function queueViewportForPointer(event: PointerEvent, target: HTMLElement) {
  const nextViewport = resolveViewportForPointer(event, target)
  if (!nextViewport) return

  pendingViewport = nextViewport
  if (viewportFrameId !== null) return

  viewportFrameId = requestAnimationFrame(() => {
    viewportFrameId = null
    flushPendingViewport()
  })
}

function resolveViewportForPointer(event: PointerEvent, target: HTMLElement) {
  const viewportBounds = resolveViewportBounds()
  const viewBox = minimapGeometry.value?.viewBox
  if (!viewportBounds || !viewBox) return null

  const rect = target.getBoundingClientRect()
  return resolveCanvasMiniMapViewportForPoint({
    point: {
      x: clamp(event.clientX - rect.left + dragCenterOffset.x, 0, rect.width),
      y: clamp(event.clientY - rect.top + dragCenterOffset.y, 0, rect.height),
    },
    viewportBounds,
    viewBox,
    width: rect.width,
    height: rect.height,
    zoom: flowViewport.value.zoom,
  })
}

function flushPendingViewport() {
  const viewport = pendingViewport
  pendingViewport = null
  if (!viewport) return

  void setViewport(viewport, { duration: 0 })
  emit('viewport-drag', viewport)
}

function resolveDragCenterOffset(event: PointerEvent, target: HTMLElement) {
  const viewportRect = minimapGeometry.value?.viewportRect
  if (!viewportRect) return { x: 0, y: 0 }

  const rect = target.getBoundingClientRect()
  const pointer = {
    x: event.clientX - rect.left,
    y: event.clientY - rect.top,
  }
  if (
    pointer.x < viewportRect.x ||
    pointer.x > viewportRect.x + viewportRect.width ||
    pointer.y < viewportRect.y ||
    pointer.y > viewportRect.y + viewportRect.height
  ) {
    return { x: 0, y: 0 }
  }

  return {
    x: viewportRect.x + viewportRect.width / 2 - pointer.x,
    y: viewportRect.y + viewportRect.height / 2 - pointer.y,
  }
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max)
}

onBeforeUnmount(() => {
  if (viewportFrameId !== null) cancelAnimationFrame(viewportFrameId)
})
</script>

<style scoped>
:global(.vue-flow__minimap.canvas-minimap) {
  margin: 0 0 24px 24px;
  padding: 4px;
  overflow: hidden;
  z-index: 6;
  border: 0;
  border-radius: 6px;
  background: transparent;
  box-shadow: none;
}

:global(.vue-flow__minimap.canvas-minimap svg) {
  display: block;
}

:global(.vue-flow__minimap.canvas-minimap .vue-flow__minimap-node) {
  fill: var(--canvas-minimap-node);
  stroke: none;
}

:global(.vue-flow__minimap.canvas-minimap .vue-flow__minimap-mask) {
  stroke: var(--canvas-minimap-viewport);
}

:global(.canvas-minimap-viewport-panel) {
  margin: 0 0 24px 24px;
  padding: 4px;
  pointer-events: none;
  z-index: 5;
  overflow: hidden;
  border: 1px solid var(--canvas-surface-border);
  border-radius: 6px;
  background: rgb(255 255 255 / 72%);
}

:global(.canvas-minimap-interaction-panel) {
  margin: 0 0 24px 24px;
  padding: 4px;
  z-index: 7;
}

.canvas-minimap-viewport-plane {
  position: relative;
  width: var(--canvas-minimap-width);
  height: var(--canvas-minimap-height);
  overflow: hidden;
}

.canvas-minimap-viewport-fill {
  position: absolute;
  top: 0;
  left: 0;
  border-radius: 3px;
  background: var(--canvas-minimap-viewport-fill);
  will-change: transform;
}

.canvas-minimap-interaction-plane {
  width: var(--canvas-minimap-width);
  height: var(--canvas-minimap-height);
  cursor: grab;
  touch-action: none;
}

.canvas-minimap-interaction-plane:active {
  cursor: grabbing;
}
</style>
