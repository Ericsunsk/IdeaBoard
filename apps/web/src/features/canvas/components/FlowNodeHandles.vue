<template>
  <Handle
    v-for="port in handleInputPorts"
    :id="`in:${port}`"
    :key="`canvas-in-anchor:${port}`"
    type="target"
    :position="Position.Left"
    class="flow-canvas-node__handle-anchor"
    :style="leftCenteredHandleStyle"
  />
  <Handle
    v-if="hasInputPorts"
    id="in:__auto"
    type="target"
    :position="Position.Left"
    :connectable="areAggregateHandlesConnectable"
    :class="[
      'flow-canvas-node__handle-aggregate',
      'flow-canvas-node__handle-aggregate--input',
      { 'is-visible': props.showHandles },
    ]"
    :style="leftCenteredHandleStyle"
    :aria-hidden="props.showHandles ? undefined : 'true'"
    :aria-label="inputAggregateAriaLabel"
    :tabindex="props.showHandles ? 0 : -1"
    title="拖拽创建输入连接"
  >
    <span class="flow-canvas-node__handle-plus" aria-hidden="true" />
  </Handle>
  <Handle
    v-if="aggregateOutputHandleId"
    :id="aggregateOutputHandleId"
    type="source"
    :position="Position.Right"
    :connectable="areAggregateHandlesConnectable"
    :class="[
      'flow-canvas-node__handle-aggregate',
      'flow-canvas-node__handle-aggregate--output',
      { 'is-visible': props.showHandles },
    ]"
    :style="rightCenteredHandleStyle"
    :aria-hidden="props.showHandles ? undefined : 'true'"
    :aria-label="outputAggregateAriaLabel"
    :tabindex="props.showHandles ? 0 : -1"
    title="拖拽创建输出连接"
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
    :style="rightCenteredHandleStyle"
  />
</template>

<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import { computed, watch } from 'vue'
import {
  getCanvasNodeVisibleInputPorts,
  getCanvasNodeVisibleOutputPorts,
} from '../../canvas-board/domain'
import type { CanvasConnectionNodeType, CanvasConnectionPort } from '../../canvas-board/domain'
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

function getNodePortsByKind(
  kind: CanvasConnectionNodeType,
  direction: 'input' | 'output',
): CanvasConnectionPort[] {
  const list =
    direction === 'input'
      ? getCanvasNodeVisibleInputPorts(kind)
      : getCanvasNodeVisibleOutputPorts(kind)
  return list.map((port) => port.port)
}

function getCanvasNodeVisiblePortsForKind(direction: 'input' | 'output'): CanvasConnectionPort[] {
  return getNodePortsByKind(props.kind, direction)
}

const handleInputPorts = computed(() => getCanvasNodeVisiblePortsForKind('input'))
const handleOutputPorts = computed(() => getCanvasNodeVisiblePortsForKind('output'))
const hasInputPorts = computed(() => handleInputPorts.value.length > 0)
const areAggregateHandlesConnectable = computed(() => props.showHandles === true)
const nodeKindLabel = computed(() => {
  const labels: Partial<Record<FlowNodeData['kind'], string>> = {
    prompt: '提示词节点',
    image: '图片节点',
    fusion: '融合节点',
  }
  return labels[props.kind] ?? '节点'
})
const inputAggregateAriaLabel = computed(() => `${nodeKindLabel.value}输入连接点，拖拽创建连接`)
const outputAggregateAriaLabel = computed(() => `${nodeKindLabel.value}输出连接点，拖拽创建连接`)
const aggregateOutputHandleId = computed(() => {
  const [firstPort] = handleOutputPorts.value
  return firstPort ? `out:${firstPort}` : null
})
const hiddenOutputAnchorPorts = computed(() => handleOutputPorts.value.slice(1))

const leftCenteredHandleStyle = {
  left: '0',
  top: '50%',
  transform: 'translateY(-50%)',
}
const rightCenteredHandleStyle = {
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
  --handle-plus-rest-x: -12px;
  --handle-plus-active-x: -26px;
  --handle-plus-hover-x: -29px;
  --handle-plus-hit-x: -26px;
  --handle-plus-x: var(--handle-plus-rest-x);
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
    opacity 0.14s ease,
    color 0.14s ease;
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
  top: 50%;
  width: 52px;
  height: 52px;
  border-radius: 999px;
  transform: translate(-50%, -50%);
  pointer-events: auto;
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
  pointer-events: none;
  transition:
    left 0.18s ease,
    border-color 0.14s ease;
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
  --handle-ring-color: #c4ccd8;
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
</style>
