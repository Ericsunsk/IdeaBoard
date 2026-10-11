<template>
  <CanvasMiniMap
    v-if="enabled"
    class="canvas-chrome-region canvas-chrome-region--minimap"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
    @center-requested="emit('center-requested', $event)"
    @drag-start="emit('drag-start')"
    @drag-move="emit('drag-move', $event)"
    @drag-end="emit('drag-end')"
  />
</template>

<script setup lang="ts">
import CanvasMiniMap from '../CanvasMiniMap.vue'

type MiniMapPoint = {
  x: number
  y: number
}

defineProps<{
  enabled: boolean
  inert: boolean
  isHidden: boolean
}>()

const emit = defineEmits<{
  'center-requested': [payload: { position: MiniMapPoint }]
  'drag-start': []
  'drag-move': [payload: { delta: MiniMapPoint }]
  'drag-end': []
}>()
</script>
