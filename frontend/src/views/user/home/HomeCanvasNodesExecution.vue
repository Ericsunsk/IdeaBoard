<template>
  <template v-for="node in editNodes" :key="node.id">
    <EditNode
      :node="node"
      :active="isNodeSelected('edit', node.id)"
      :dragging="isNodeDragging('edit', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getEditNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('edit', node.id)"
      @node-mousedown="emit('edit-node-mousedown', $event, node)"
      @delete="emit('edit-delete', node.id)"
      @update:mode="emit('edit-update-mode', node, $event)"
      @generate="emit('edit-generate', node)"
      @port-mousedown="(event, port) => emit('edit-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in fusionNodes" :key="node.id">
    <FusionNode
      :node="node"
      :active="isNodeSelected('fusion', node.id)"
      :dragging="isNodeDragging('fusion', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getFusionNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('fusion', node.id)"
      @node-mousedown="emit('fusion-node-mousedown', $event, node)"
      @delete="emit('fusion-delete', node.id)"
      @update:blend-ratio="emit('fusion-update-blend-ratio', node, $event)"
      @generate="emit('fusion-generate', node)"
      @port-mousedown="(event, port) => emit('fusion-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in enhanceNodes" :key="node.id">
    <EnhanceNode
      :node="node"
      :active="isNodeSelected('enhance', node.id)"
      :dragging="isNodeDragging('enhance', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getEnhanceNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('enhance', node.id)"
      @node-mousedown="emit('enhance-node-mousedown', $event, node)"
      @delete="emit('enhance-delete', node.id)"
      @update:mode="emit('enhance-update-mode', node, $event)"
      @update:upscale-mode="emit('enhance-update-upscale-mode', node, $event)"
      @update:upscale-factor="emit('enhance-update-upscale-factor', node, $event)"
      @update:output-format="emit('enhance-update-output-format', node, $event)"
      @update:output-quality="emit('enhance-update-output-quality', node, $event)"
      @update:extend-top="emit('enhance-update-extend-top', node, $event)"
      @update:extend-right="emit('enhance-update-extend-right', node, $event)"
      @update:extend-bottom="emit('enhance-update-extend-bottom', node, $event)"
      @update:extend-left="emit('enhance-update-extend-left', node, $event)"
      @generate="emit('enhance-generate', node)"
      @port-mousedown="(event, port) => emit('enhance-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in tryOnNodes" :key="node.id">
    <TryOnNode
      :node="node"
      :active="isNodeSelected('tryon', node.id)"
      :dragging="isNodeDragging('tryon', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getTryOnNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('tryon', node.id)"
      @node-mousedown="emit('tryon-node-mousedown', $event, node)"
      @delete="emit('tryon-delete', node.id)"
      @update:tile-mode="emit('tryon-update-tile-mode', node, $event)"
      @update:rotation="emit('tryon-update-rotation', node, $event)"
      @update:scale="emit('tryon-update-scale', node, $event)"
      @update:opacity="emit('tryon-update-opacity', node, $event)"
      @generate="emit('tryon-generate', node)"
      @port-mousedown="(event, port) => emit('tryon-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in colorNodes" :key="node.id">
    <ColorNode
      :node="node"
      :active="isNodeSelected('color', node.id)"
      :dragging="isNodeDragging('color', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getColorNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('color', node.id)"
      @node-mousedown="emit('color-node-mousedown', $event, node)"
      @delete="emit('color-delete', node.id)"
      @apply-preset="emit('color-apply-preset', node, $event)"
      @remove-color="emit('color-remove-color', node, $event)"
      @add-color="emit('color-add-color', node, $event)"
      @strength-input="emit('color-strength-input', node, $event)"
      @generate="emit('color-generate', node)"
      @port-mousedown="(event, port) => emit('color-port-mousedown', event, node.id, port)"
    />
  </template>

</template>

<script setup lang="ts">
import EditNode from '../../../components/canvas/nodes/EditNode.vue'
import EnhanceNode from '../../../components/canvas/nodes/EnhanceNode.vue'
import FusionNode from '../../../components/canvas/nodes/FusionNode.vue'
import ColorNode from '../../../components/canvas/nodes/ColorNode.vue'
import TryOnNode from '../../../components/canvas/nodes/TryOnNode.vue'
import type {
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ColorNodeItem,
  TryOnNodeItem,
} from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'

