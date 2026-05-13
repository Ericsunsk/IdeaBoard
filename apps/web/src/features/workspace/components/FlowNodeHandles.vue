<template>
  <template v-if="kind !== 'message'">
    <Handle
      v-for="port in handleInputPorts"
      :id="`in:${port}`"
      :key="`canvas-in-anchor:${port}`"
      type="target"
      :position="Position.Left"
      class="flow-canvas-node__handle-anchor"
      :style="inputAnchorHandleStyle"
    />
    <Handle
      v-if="hasInputPorts"
      id="in:__auto"
      type="target"
      :position="Position.Left"
      :class="[
        'flow-canvas-node__handle-aggregate',
        'flow-canvas-node__handle-aggregate--input',
        { 'is-visible': props.showHandles },
      ]"
      :style="inputAggregateHandleStyle"
    >
      <span class="flow-canvas-node__handle-plus" aria-hidden="true" />
    </Handle>
    <Handle
      v-if="aggregateOutputHandleId"
      :id="aggregateOutputHandleId"
      type="source"
      :position="Position.Right"
      :class="[
        'flow-canvas-node__handle-aggregate',
        'flow-canvas-node__handle-aggregate--output',
        { 'is-visible': props.showHandles },
      ]"
      :style="outputAggregateHandleStyle"
    >
      <span class="flow-canvas-node__handle-plus" aria-hidden="true" />
    </Handle>
    <Handle
      v-for="port in hiddenOutputAnchorPorts"
      :id="`out:${port}`"
      :key="`canvas-out-anchor:${port}`"
      type="source"
      :position="Position.Right"
      class="flow-canvas-node__handle-anchor"
      :style="outputAnchorHandleStyle"
    />
  </template>
</template>

<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
} from '../config/canvasNodeConnectionContract'
import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'

const props = defineProps<{
  kind: FlowNodeData['kind']
  snapshotNode: unknown
  uiState?: NodeUiState | PromptNodeUiState
  showHandles?: boolean
}>()

const emit = defineEmits<{
  (e: 'request-internals-update'): void
}>()

function getCanvasNodeType(): CanvasConnectionNodeType | null {
  return props.kind === 'message' ? null : props.kind
}

function getNodePortsByKind(
  kind: CanvasConnectionNodeType,
  direction: 'input' | 'output',
): CanvasConnectionPort[] {
  const node =
    props.snapshotNode && typeof props.snapshotNode === 'object'
      ? (props.snapshotNode as Record<string, unknown>)
      : { id: 0 }
  const list =
    direction === 'input'
      ? getCanvasNodeVisibleInputPorts(kind, node as never)
      : getCanvasNodeVisibleOutputPorts(kind, node as never)
  return list.map((port) => port.port)
}

function getCanvasNodeVisiblePortsForKind(direction: 'input' | 'output'): CanvasConnectionPort[] {
  const kind = getCanvasNodeType()
  if (!kind) return []
  return getNodePortsByKind(kind, direction)
}

const handleInputPorts = computed(() => getCanvasNodeVisiblePortsForKind('input'))
const handleOutputPorts = computed(() => getCanvasNodeVisiblePortsForKind('output'))
const hasInputPorts = computed(() => handleInputPorts.value.length > 0)
const aggregateOutputHandleId = computed(() => {
  const [firstPort] = handleOutputPorts.value
  return firstPort ? `out:${firstPort}` : null
})
const hiddenOutputAnchorPorts = computed(() => handleOutputPorts.value.slice(1))

const inputAnchorHandleStyle = {
  left: '0',
  top: '50%',
  transform: 'translateY(-50%)',
}
const outputAnchorHandleStyle = {
  right: '0',
  top: '50%',
  transform: 'translateY(-50%)',
}
const inputAggregateHandleStyle = {
  left: '0',
  top: '50%',
  transform: 'translateY(-50%)',
}
const outputAggregateHandleStyle = {
  right: '0',
  top: '50%',
  transform: 'translateY(-50%)',
}

watch(
  () => [
    props.kind,
    props.snapshotNode,
    handleInputPorts.value.join('|'),
    handleOutputPorts.value.join('|'),
  ],
  () => {
    emit('request-internals-update')
  },
  { deep: true, flush: 'post' },
)
</script>
<style scoped>
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
  --handle-ring-color: #d8e0ea;
  --handle-plus-x: -26px;
  width: 1px;
  height: 1px;
  min-width: 1px;
  min-height: 1px;
  border: 0;
  background: transparent;
  color: #64748b;
  opacity: 0;
  box-shadow: none;
  pointer-events: all;
  z-index: 24;
  cursor: crosshair;
  touch-action: none;
  transition:
    opacity 0.14s ease,
    color 0.14s ease;
}

.flow-canvas-node__handle-aggregate--output {
  --handle-plus-x: 27px;
}

.flow-canvas-node__handle-plus {
  position: absolute;
  left: var(--handle-plus-x);
  top: 50%;
  display: block;
  width: 32px;
  height: 32px;
  border: 1.5px solid var(--handle-ring-color);
  border-radius: 999px;
  background: #ffffff;
  box-shadow: none;
  transform: translate(-50%, -50%);
  pointer-events: auto;
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

.flow-canvas-node__handle-aggregate:hover,
.flow-canvas-node__handle-aggregate.connecting {
  --handle-ring-color: #c4ccd8;
  color: #334155;
}

.flow-canvas-node__handle-aggregate.is-visible,
.flow-canvas-node__handle-aggregate.connecting {
  opacity: 1;
}
</style>
