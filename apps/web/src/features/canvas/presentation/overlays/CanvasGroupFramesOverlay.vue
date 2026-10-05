<template>
  <div class="canvas-group-frame-body-layer" aria-hidden="true">
    <div
      v-for="frame in frames"
      :key="frame.id"
      class="canvas-group-frame-body"
      :class="{ 'is-selected': frame.selected }"
      :style="frameStyle(frame)"
    />
  </div>
  <div class="canvas-group-frame-controls-layer">
    <div
      v-for="frame in selectedFrames"
      :key="frame.id"
      class="canvas-group-frame-controls"
      :style="frameStyle(frame)"
    >
      <button
        v-for="handle in resizeHandles"
        :key="handle"
        type="button"
        class="canvas-group-frame-resize-handle"
        :class="[`handle-${handle}`, resizeHandleKindClass(handle)]"
        :aria-label="t('canvas.controls.resizeGroupBounds')"
        @pointerdown.stop.prevent="onResizePointerDown($event, frame, handle)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  CanvasGroupFrame,
  CanvasGroupResizeHandle,
} from '../../domain/graph/canvasGroupGeometry.ts'

const props = defineProps<{
  frames: CanvasGroupFrame[]
}>()

const emit = defineEmits<{
  (
    event: 'resize-start',
    payload: {
      event: PointerEvent
      groupId: CanvasGroupFrame['id']
      handle: CanvasGroupResizeHandle
    },
  ): void
}>()

const { t } = useI18n()

const resizeHandles: CanvasGroupResizeHandle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']
const selectedFrames = computed(() => props.frames.filter((frame) => frame.selected))

function resizeHandleKindClass(handle: CanvasGroupResizeHandle) {
  return handle.length === 2 ? 'is-corner-handle' : 'is-edge-handle'
}

function frameStyle(frame: CanvasGroupFrame): CSSProperties {
  return {
    left: `${frame.bounds.left}px`,
    top: `${frame.bounds.top}px`,
    width: `${frame.bounds.width}px`,
    height: `${frame.bounds.height}px`,
  }
}

function onResizePointerDown(
  event: PointerEvent,
  frame: CanvasGroupFrame,
  handle: CanvasGroupResizeHandle,
) {
  emit('resize-start', {
    event,
    groupId: frame.id,
    handle,
  })
}
</script>

<style scoped>
.canvas-group-frame-body-layer,
.canvas-group-frame-controls-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.canvas-group-frame-controls-layer {
  z-index: var(--canvas-group-frame-controls-z);
}

.canvas-group-frame-body,
.canvas-group-frame-controls {
  position: absolute;
}

.canvas-group-frame-controls {
  --canvas-group-frame-edge-hit-size: 16px;
  --canvas-group-frame-corner-hit-size: 24px;
}

.canvas-group-frame-body {
  z-index: var(--canvas-group-frame-body-z);
  border: 1px solid var(--canvas-group-frame-border);
  border-radius: var(--canvas-selection-group-radius);
  background: var(--canvas-group-frame-surface);
  box-shadow: none;
  pointer-events: none;
}

.canvas-group-frame-body.is-selected {
  z-index: var(--canvas-group-frame-selected-body-z);
  box-shadow: 0 0 0 var(--canvas-group-frame-selected-ring-width)
    var(--canvas-group-frame-selected-ring-color);
  pointer-events: auto;
}

.canvas-group-frame-resize-handle {
  position: absolute;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  appearance: none;
  background: transparent;
  box-shadow: none;
  pointer-events: auto;
  touch-action: none;
  user-select: none;
}

.canvas-group-frame-resize-handle.is-edge-handle {
  z-index: 1;
}

.canvas-group-frame-resize-handle.is-corner-handle {
  z-index: 2;
  width: var(--canvas-group-frame-corner-hit-size);
  height: var(--canvas-group-frame-corner-hit-size);
}

.canvas-group-frame-resize-handle:focus-visible {
  outline: var(--focus-outline-width) solid var(--focus-outline-color);
  outline-offset: var(--focus-outline-offset);
}

.canvas-group-frame-resize-handle.handle-n {
  top: 0;
  right: 0;
  left: 0;
  height: var(--canvas-group-frame-edge-hit-size);
  cursor: ns-resize;
  transform: translateY(-50%);
}

.canvas-group-frame-resize-handle.handle-ne {
  top: 0;
  right: 0;
  cursor: nesw-resize;
  transform: translate(50%, -50%);
}

.canvas-group-frame-resize-handle.handle-e {
  top: 0;
  right: 0;
  bottom: 0;
  width: var(--canvas-group-frame-edge-hit-size);
  cursor: ew-resize;
  transform: translateX(50%);
}

.canvas-group-frame-resize-handle.handle-se {
  right: 0;
  bottom: 0;
  cursor: nwse-resize;
  transform: translate(50%, 50%);
}

.canvas-group-frame-resize-handle.handle-s {
  right: 0;
  bottom: 0;
  left: 0;
  height: var(--canvas-group-frame-edge-hit-size);
  cursor: ns-resize;
  transform: translateY(50%);
}

.canvas-group-frame-resize-handle.handle-sw {
  bottom: 0;
  left: 0;
  cursor: nesw-resize;
  transform: translate(-50%, 50%);
}

.canvas-group-frame-resize-handle.handle-w {
  top: 0;
  bottom: 0;
  left: 0;
  width: var(--canvas-group-frame-edge-hit-size);
  cursor: ew-resize;
  transform: translateX(-50%);
}

.canvas-group-frame-resize-handle.handle-nw {
  top: 0;
  left: 0;
  cursor: nwse-resize;
  transform: translate(-50%, -50%);
}

@media (prefers-reduced-motion: reduce) {
  .canvas-group-frame-resize-handle {
    transition: none;
  }
}
</style>
