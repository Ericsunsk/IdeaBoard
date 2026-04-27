<template>
  <template v-for="node in modelNodes" :key="node.id">
    <ModelNode
      :node="node"
      :active="isNodeSelected('model', node.id)"
      :dragging="isNodeDragging('model', node.id)"
      :copy-mode="copyMode"
      :ui-state="getModelNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('model', node.id)"
      :background-options="modelBackgroundOptions"
      @node-mousedown="emit('model-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('model-port-mousedown', event, node.id, port)"
      @delete="emit('model-delete', node.id)"
      @update-angle-yaw="emit('model-update-angle-yaw', node, $event)"
      @update-angle-framing="emit('model-update-angle-framing', node, $event)"
      @update-model-strength="emit('model-update-model-strength', node, $event)"
      @update-pose-strength="emit('model-update-pose-strength', node, $event)"
      @select-background="emit('model-select-background', node, $event)"
      @generate="emit('model-generate', node)"
    />
  </template>

  <template v-for="node in viewpointNodes" :key="node.id">
    <ViewpointNode
      :node="node"
      :active="isNodeSelected('viewpoint', node.id)"
      :dragging="isNodeDragging('viewpoint', node.id)"
      :copy-mode="copyMode"
      :ui-state="getViewpointNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('viewpoint', node.id)"
      @node-mousedown="emit('viewpoint-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('viewpoint-port-mousedown', event, node.id, port)"
      @delete="emit('viewpoint-delete', node.id)"
      @update:category="emit('viewpoint-update-category', node, $event)"
      @select:viewpoint="emit('viewpoint-select', node, $event)"
      @update:lighting="emit('viewpoint-update-lighting', node, $event)"
      @generate="emit('viewpoint-generate', node)"
    />
  </template>

  <template v-for="node in maskNodes" :key="node.id">
    <MaskNode
      :node="node"
      :active="isNodeSelected('mask', node.id)"
      :dragging="isNodeDragging('mask', node.id)"
      :copy-mode="copyMode"
      :stage-height="getMaskNodeStageHeight(node)"
      :canvas-width="getMaskCanvasWidth(node)"
      :source-image-url="getMaskNodeEffectiveSource(node).url"
      :has-connected-image-input="getMaskNodeEffectiveSource(node).connected"
      :ui-state="getMaskNodeUiState(node)"
      @node-mousedown="emit('mask-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('mask-port-mousedown', event, node.id, port)"
      @delete="emit('mask-delete', node.id)"
      @open-file-picker="emit('mask-open-file-picker', node)"
      @clear-image="emit('mask-clear-image', node)"
      @stage-drop="emit('mask-stage-drop', $event, node)"
      @canvas-mousedown="emit('mask-canvas-mousedown', $event, node)"
      @set-draw-mode="emit('mask-set-draw-mode', node, $event)"
      @update:brush-size="emit('mask-update-brush-size', node, $event)"
      @update:refine-amount="emit('mask-update-refine-amount', node, $event)"
      @toggle-invert="emit('mask-toggle-invert', node)"
      @clear-mask="emit('mask-clear-mask', node)"
      @set-canvas-ref="emit('mask-set-canvas-ref', node.id, $event)"
    />
  </template>
</template>

<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import type {
  MaskNodeItem,
  ModelNodeItem,
  ViewpointNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

const ModelNode = defineAsyncComponent(() => import('../nodes/ModelNode.vue'))
const ViewpointNode = defineAsyncComponent(() => import('../nodes/ViewpointNode.vue'))
const MaskNode = defineAsyncComponent(() => import('../nodes/MaskNode.vue'))

const props = defineProps<{
  modelNodes: ModelNodeItem[]
  viewpointNodes: ViewpointNodeItem[]
  maskNodes: MaskNodeItem[]
  copyMode: boolean
  isNodeSelected: (
    kind: 'model' | 'viewpoint' | 'mask',
    id: number,
  ) => boolean
  isNodeDragging: (
    kind: 'model' | 'viewpoint' | 'mask',
    id: number,
  ) => boolean
  getModelNodeUiState: (node: ModelNodeItem) => NodeUiState | undefined
  getViewpointNodeUiState: (node: ViewpointNodeItem) => NodeUiState | undefined
  getMaskNodeUiState: (node: MaskNodeItem) => NodeUiState | undefined
  getNodeConnectionErrorPorts: (
    kind: 'model' | 'viewpoint',
    id: number,
  ) => string[]
  getMaskNodeStageHeight: (node: MaskNodeItem) => number
  getMaskCanvasWidth: (node: MaskNodeItem) => number
  getMaskNodeEffectiveSource: (node: MaskNodeItem) => { url: string; ratio: number; connected: boolean }
  modelBackgroundOptions: Array<{ key: string; label: string }>
}>()

const emit = defineEmits<{
  (e: 'model-node-mousedown', event: MouseEvent, node: ModelNodeItem): void
  (e: 'model-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'model-delete', nodeId: number): void
  (e: 'model-update-angle-yaw', node: ModelNodeItem, value: number): void
  (e: 'model-update-angle-framing', node: ModelNodeItem, value: 'full' | 'half' | 'portrait'): void
  (e: 'model-update-model-strength', node: ModelNodeItem, value: number): void
  (e: 'model-update-pose-strength', node: ModelNodeItem, value: number): void
  (e: 'model-select-background', node: ModelNodeItem, key: string): void
  (e: 'model-generate', node: ModelNodeItem): void

  (e: 'viewpoint-node-mousedown', event: MouseEvent, node: ViewpointNodeItem): void
  (e: 'viewpoint-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'viewpoint-delete', nodeId: number): void
  (e: 'viewpoint-update-category', node: ViewpointNodeItem, category: ViewpointNodeItem['activeCategory']): void
  (e: 'viewpoint-select', node: ViewpointNodeItem, key: string): void
  (e: 'viewpoint-update-lighting', node: ViewpointNodeItem, value: ViewpointNodeItem['lighting']): void
  (e: 'viewpoint-generate', node: ViewpointNodeItem): void

  (e: 'mask-node-mousedown', event: MouseEvent, node: MaskNodeItem): void
  (e: 'mask-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'mask-delete', nodeId: number): void
  (e: 'mask-open-file-picker', node: MaskNodeItem): void
  (e: 'mask-clear-image', node: MaskNodeItem): void
  (e: 'mask-stage-drop', event: DragEvent, node: MaskNodeItem): void
  (e: 'mask-canvas-mousedown', event: MouseEvent, node: MaskNodeItem): void
  (e: 'mask-set-draw-mode', node: MaskNodeItem, value: 'paint' | 'erase'): void
  (e: 'mask-update-brush-size', node: MaskNodeItem, value: number): void
  (e: 'mask-update-refine-amount', node: MaskNodeItem, value: number): void
  (e: 'mask-toggle-invert', node: MaskNodeItem): void
  (e: 'mask-clear-mask', node: MaskNodeItem): void
  (e: 'mask-set-canvas-ref', nodeId: number, element: Element | null): void
}>()
</script>
