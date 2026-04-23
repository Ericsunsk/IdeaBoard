<template>
  <div
    class="canvas-item"
    :class="{ active: isActive, 'is-pending': isPending }"
    :style="{
      left: '50%',
      top: '50%',
      transform: `translate(${item.x}px, ${item.y}px)`,
      width: item.width ? `${item.width}px` : 'auto',
    }"
    @mousedown.stop="emit('item-mousedown', $event)"
    @contextmenu.prevent.stop="emit('item-contextmenu', $event)"
  >
    <HomeCanvasImageDisplay
      :item="item"
      :is-pending="isPending"
      :display-height="displayHeight"
      :set-image-ref="setImageRef"
      @image-load="emit('image-load', $event)"
    />

    <HomeCanvasImageActionMenu
      v-if="showActionMenu"
      :show-style-strength-control="showStyleStrengthControl"
      :style-strength-menu-style="styleStrengthMenuStyle"
      :direct-generating="directGenerating"
      :display-height="displayHeight"
      :style-innovate-strength-pct="styleInnovateStrengthPct"
      :set-style-strength-track-ref="setStyleStrengthTrackRef"
      @action-remove-bg="emit('action-remove-bg')"
      @style-innovate-mousedown="emit('style-innovate-mousedown', $event)"
      @style-innovate-click="emit('style-innovate-click')"
      @action-to-vector="emit('action-to-vector')"
      @action-sketch-to-design="emit('action-sketch-to-design')"
      @style-strength-track-mousedown="emit('style-strength-track-mousedown', $event)"
    />

    <template v-if="showResizeHandles">
      <div class="resize-edge n" @mousedown.stop="emitResize($event, 'n')"></div>
      <div class="resize-edge s" @mousedown.stop="emitResize($event, 's')"></div>
      <div class="resize-edge e" @mousedown.stop="emitResize($event, 'e')"></div>
      <div class="resize-edge w" @mousedown.stop="emitResize($event, 'w')"></div>
      <div class="resize-corner nw" @mousedown.stop="emitResize($event, 'nw')"></div>
      <div class="resize-corner ne" @mousedown.stop="emitResize($event, 'ne')"></div>
      <div class="resize-corner sw" @mousedown.stop="emitResize($event, 'sw')"></div>
      <div class="resize-corner se" @mousedown.stop="emitResize($event, 'se')"></div>
    </template>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'
import HomeCanvasImageDisplay from './HomeCanvasImageDisplay.vue'
import type { CanvasImageItem } from '../../types/canvasNodes'

type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'sw' | 'se'

defineProps<{
  item: CanvasImageItem
  isActive: boolean
  isPending: boolean
  showActionMenu: boolean
  showStyleStrengthControl: boolean
  styleStrengthMenuStyle?: string | Record<string, string>
  directGenerating: boolean
  displayHeight: number
  styleInnovateStrengthPct: number
  showResizeHandles: boolean
  setImageRef: (el: Element | ComponentPublicInstance | null) => void
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (event: 'item-mousedown', value: MouseEvent): void
  (event: 'item-contextmenu', value: MouseEvent): void
  (event: 'image-load', value: Event): void
  (event: 'action-remove-bg'): void
  (event: 'style-innovate-mousedown', value: MouseEvent): void
  (event: 'style-innovate-click'): void
  (event: 'action-to-vector'): void
  (event: 'action-sketch-to-design'): void
  (event: 'style-strength-track-mousedown', value: MouseEvent): void
  (event: 'resize-mousedown', payload: { event: MouseEvent; direction: ResizeDirection }): void
}>()

function emitResize(event: MouseEvent, direction: ResizeDirection) {
  emit('resize-mousedown', { event, direction })
}
</script>

<style scoped>
.canvas-item {
  position: absolute;
  background: #fff;
  padding: 4px;
  border-radius: 12px;
  border: 1.5px solid #e2e4e8;
  user-select: none;
  -webkit-user-select: none;
  pointer-events: auto;
}

.canvas-item.active {
  border-color: #9278e6;
  z-index: 10;
}

.canvas-item.is-pending {
  border-color: rgba(146, 120, 230, 0.44);
}

.resize-edge,
.resize-corner {
  position: absolute;
  z-index: 11;
}

.resize-edge.n {
  top: -4px;
  left: 10px;
  right: 10px;
  height: 8px;
  cursor: ns-resize;
}

.resize-edge.s {
  bottom: -4px;
  left: 10px;
  right: 10px;
  height: 8px;
  cursor: ns-resize;
}

.resize-edge.e {
  right: -4px;
  top: 10px;
  bottom: 10px;
  width: 8px;
  cursor: ew-resize;
}

.resize-edge.w {
  left: -4px;
  top: 10px;
  bottom: 10px;
  width: 8px;
  cursor: ew-resize;
}

.resize-corner {
  width: 16px;
  height: 16px;
  background: transparent;
  border: none;
  border-radius: 50%;
}

.resize-corner.nw {
  top: -8px;
  left: -8px;
  cursor: nwse-resize;
}

.resize-corner.ne {
  top: -8px;
  right: -8px;
  cursor: nesw-resize;
}

.resize-corner.sw {
  bottom: -8px;
  left: -8px;
  cursor: nesw-resize;
}

.resize-corner.se {
  bottom: -8px;
  right: -8px;
  cursor: nwse-resize;
}
</style>
