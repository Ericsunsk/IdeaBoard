<template>
  <div class="image-node-expand-overlay">
    <div
      class="image-node-expand-frame"
      data-image-extend-frame
      :style="overlayStyle"
      @pointermove="emit('handle-pointermove', $event)"
      @pointerup="emit('handle-pointerup', $event)"
      @pointercancel="emit('handle-pointerup', $event)"
      @pointerdown.stop.prevent="emit('frame-pointerdown', $event)"
    >
      <button
        v-for="anchor in anchors"
        :key="anchor"
        type="button"
        class="image-node-expand-handle"
        :class="`is-${anchor}`"
        :style="handleStyle"
        @pointerdown.stop.prevent="emit('handle-pointerdown', { anchor, event: $event })"
      />
      <div class="image-node-expand-composer-wrap" data-image-extend-composer @pointerdown.stop>
        <NodePromptComposer
          v-model="promptDraft"
          class="image-node-expand-composer"
          offset="10px"
          placeholder="输入扩图提示词"
          :disabled="directGenerating"
          :allow-empty="true"
          :output-count="outputCount"
          @update:output-count="emit('update:output-count', $event)"
          @submit="onSubmit"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import NodePromptComposer from './NodePromptComposer.vue'
import {
  CANVAS_EXTEND_DRAG_ANCHORS,
  type CanvasExtendDragAnchor,
} from '../../composables/useImageCanvasExtendOverlay'

const props = defineProps<{
  overlayStyle: Record<string, string>
  handleRadius: number
  directGenerating: boolean
  outputCount: number
}>()

const emit = defineEmits<{
  (e: 'submit', prompt: string, outputCount: number): void
  (e: 'update:output-count', value: number): void
  (e: 'handle-pointerdown', payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent }): void
  (e: 'handle-pointermove', event: PointerEvent): void
  (e: 'handle-pointerup', event: PointerEvent): void
  (e: 'frame-pointerdown', event: PointerEvent): void
}>()

const promptDraft = ref('')
const anchors = CANVAS_EXTEND_DRAG_ANCHORS
const handleStyle = computed(() => {
  const radius = Number.isFinite(props.handleRadius)
    ? Math.max(2, Math.round(props.handleRadius))
    : 4
  const diameter = radius * 2
  return {
    width: `${diameter}px`,
    height: `${diameter}px`,
  }
})

function onSubmit(prompt: string, outputCount: number) {
  if (props.directGenerating) return
  emit('submit', prompt, outputCount)
  promptDraft.value = ''
}
</script>

<style scoped>
.image-node-expand-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: none;
}

.image-node-expand-frame {
  --expand-frame-stroke: 2px;
  --expand-frame-accent: #9278e6;
  --expand-frame-grid: #9278e6;
  --expand-frame-contrast: rgba(255, 255, 255, 0.74);
  --expand-handle-center-inset: calc(var(--expand-frame-stroke) / 2);
  position: absolute;
  border: 0;
  border-radius: 5px;
  box-sizing: border-box;
  pointer-events: auto;
  cursor: move;
  background-image:
    repeating-linear-gradient(to bottom, var(--expand-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to bottom, var(--expand-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to right, var(--expand-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to right, var(--expand-frame-grid) 0 4px, transparent 4px 9px);
  background-size:
    1px 100%,
    1px 100%,
    100% 1px,
    100% 1px;
  background-position:
    33.333% 0,
    66.666% 0,
    0 33.333%,
    0 66.666%;
  background-repeat: no-repeat;
}

.image-node-expand-frame::before {
  content: '';
  position: absolute;
  inset: 0;
  border: var(--expand-frame-stroke) solid var(--expand-frame-accent);
  border-radius: inherit;
  box-shadow:
    0 0 0 1px var(--expand-frame-contrast),
    inset 0 0 0 1px var(--expand-frame-contrast);
  pointer-events: none;
}

.image-node-expand-composer-wrap {
  position: absolute;
  left: 0;
  right: 0;
  top: 100%;
  pointer-events: none;
}

.image-node-expand-composer {
  pointer-events: auto;
}

.image-node-expand-handle {
  position: absolute;
  border: 1.5px solid #9278e6;
  background: #ffffff;
  border-radius: 999px;
  padding: 0;
  pointer-events: auto;
  box-sizing: border-box;
}

.image-node-expand-handle.is-n {
  left: 50%;
  top: var(--expand-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: ns-resize;
}

.image-node-expand-handle.is-s {
  left: 50%;
  top: calc(100% - var(--expand-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: ns-resize;
}

.image-node-expand-handle.is-e {
  left: calc(100% - var(--expand-handle-center-inset));
  top: 50%;
  transform: translate(-50%, -50%);
  cursor: ew-resize;
}

.image-node-expand-handle.is-w {
  left: var(--expand-handle-center-inset);
  top: 50%;
  transform: translate(-50%, -50%);
  cursor: ew-resize;
}

.image-node-expand-handle.is-nw {
  left: var(--expand-handle-center-inset);
  top: var(--expand-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: nwse-resize;
}

.image-node-expand-handle.is-ne {
  left: calc(100% - var(--expand-handle-center-inset));
  top: var(--expand-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: nesw-resize;
}

.image-node-expand-handle.is-sw {
  left: var(--expand-handle-center-inset);
  top: calc(100% - var(--expand-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: nesw-resize;
}

.image-node-expand-handle.is-se {
  left: calc(100% - var(--expand-handle-center-inset));
  top: calc(100% - var(--expand-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: nwse-resize;
}
</style>
