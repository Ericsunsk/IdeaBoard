<template>
  <div
    class="flow-canvas-node__handle-rail"
    :class="{ 'is-connection-dragging': isConnectionDragging }"
  >
    <Handle
      v-for="anchor in presentation.anchors"
      :id="anchor.id"
      :key="`canvas-${anchor.direction}-anchor:${anchor.id}`"
      :type="anchor.handleType"
      :position="anchor.position"
      :connectable="false"
      class="flow-canvas-node__handle-anchor"
      :style="anchor.style"
    />
    <Handle
      v-for="aggregate in presentation.aggregates"
      :id="aggregate.id"
      :ref="aggregate.setElement"
      :key="`canvas-${aggregate.direction}-aggregate:${aggregate.id}`"
      :type="aggregate.handleType"
      :position="aggregate.position"
      :connectable="aggregate.connectable"
      :connectable-start="aggregate.direction === 'output' && aggregate.connectable"
      :connectable-end="aggregate.direction === 'input' && aggregate.connectable"
      :class="[
        'flow-canvas-node__handle-aggregate',
        `flow-canvas-node__handle-aggregate--${aggregate.direction}`,
        {
          'is-visible': aggregate.visible,
          'is-proximity-active': aggregate.proximityActive,
          'is-node-drop-target': isConnectionDragging && aggregate.direction === 'input',
        },
      ]"
      :style="[
        aggregate.style,
        isConnectionDragging && aggregate.direction === 'input' ? nodeDropTargetStyle : undefined,
      ]"
      :aria-hidden="aggregate.visible && !isConnectionDragging ? undefined : 'true'"
      :aria-label="aggregate.ariaLabel"
      :tabindex="aggregate.visible && !isConnectionDragging ? 0 : -1"
      :title="aggregate.title"
    >
      <span class="flow-canvas-node__handle-plus" aria-hidden="true" />
    </Handle>
  </div>
</template>

<script setup lang="ts">
import { Handle, useVueFlow } from '@vue-flow/core'
import { computed, watch, type CSSProperties } from 'vue'
import type { FlowNodeData } from '../types/flowTypes'
import { useFlowCanvasNodeHandleRail } from '../composables/useFlowCanvasNodeHandleRail'
import { useFlowNodeHandlesPresentation } from '../composables/useFlowNodeHandlesPresentation'

const props = defineProps<{
  nodeId: string
  kind: FlowNodeData['kind']
  interactionRoot: HTMLElement | null
  dragging?: boolean
  disabled?: boolean
}>()

const interactionRoot = computed(() => props.interactionRoot)
const { showHandleControls, refreshTransientHandleVisibility } =
  useFlowCanvasNodeHandleRail(interactionRoot)
const { connectionStartHandle } = useVueFlow()
const isConnectionDragging = computed(() => connectionStartHandle.value !== null)
const nodeDropTargetStyle = {
  left: '0',
  right: '0',
  top: '0',
  bottom: '0',
  width: 'auto',
  height: 'auto',
  transform: 'none',
} satisfies CSSProperties
const presentation = useFlowNodeHandlesPresentation({
  kind: () => props.kind,
  showHandles: () => showHandleControls.value,
  disabled: () => props.disabled === true,
})

watch(
  () => props.dragging === true,
  (dragging, wasDragging) => {
    if (wasDragging && !dragging) refreshTransientHandleVisibility()
  },
  { flush: 'post' },
)
watch(
  connectionStartHandle,
  (current, previous) => {
    if (previous?.nodeId === props.nodeId && current === null) {
      refreshTransientHandleVisibility()
    }
  },
  { flush: 'post' },
)
</script>

<style scoped>
.flow-canvas-node__handle-rail {
  position: absolute;
  inset: 0;
  z-index: 24;
  pointer-events: none;
  overflow: visible;
}

.flow-canvas-node__handle-rail.is-connection-dragging {
  z-index: 50;
}

.flow-canvas-node__handle-anchor {
  width: 1px;
  height: 1px;
  min-width: 1px;
  min-height: 1px;
  border: 0;
  background: transparent;
  opacity: 0;
  box-shadow: none;
  pointer-events: none;
  z-index: 1;
}

