<template>
  <MiniMap
    class="canvas-minimap"
    position="bottom-left"
    :width="CANVAS_MINIMAP_WIDTH"
    :height="CANVAS_MINIMAP_HEIGHT"
    node-color="var(--canvas-minimap-node)"
    node-stroke-color="transparent"
    :node-stroke-width="0"
    :node-border-radius="1.5"
    mask-color="var(--canvas-minimap-mask)"
    mask-stroke-color="var(--canvas-minimap-viewport)"
    :mask-stroke-width="1.2"
    :mask-border-radius="3"
    :offset-scale="CANVAS_MINIMAP_OFFSET_SCALE"
    :pannable="true"
    :zoomable="true"
    :inverse-pan="false"
    :zoom-step="1"
    aria-label="画布小地图"
    @click="onMiniMapClick"
  />
</template>

<script setup lang="ts">
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/minimap/dist/style.css'

const emit = defineEmits<{
  'center-requested': [payload: { position: { x: number; y: number } }]
}>()

const CANVAS_MINIMAP_WIDTH = 188
const CANVAS_MINIMAP_HEIGHT = 124
const CANVAS_MINIMAP_OFFSET_SCALE = 4

function onMiniMapClick(params: { event: MouseEvent; position: { x: number; y: number } }) {
  if (params.event.defaultPrevented) return
  emit('center-requested', { position: params.position })
}
</script>

<style scoped>
:global(.vue-flow__minimap.canvas-minimap) {
  margin: 0 0 24px 24px;
  padding: 4px;
  overflow: hidden;
  z-index: 6;
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

:global(.vue-flow__minimap.canvas-minimap.pannable:active) {
  cursor: grabbing;
}
</style>
