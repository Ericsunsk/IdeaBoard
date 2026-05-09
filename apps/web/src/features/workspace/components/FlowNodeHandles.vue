<template>
  <template v-if="kind !== 'message'">
    <Handle
      v-for="port in handleInputPorts"
      :id="`in:${port}`"
      :key="`canvas-in:${port}`"
      type="target"
      :position="Position.Left"
      :class="[
        'flow-canvas-node__handle',
        'flow-canvas-node__handle--input',
        {
          'is-connected': isHandleConnected(port, 'input'),
          'is-pending': isHandlePending(port),
        },
      ]"
      :data-port-kind="resolvePortVisualKind(port)"
      :style="resolveHandleStyle(port, 'input')"
    />
    <Handle
      v-for="port in handleOutputPorts"
      :id="`out:${port}`"
      :key="`canvas-out:${port}`"
      type="source"
      :position="Position.Right"
      :class="[
        'flow-canvas-node__handle',
        'flow-canvas-node__handle--output',
        {
          'is-connected': isHandleConnected(port, 'output'),
          'is-pending': isHandlePending(port),
        },
      ]"
      :data-port-kind="resolvePortVisualKind(port)"
      :style="resolveHandleStyle(port, 'output')"
    />
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { getCanvasNodeVisibleInputPorts, getCanvasNodeVisibleOutputPorts } from '../config/canvasNodeConnectionContract'
import {
  buildEditNodeIoContext,
  buildNodeIoFromSlots,
  COLOR_NODE_IO_SLOTS,
  EDIT_NODE_IO_SLOTS,
  ENHANCE_NODE_IO_SLOTS,
  FUSION_NODE_IO_SLOTS,
  IMAGE_NODE_IO_SLOTS,
  MASK_NODE_IO_SLOTS,
  MODEL_NODE_IO_SLOTS,
  PROMPT_NODE_IO_SLOTS,
  VIEWPOINT_NODE_IO_SLOTS,
  WEAR_NODE_IO_SLOTS,
} from '../components/nodes/nodeIoConfig'
import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  EditNodeItem,
  EnhanceNodeItem,
} from '../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'

const props = defineProps<{
  kind: FlowNodeData['kind']
  snapshotNode: unknown
  uiState?: NodeUiState | PromptNodeUiState
}>()

function getCanvasNodeType(): CanvasConnectionNodeType | null {
  return props.kind === 'message' ? null : props.kind
}

function getNodePortsByKind(kind: CanvasConnectionNodeType, direction: 'input' | 'output'): CanvasConnectionPort[] {
  const node = props.snapshotNode && typeof props.snapshotNode === 'object'
    ? props.snapshotNode as Record<string, unknown>
    : { id: 0 }
  const list = direction === 'input'
    ? getCanvasNodeVisibleInputPorts(kind, node as never)
    : getCanvasNodeVisibleOutputPorts(kind, node as never)
  return list.map(port => port.port)
}

function getNodeIoTopMap(kind: CanvasConnectionNodeType): Partial<Record<CanvasConnectionPort, number>> {
  if (!props.snapshotNode || typeof props.snapshotNode !== 'object') return {}
  const snapshotNode = props.snapshotNode as Record<string, unknown>
  const state = {}
  if (kind === 'prompt') return portTopMapFromSlots(PROMPT_NODE_IO_SLOTS, null, state)
  if (kind === 'image') return portTopMapFromSlots(IMAGE_NODE_IO_SLOTS, null, state)
  if (kind === 'mask') return portTopMapFromSlots(MASK_NODE_IO_SLOTS, null, state)
  if (kind === 'color') return portTopMapFromSlots(COLOR_NODE_IO_SLOTS, null, state)
  if (kind === 'model') return portTopMapFromSlots(MODEL_NODE_IO_SLOTS, null, state)
  if (kind === 'viewpoint') return portTopMapFromSlots(VIEWPOINT_NODE_IO_SLOTS, null, state)
  if (kind === 'fusion') return portTopMapFromSlots(FUSION_NODE_IO_SLOTS, null, state)
  if (kind === 'wear') return portTopMapFromSlots(WEAR_NODE_IO_SLOTS, null, state)
  if (kind === 'edit') return portTopMapFromSlots(EDIT_NODE_IO_SLOTS, buildEditNodeIoContext(snapshotNode as unknown as EditNodeItem), state)
  if (kind === 'enhance') return portTopMapFromSlots(ENHANCE_NODE_IO_SLOTS, {
    supportsPrompt: (snapshotNode as unknown as EnhanceNodeItem).mode === 'canvas-extend',
    supportsMask: (snapshotNode as unknown as EnhanceNodeItem).mode === 'hd-upscale',
  }, state)
  return {}
}

function portTopMapFromSlots<T>(
  slots: readonly unknown[],
  context: T,
  state: { connected?: Record<string, boolean>; pending?: Record<string, boolean> },
): Partial<Record<CanvasConnectionPort, number>> {
  const built = buildNodeIoFromSlots(slots as never, context, state)
  const map: Partial<Record<CanvasConnectionPort, number>> = {}
  for (const port of built.ports) {
    if (port.visible === false) continue
    if (port.top == null) continue
    map[port.name as CanvasConnectionPort] = port.top
  }
  return map
}

function getCanvasNodeVisiblePortsForKind(direction: 'input' | 'output'): CanvasConnectionPort[] {
  const kind = getCanvasNodeType()
  if (!kind) return []
  return getNodePortsByKind(kind, direction)
}

function resolveHandleStyle(port: CanvasConnectionPort, direction: 'input' | 'output') {
  const kind = getCanvasNodeType()
  if (!kind) return undefined
  const topMap = getNodeIoTopMap(kind)
  const top = topMap[port]
  const side = direction === 'input' ? { left: '-5.5px' } : { right: '-5.5px' }
  if (typeof top === 'number' && Number.isFinite(top)) {
    return { top: `${top}px`, transform: 'translateY(-50%)', ...side }
  }
  return { ...side }
}

function resolvePortVisualKind(port: string): 'image' | 'mask' | 'prompt' | null {
  if (port === 'image' || port === 'imageA' || port === 'imageB' || port === 'output') return 'image'
  if (port === 'mask') return 'mask'
  if (port === 'prompt') return 'prompt'
  return null
}

function isHandleConnected(port: CanvasConnectionPort, direction: 'input' | 'output'): boolean {
  const connected = props.uiState?.connection?.connected as Partial<Record<string, boolean | undefined>> | undefined
  if (!connected) return false
  return !!connected[port]
}

function isHandlePending(port: CanvasConnectionPort): boolean {
  const pending = props.uiState?.connection?.pending as Partial<Record<string, boolean | undefined>> | undefined
  if (!pending) return false
  return !!pending[port]
}

const handleInputPorts = computed(() => getCanvasNodeVisiblePortsForKind('input'))
const handleOutputPorts = computed(() => getCanvasNodeVisiblePortsForKind('output'))
</script>