const props = defineProps<{
  editNodes: EditNodeItem[]
  fusionNodes: FusionNodeItem[]
  enhanceNodes: EnhanceNodeItem[]
  tryOnNodes: TryOnNodeItem[]
  colorNodes: ColorNodeItem[]
  generating: boolean
  copyMode: boolean
  isNodeSelected: (
    kind: 'edit' | 'fusion' | 'enhance' | 'tryon' | 'color',
    id: number,
  ) => boolean
  isNodeDragging: (
    kind: 'edit' | 'fusion' | 'enhance' | 'tryon' | 'color',
    id: number,
  ) => boolean
  getEditNodeUiState: (node: EditNodeItem) => NodeUiState | undefined
  getFusionNodeUiState: (node: FusionNodeItem) => NodeUiState | undefined
  getEnhanceNodeUiState: (node: EnhanceNodeItem) => NodeUiState | undefined
  getTryOnNodeUiState: (node: TryOnNodeItem) => NodeUiState | undefined
  getColorNodeUiState: (node: ColorNodeItem) => NodeUiState | undefined
  getNodeConnectionErrorPorts: (
    kind: 'edit' | 'fusion' | 'enhance' | 'tryon' | 'color',
    id: number,
  ) => string[]
}>()

const emit = defineEmits<{
  (e: 'edit-node-mousedown', event: MouseEvent, node: EditNodeItem): void
  (e: 'edit-delete', nodeId: number): void
  (e: 'edit-update-mode', node: EditNodeItem, value: EditNodeItem['mode']): void
  (e: 'edit-generate', node: EditNodeItem): void
  (e: 'edit-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'fusion-node-mousedown', event: MouseEvent, node: FusionNodeItem): void
  (e: 'fusion-delete', nodeId: number): void
  (e: 'fusion-update-blend-ratio', node: FusionNodeItem, value: number): void
  (e: 'fusion-generate', node: FusionNodeItem): void
  (e: 'fusion-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'enhance-node-mousedown', event: MouseEvent, node: EnhanceNodeItem): void
  (e: 'enhance-delete', nodeId: number): void
  (e: 'enhance-update-mode', node: EnhanceNodeItem, value: EnhanceNodeItem['mode']): void
  (e: 'enhance-update-upscale-mode', node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleMode']): void
  (e: 'enhance-update-upscale-factor', node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleFactor']): void
  (e: 'enhance-update-output-format', node: EnhanceNodeItem, value: EnhanceNodeItem['outputFormat']): void
  (e: 'enhance-update-output-quality', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-update-extend-top', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-update-extend-right', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-update-extend-bottom', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-update-extend-left', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-generate', node: EnhanceNodeItem): void
  (e: 'enhance-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'tryon-node-mousedown', event: MouseEvent, node: TryOnNodeItem): void
  (e: 'tryon-delete', nodeId: number): void
  (e: 'tryon-update-tile-mode', node: TryOnNodeItem, value: TryOnNodeItem['tileMode']): void
  (e: 'tryon-update-rotation', node: TryOnNodeItem, value: number): void
  (e: 'tryon-update-scale', node: TryOnNodeItem, value: number): void
  (e: 'tryon-update-opacity', node: TryOnNodeItem, value: number): void
  (e: 'tryon-generate', node: TryOnNodeItem): void
  (e: 'tryon-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'color-node-mousedown', event: MouseEvent, node: ColorNodeItem): void
  (e: 'color-delete', nodeId: number): void
  (e: 'color-apply-preset', node: ColorNodeItem, value: string[]): void
  (e: 'color-remove-color', node: ColorNodeItem, index: number): void
  (e: 'color-add-color', node: ColorNodeItem, value: string): void
  (e: 'color-strength-input', node: ColorNodeItem, value: number): void
  (e: 'color-generate', node: ColorNodeItem): void
  (e: 'color-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
}>()
</script>
