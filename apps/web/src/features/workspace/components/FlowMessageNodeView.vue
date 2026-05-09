<template>
  <div
    class="flow-canvas-node__message"
    :class="{ 'is-pending': isMessagePending, 'is-selected': selected }"
    :style="{ width: `${messageDisplayWidth}px` }"
    @mousedown="$emit('node-mousedown', $event)"
    @contextmenu.stop.prevent="$emit('contextmenu', $event)"
  >
    <div class="flow-canvas-node__drag-zone flow-canvas-node__drag-zone--message" />
    <img
      :src="node.url"
      class="flow-canvas-node__message-image flow-canvas-node__drag-surface nodrag"
      :ref="setMessageImageRef"
      draggable="true"
      @load="onMessageImageLoad"
      @dragstart="$emit('image-dragstart', $event)"
    />
    <button
      v-for="direction in MESSAGE_RESIZE_DIRECTIONS"
      v-if="selected && !isMessagePending"
      :key="`message-resize-${direction}`"
      class="flow-canvas-node__message-resize-handle"
      :class="`is-${direction}`"
      type="button"
      :aria-label="`Resize message image ${direction}`"
      @mousedown.stop.prevent="$emit('resize-mousedown', { event: $event, direction })"
    />
    <div v-if="isMessagePending" class="flow-canvas-node__message-pending" aria-hidden="true">
      处理中
    </div>
    <HomeCanvasImageActionMenu
      v-if="showMessageActionMenu"
      :show-style-strength-control="showStyleStrengthControl"
      :style-strength-menu-style="showStyleStrengthControl ? messageStyleStrengthMenuStyle : undefined"
      :direct-generating="messageDirectGenerating"
      :display-height="messageDisplayHeight"
      :style-innovate-strength-pct="styleInnovateStrengthPct"
      :set-style-strength-track-ref="setStyleStrengthTrackRef"
      @action-remove-bg="$emit('message-action', 'remove-bg')"
      @style-innovate-mousedown="$emit('style-innovate-mousedown', $event)"
      @style-innovate-click="$emit('message-action', 'style-innovate')"
      @action-to-vector="$emit('message-action', 'to-vector')"
      @action-sketch-to-design="$emit('message-action', 'sketch-to-design')"
      @style-strength-track-mousedown="$emit('style-strength-track-mousedown', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, type ComponentPublicInstance } from 'vue'
import type { CanvasImageItem } from '../types/canvasNodes'
import { useFlowMessageNode, MESSAGE_RESIZE_DIRECTIONS, type MessageAction, type ResizeDirection } from '../composables/useFlowMessageNode'
import HomeCanvasImageActionMenu from './home/HomeCanvasImageActionMenu.vue'

const props = defineProps<{
  node: CanvasImageItem
  selected: boolean
  messageActionVisible: boolean
  messageDirectGenerating: boolean
  showStyleStrengthControl: boolean
  styleInnovateStrengthPct: number
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'image-dragstart', event: DragEvent): void
  (e: 'resize-mousedown', payload: { event: MouseEvent; direction: ResizeDirection }): void
  (e: 'message-action', action: MessageAction): void
  (e: 'style-innovate-mousedown', event: MouseEvent): void
  (e: 'style-strength-track-mousedown', event: MouseEvent): void
  (e: 'request-internals-update'): void
}>()

const messageSnapshot = computed<CanvasImageItem | null>(() => props.node)

const {
  messageDisplayWidth,
  messageDisplayHeight,
  messageStyleStrengthMenuStyle,
  isMessagePending,
  setMessageImageRef,
  onMessageImageLoad: _onMessageImageLoad,
} = useFlowMessageNode(messageSnapshot)

const showMessageActionMenu = computed(() => !!props.messageActionVisible && !!props.node)

function onMessageImageLoad() {
  _onMessageImageLoad()
  emit('request-internals-update')
}
</script>
<style scoped>
.flow-canvas-node__drag-zone {
  position: absolute;
  z-index: 20;
  pointer-events: auto;
  cursor: grab;
  touch-action: none;
}

.flow-canvas-node__drag-zone:active {
  cursor: grabbing;
}

.flow-canvas-node__drag-zone--message {
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 1;
}

.flow-canvas-node__message {
  position: relative;
  background: #fff;
  border: 1.5px solid #e2e4e8;
  border-radius: 12px;
  padding: 4px;
  box-sizing: border-box;
  overflow: visible;
}

.flow-canvas-node__message.is-selected {
  border-color: #9278e6;
}

.flow-canvas-node__message-image {
  display: block;
  width: 100%;
  height: auto;
  object-fit: contain;
  border-radius: 8px;
  aspect-ratio: auto;
  user-select: none;
  -webkit-user-select: none;
  -webkit-user-drag: none;
}

.flow-canvas-node__drag-surface {
  cursor: grab;
}

.flow-canvas-node__drag-surface:active {
  cursor: grabbing;
}

.flow-canvas-node__message-pending {
  position: absolute;
  inset: 4px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.12);
  color: #334155;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  pointer-events: none;
  user-select: none;
  z-index: 3;
}

.flow-canvas-node__message-resize-handle {
  position: absolute;
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 999px;
  background: transparent;
  z-index: 14;
}

.flow-canvas-node__message-resize-handle.is-n {
  top: -7px;
  left: 50%;
  transform: translateX(-50%);
  cursor: ns-resize;
}

.flow-canvas-node__message-resize-handle.is-s {
  bottom: -7px;
  left: 50%;
  transform: translateX(-50%);
  cursor: ns-resize;
}

.flow-canvas-node__message-resize-handle.is-e {
  right: -7px;
  top: 50%;
  transform: translateY(-50%);
  cursor: ew-resize;
}

.flow-canvas-node__message-resize-handle.is-w {
  left: -7px;
  top: 50%;
  transform: translateY(-50%);
  cursor: ew-resize;
}

.flow-canvas-node__message-resize-handle.is-nw {
  left: -7px;
  top: -7px;
  cursor: nwse-resize;
}

.flow-canvas-node__message-resize-handle.is-ne {
  right: -7px;
  top: -7px;
  cursor: nesw-resize;
}

.flow-canvas-node__message-resize-handle.is-sw {
  left: -7px;
  bottom: -7px;
  cursor: nesw-resize;
}

.flow-canvas-node__message-resize-handle.is-se {
  right: -7px;
  bottom: -7px;
  cursor: nwse-resize;
}
</style>
