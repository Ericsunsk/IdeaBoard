<template>
  <div class="image-node-outpaint-overlay">
    <div
      class="image-node-outpaint-frame"
      data-outpaint-frame
      :style="overlayStyle"
      @pointermove.stop="emit('handle-pointermove', $event)"
      @pointerup.stop="emit('handle-pointerup', $event)"
      @pointercancel.stop="emit('handle-pointerup', $event)"
      @pointerdown.stop.prevent="emit('frame-pointerdown', $event)"
      @mousedown.stop
      @mouseup.stop
      @click.stop
    >
      <button
        v-for="anchor in anchors"
        :key="anchor"
        type="button"
        class="image-node-outpaint-handle"
        :class="`is-${anchor}`"
        :style="handleStyle"
        @pointerdown.stop.prevent="emit('handle-pointerdown', { anchor, event: $event })"
      />
      <div class="image-node-outpaint-composer-wrap" data-outpaint-composer @pointerdown.stop>
        <NodePromptComposer
          v-model="promptDraft"
          class="image-node-outpaint-composer"
          offset="10px"
          :placeholder="outpaintPromptPlaceholder"
          :allow-empty="true"
          :output-count="1"
          :show-output-count="false"
          :show-output-resolution="false"
          :billing-state="billingQuote"
          @submit="onSubmit"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { NodePromptComposer } from '../../../presentation/nodes/primitives'
import { OUTPAINT_DRAG_ANCHORS, type OutpaintDragAnchor } from '../useImageOutpaintOverlay'
import { SERVICE } from '../../../../../core/contracts/imageServices'
import { useCanvasBillingQuote } from '../../execution'
import { buildOutpaintReadinessRequest } from '../../execution/commands/imageTaskRequests'
import { i18n } from '../../../../../core/i18n'

const props = defineProps<{
  overlayStyle: Record<string, string>
  handleRadius: number
}>()

const emit = defineEmits<{
  (e: 'submit', prompt: string): void
  (e: 'handle-pointerdown', payload: { anchor: OutpaintDragAnchor; event: PointerEvent }): void
  (e: 'handle-pointermove', event: PointerEvent): void
  (e: 'handle-pointerup', event: PointerEvent): void
  (e: 'frame-pointerdown', event: PointerEvent): void
}>()

const promptDraft = ref('')
const anchors = OUTPAINT_DRAG_ANCHORS
const outpaintPromptPlaceholder = i18n.global.t('canvas.tools.outpaintPromptPlaceholder')
const billingQuote = useCanvasBillingQuote(
  () => SERVICE.outpaint,
  () => true,
  () => buildOutpaintReadinessRequest(),
)
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

function onSubmit(prompt: string) {
  emit('submit', prompt)
  promptDraft.value = ''
}
</script>

<style scoped>
.image-node-outpaint-overlay {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: none;
}

.image-node-outpaint-frame {
  --outpaint-frame-stroke: 2px;
  --outpaint-frame-accent: var(--canvas-accent);
  --outpaint-frame-grid: var(--canvas-accent);
  --outpaint-frame-contrast: rgba(255, 255, 255, 0.74);
  --outpaint-handle-center-inset: calc(var(--outpaint-frame-stroke) / 2);
  position: absolute;
  border: 0;
  border-radius: 5px;
  box-sizing: border-box;
  pointer-events: auto;
  cursor: move;
  background-image:
    repeating-linear-gradient(to bottom, var(--outpaint-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to bottom, var(--outpaint-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to right, var(--outpaint-frame-grid) 0 4px, transparent 4px 9px),
    repeating-linear-gradient(to right, var(--outpaint-frame-grid) 0 4px, transparent 4px 9px);
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

.image-node-outpaint-frame::before {
  content: '';
  position: absolute;
  inset: 0;
  border: var(--outpaint-frame-stroke) solid var(--outpaint-frame-accent);
  border-radius: inherit;
  box-shadow:
    0 0 0 1px var(--outpaint-frame-contrast),
    inset 0 0 0 1px var(--outpaint-frame-contrast);
  pointer-events: none;
}

.image-node-outpaint-composer-wrap {
  position: absolute;
  left: 0;
  right: 0;
  top: 100%;
  pointer-events: none;
}

.image-node-outpaint-composer {
  pointer-events: auto;
}

.image-node-outpaint-handle {
  position: absolute;
  border: 1.5px solid var(--outpaint-frame-accent);
  background: #ffffff;
  border-radius: var(--radius-pill);
  padding: 0;
  pointer-events: auto;
  box-sizing: border-box;
}

.image-node-outpaint-handle.is-n {
  left: 50%;
  top: var(--outpaint-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: ns-resize;
}

.image-node-outpaint-handle.is-s {
  left: 50%;
  top: calc(100% - var(--outpaint-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: ns-resize;
}

.image-node-outpaint-handle.is-e {
  left: calc(100% - var(--outpaint-handle-center-inset));
  top: 50%;
  transform: translate(-50%, -50%);
  cursor: ew-resize;
}

.image-node-outpaint-handle.is-w {
  left: var(--outpaint-handle-center-inset);
  top: 50%;
  transform: translate(-50%, -50%);
  cursor: ew-resize;
}

.image-node-outpaint-handle.is-nw {
  left: var(--outpaint-handle-center-inset);
  top: var(--outpaint-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: nwse-resize;
}

.image-node-outpaint-handle.is-ne {
  left: calc(100% - var(--outpaint-handle-center-inset));
  top: var(--outpaint-handle-center-inset);
  transform: translate(-50%, -50%);
  cursor: nesw-resize;
}

.image-node-outpaint-handle.is-sw {
  left: var(--outpaint-handle-center-inset);
  top: calc(100% - var(--outpaint-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: nesw-resize;
}

.image-node-outpaint-handle.is-se {
  left: calc(100% - var(--outpaint-handle-center-inset));
  top: calc(100% - var(--outpaint-handle-center-inset));
  transform: translate(-50%, -50%);
  cursor: nwse-resize;
}
</style>
