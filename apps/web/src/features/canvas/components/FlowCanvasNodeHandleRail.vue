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
      :key="`canvas-${aggregate.direction}-aggregate:${aggregate.id}`"
      :type="aggregate.handleType"
      :position="aggregate.position"
      :connectable="aggregate.connectable"
      :connectable-start="aggregate.direction === 'output' && aggregate.connectable"
      :connectable-end="false"
      :class="[
        'flow-canvas-node__handle-aggregate',
        `flow-canvas-node__handle-aggregate--${aggregate.direction}`,
        {
          'is-visible': aggregate.visible,
          'is-proximity-active': aggregate.proximityActive,
        },
      ]"
      :style="aggregate.style"
      :aria-hidden="aggregate.visible && !isConnectionDragging ? undefined : 'true'"
      :aria-label="aggregate.ariaLabel"
      :tabindex="aggregate.visible && !isConnectionDragging ? 0 : -1"
      :title="aggregate.title"
    >
      <span class="flow-canvas-node__handle-plus" aria-hidden="true">
        <Plus class="flow-canvas-node__handle-icon" />
      </span>
    </Handle>
  </div>
</template>

<script setup lang="ts">
import { Handle, useVueFlow } from '@vue-flow/core'
import { Plus } from '@iconoir/vue'
import { computed, watch } from 'vue'
import type { FlowNodeData } from '../types/flowTypes'
import { useFlowCanvasNodeHandleRail } from '../renderer/vue-flow/useFlowCanvasNodeHandleRail'
import { useFlowNodeHandlesPresentation } from '../renderer/vue-flow/useFlowNodeHandlesPresentation'

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
const presentation = useFlowNodeHandlesPresentation({
  nodeId: () => props.nodeId,
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
  --handle-ring-color: var(--flow-edge-color);
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
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: calc(var(--flow-edge-width) * 1px) solid var(--handle-ring-color);
  border-radius: 999px;
  background: transparent;
  color: var(--handle-ring-color);
  box-shadow: none;
  transform: translate(-50%, -50%);
  pointer-events: none;
  transition:
    left var(--motion-control-duration) var(--motion-control-easing),
    top var(--motion-control-duration) var(--motion-control-easing),
    border-color var(--motion-feedback-duration) var(--motion-feedback-easing),
    color var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.flow-canvas-node__handle-icon {
  width: 16px;
  height: 16px;
  stroke-width: calc(var(--flow-edge-width) * 1.5);
}

.flow-canvas-node__handle-aggregate.is-visible:hover,
.flow-canvas-node__handle-aggregate.connecting,
.flow-canvas-node__handle-aggregate:focus-visible {
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
  opacity: 1;
  visibility: visible;
  pointer-events: all;
}

.flow-canvas-node__handle-rail.is-connection-dragging .flow-canvas-node__handle-aggregate {
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
}
</style>