.flow-canvas-node__handle-aggregate {
  --handle-ring-color: var(--color-canvas-handle-border);
  --handle-plus-rest-x: -12px;
  --handle-plus-active-x: -26px;
  --handle-plus-hover-x: -29px;
  --handle-plus-hit-x: -26px;
  --handle-plus-hit-y: 0px;
  --handle-plus-x: var(--handle-plus-rest-x);
  --handle-plus-y: 0px;
  width: 1px;
  height: 1px;
  min-width: 1px;
  min-height: 1px;
  border: 0;
  background: transparent;
  color: var(--handle-ring-color);
  opacity: 0;
  visibility: hidden;
  box-shadow: none;
  pointer-events: none;
  z-index: 24;
  cursor: crosshair;
  touch-action: none;
  outline: none;
  transition:
    opacity var(--motion-feedback-duration) var(--motion-feedback-easing),
    color var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.flow-canvas-node__handle-aggregate--output {
  --handle-plus-rest-x: 13px;
  --handle-plus-active-x: 27px;
  --handle-plus-hover-x: 30px;
  --handle-plus-hit-x: 27px;
}

.flow-canvas-node__handle-aggregate::before {
  content: '';
  position: absolute;
  left: var(--handle-plus-hit-x);
  top: calc(50% + var(--handle-plus-hit-y));
  width: calc(52px * var(--canvas-node-ui-scale, 1));
  height: calc(52px * var(--canvas-node-ui-scale, 1));
  border-radius: 999px;
  transform: translate(-50%, -50%);
  pointer-events: auto;
}

.flow-canvas-node__handle-plus {
  position: absolute;
  left: var(--handle-plus-x);
  top: calc(50% + var(--handle-plus-y));
  display: block;
  width: 32px;
  height: 32px;
  border: 1.5px solid var(--handle-ring-color);
  border-radius: 999px;
  background: var(--color-ib-surface-raised);
  box-shadow: none;
  transform: translate(-50%, -50%);
  pointer-events: none;
  transition:
    left var(--motion-control-duration) var(--motion-control-easing),
    top var(--motion-control-duration) var(--motion-control-easing),
    border-color var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.flow-canvas-node__handle-plus::before,
.flow-canvas-node__handle-plus::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 50%;
  width: 12px;
  height: 2px;
  border-radius: 999px;
  background: currentColor;
  transform: translate(-50%, -50%);
}

.flow-canvas-node__handle-plus::after {
  transform: translate(-50%, -50%) rotate(90deg);
}

.flow-canvas-node__handle-aggregate.is-visible:hover,
.flow-canvas-node__handle-aggregate.connecting,
.flow-canvas-node__handle-aggregate:focus-visible {
  --handle-ring-color: var(--color-canvas-handle-border-active);
  --handle-plus-x: var(--handle-plus-hover-x);
  opacity: 1;
}

.flow-canvas-node__handle-aggregate.is-visible,
.flow-canvas-node__handle-aggregate.connecting,
.flow-canvas-node__handle-aggregate:focus-visible {
  --handle-plus-x: var(--handle-plus-active-x);
  opacity: 1;
  visibility: visible;
  pointer-events: all;
}

.flow-canvas-node__handle-aggregate:focus-visible .flow-canvas-node__handle-plus {
  border-color: var(--ib-focus-outline-color);
}

.flow-canvas-node__handle-aggregate.is-visible:hover {
  --handle-plus-x: var(--handle-plus-hover-x);
}

.flow-canvas-node__handle-aggregate.is-proximity-active {
  --handle-ring-color: var(--color-canvas-handle-border-active);
  opacity: 1;
  visibility: visible;
  pointer-events: all;
}

.flow-canvas-node__handle-aggregate.is-proximity-active .flow-canvas-node__handle-plus {
  transition: border-color var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.flow-canvas-node__handle-rail.is-connection-dragging .flow-canvas-node__handle-aggregate {
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
}

.flow-canvas-node__handle-rail.is-connection-dragging
  .flow-canvas-node__handle-aggregate--input.is-node-drop-target {
  visibility: visible;
  pointer-events: auto;
}
</style>
