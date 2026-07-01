<template>
  <div class="canvas-minimap-shell" :style="minimapShellStyle">
    <MiniMap
      class="canvas-minimap"
      position="bottom-left"
      :width="CANVAS_MINIMAP_WIDTH"
      :height="CANVAS_MINIMAP_HEIGHT"
      node-color="var(--canvas-minimap-node)"
      node-stroke-color="transparent"
      :node-stroke-width="0"
      :node-border-radius="1.5"
      mask-color="transparent"
      mask-stroke-color="transparent"
      :mask-stroke-width="1.2"
      :mask-border-radius="CANVAS_MINIMAP_VIEWPORT_RADIUS"
      :offset-scale="CANVAS_MINIMAP_OFFSET_SCALE"
      :pannable="true"
      :zoomable="true"
      :inverse-pan="false"
      :zoom-step="1"
      aria-label="画布小地图"
      @click="onMiniMapClick"
    />
    <svg
      v-if="viewportOverlay"
      class="canvas-minimap-viewport-overlay"
      :width="CANVAS_MINIMAP_WIDTH"
      :height="CANVAS_MINIMAP_HEIGHT"
      :viewBox="viewportOverlay.viewBox"
      aria-hidden="true"
    >
      <rect
        class="canvas-minimap-viewport-fill"
        :x="viewportOverlay.x"
        :y="viewportOverlay.y"
        :width="viewportOverlay.width"
        :height="viewportOverlay.height"
        :rx="CANVAS_MINIMAP_VIEWPORT_RADIUS"
        :ry="CANVAS_MINIMAP_VIEWPORT_RADIUS"
      />
    </svg>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { getBoundsofRects, getRectOfNodes, useVueFlow } from '@vue-flow/core'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/minimap/dist/style.css'

const emit = defineEmits<{
  'center-requested': [payload: { position: { x: number; y: number } }]
}>()

const CANVAS_MINIMAP_WIDTH = 188
const CANVAS_MINIMAP_HEIGHT = 124
const CANVAS_MINIMAP_OFFSET_SCALE = 4
const CANVAS_MINIMAP_VIEWPORT_RADIUS = 3
const CANVAS_MINIMAP_FRAME_INSET = 5

const minimapShellStyle = {
  width: `${CANVAS_MINIMAP_WIDTH + CANVAS_MINIMAP_FRAME_INSET * 2}px`,
  height: `${CANVAS_MINIMAP_HEIGHT + CANVAS_MINIMAP_FRAME_INSET * 2}px`,
}

type MiniMapRect = {
  x: number
  y: number
  width: number
  height: number
}

const { dimensions, getNodesInitialized, viewport } = useVueFlow()

const viewportOverlay = computed(() => {
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
  const visibleBounds = initializedNodes.length ? getBoundsofRects(nodeBounds, viewBounds) : viewBounds
  const viewBox = resolveMiniMapViewBox(visibleBounds)

  if (!viewBox) return null

  return {
    x: viewBounds.x,
    y: viewBounds.y,
    width: Math.max(0, viewBounds.width),
    height: Math.max(0, viewBounds.height),
    viewBox,
  }
})

function resolveMiniMapViewBox(bounds: MiniMapRect): string | null {
  const scaledWidth = bounds.width / CANVAS_MINIMAP_WIDTH
  const scaledHeight = bounds.height / CANVAS_MINIMAP_HEIGHT
  const viewScale = Math.max(scaledWidth, scaledHeight)
  const viewWidth = viewScale * CANVAS_MINIMAP_WIDTH
  const viewHeight = viewScale * CANVAS_MINIMAP_HEIGHT
  const offset = CANVAS_MINIMAP_OFFSET_SCALE * viewScale
  const x = bounds.x - (viewWidth - bounds.width) / 2 - offset
  const y = bounds.y - (viewHeight - bounds.height) / 2 - offset
  const width = viewWidth + offset * 2
  const height = viewHeight + offset * 2

  if (![x, y, width, height].every(Number.isFinite)) return null
  return `${x} ${y} ${width} ${height}`
}

function onMiniMapClick(params: { event: MouseEvent; position: { x: number; y: number } }) {
  if (params.event.defaultPrevented) return
  emit('center-requested', { position: params.position })
}
</script>

<style scoped>
.canvas-minimap-shell {
  position: absolute;
  bottom: 24px;
  left: 24px;
  z-index: 6;
}

:global(.vue-flow__minimap.canvas-minimap) {
  position: static;
  margin: 0;
  padding: 4px;
  overflow: hidden;
  border: 1px solid var(--canvas-surface-border);
  border-radius: 6px;
  background: rgb(255 255 255 / 72%);
  box-shadow: none;
}

:global(.vue-flow__minimap.canvas-minimap svg) {
  display: block;
  touch-action: none;
}

:global(.vue-flow__minimap.canvas-minimap .vue-flow__minimap-node) {
  fill: var(--canvas-minimap-node);
  stroke: none;
}

:global(.vue-flow__minimap.canvas-minimap .vue-flow__minimap-mask) {
  stroke: var(--canvas-minimap-viewport);
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
  stroke-width: 1.2;
}

:global(.vue-flow__minimap.canvas-minimap.pannable:active) {
  cursor: grabbing;
}
</style>
