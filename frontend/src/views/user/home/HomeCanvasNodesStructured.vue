<template>
  <template v-for="node in modelNodes" :key="node.id">
    <ModelNode
      :node="node"
      :active="isNodeSelected('model', node.id)"
      :dragging="isNodeDragging('model', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :stage-height="getModelNodeStageHeight(node)"
      :source-image-url="getModelNodeEffectiveSource(node).url"
      :has-connected-image-input="getModelNodeEffectiveSource(node).connected"
      :ui-state="getModelNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('model', node.id)"
      :angle-options="modelAngleOptions"
      :background-options="modelBackgroundOptions"
      @node-mousedown="emit('model-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('model-port-mousedown', event, node.id, port)"
      @delete="emit('model-delete', node.id)"
      @open-file-picker="emit('model-open-file-picker', node)"
      @clear-image="emit('model-clear-image', node)"
      @stage-drop="emit('model-stage-drop', $event, node)"
      @toggle-angle="emit('model-toggle-angle', node, $event)"
      @clear-angles="emit('model-clear-angles', node)"
      @select-background="emit('model-select-background', node, $event)"
      @generate="emit('model-generate', node)"
    />
  </template>

  <template v-for="node in mannequinNodes" :key="node.id">
    <MannequinNode
      :node="node"
      :active="isNodeSelected('mannequin', node.id)"
      :dragging="isNodeDragging('mannequin', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :stage-height="getMannequinNodeStageHeight(node)"
      :source-image-url="getMannequinNodeEffectiveSource(node).url"
      :has-connected-image-input="getMannequinNodeEffectiveSource(node).connected"
      :ui-state="getMannequinNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('mannequin', node.id)"
      @node-mousedown="emit('mannequin-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('mannequin-port-mousedown', event, node.id, port)"
      @delete="emit('mannequin-delete', node.id)"
      @open-file-picker="emit('mannequin-open-file-picker', node)"
      @clear-image="emit('mannequin-clear-image', node)"
      @stage-drop="emit('mannequin-stage-drop', $event, node)"
      @generate="emit('mannequin-generate', node)"
    />
  </template>

  <template v-for="node in viewpointNodes" :key="node.id">
    <ViewpointNode
      :node="node"
      :active="isNodeSelected('viewpoint', node.id)"
      :dragging="isNodeDragging('viewpoint', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :stage-height="getViewpointNodeStageHeight(node)"
      :source-images="getViewpointNodeEffectiveSourceImages(node)"
      :has-connected-image-input="getViewpointNodeEffectiveSource(node).connected"
      :ui-state="getViewpointNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('viewpoint', node.id)"
      @node-mousedown="emit('viewpoint-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('viewpoint-port-mousedown', event, node.id, port)"
      @delete="emit('viewpoint-delete', node.id)"
      @open-file-picker="emit('viewpoint-open-file-picker', node)"
      @clear-images="emit('viewpoint-clear-images', node)"
      @remove-image="emit('viewpoint-remove-image', node, $event)"
      @stage-drop="emit('viewpoint-stage-drop', $event, node)"
      @update:category="emit('viewpoint-update-category', node, $event)"
      @toggle:viewpoint="emit('viewpoint-toggle-selection', node, $event)"
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
import MannequinNode from '../../../components/canvas/nodes/MannequinNode.vue'
import MaskNode from '../../../components/canvas/nodes/MaskNode.vue'
import ModelNode from '../../../components/canvas/nodes/ModelNode.vue'
import ViewpointNode from '../../../components/canvas/nodes/ViewpointNode.vue'
import type {
  ImageNodeOutputItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  ViewpointNodeItem,
} from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'

const props = defineProps<{
  modelNodes: ModelNodeItem[]
  mannequinNodes: MannequinNodeItem[]
  viewpointNodes: ViewpointNodeItem[]
  maskNodes: MaskNodeItem[]
  generating: boolean
  copyMode: boolean
  isNodeSelected: (
    kind: 'model' | 'mannequin' | 'viewpoint' | 'mask',
    id: number,
  ) => boolean
  isNodeDragging: (
    kind: 'model' | 'mannequin' | 'viewpoint' | 'mask',
    id: number,
  ) => boolean
  getModelNodeUiState: (node: ModelNodeItem) => NodeUiState | undefined
  getMannequinNodeUiState: (node: MannequinNodeItem) => NodeUiState | undefined
  getViewpointNodeUiState: (node: ViewpointNodeItem) => NodeUiState | undefined
  getMaskNodeUiState: (node: MaskNodeItem) => NodeUiState | undefined
  getNodeConnectionErrorPorts: (
    kind: 'model' | 'mannequin' | 'viewpoint',
    id: number,
  ) => string[]
  getModelNodeStageHeight: (node: ModelNodeItem) => number
  getModelNodeEffectiveSource: (node: ModelNodeItem) => { url: string; ratio: number; connected: boolean }
  getMannequinNodeStageHeight: (node: MannequinNodeItem) => number
  getMannequinNodeEffectiveSource: (node: MannequinNodeItem) => { url: string; ratio: number; connected: boolean }
  getViewpointNodeStageHeight: (node: ViewpointNodeItem) => number
  getViewpointNodeEffectiveSource: (node: ViewpointNodeItem) => { images: { url: string; ratio: number }[]; connected: boolean }
  getViewpointNodeEffectiveSourceImages: (node: ViewpointNodeItem) => ImageNodeOutputItem[]
  getMaskNodeStageHeight: (node: MaskNodeItem) => number
  getMaskCanvasWidth: (node: MaskNodeItem) => number
  getMaskNodeEffectiveSource: (node: MaskNodeItem) => { url: string; ratio: number; connected: boolean }
  modelAngleOptions: Array<{ key: string; label: string }>
  modelBackgroundOptions: Array<{ key: string; label: string }>
}>()

const emit = defineEmits<{
  (e: 'model-node-mousedown', event: MouseEvent, node: ModelNodeItem): void
  (e: 'model-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'model-delete', nodeId: number): void
  (e: 'model-open-file-picker', node: ModelNodeItem): void
  (e: 'model-clear-image', node: ModelNodeItem): void
  (e: 'model-stage-drop', event: DragEvent, node: ModelNodeItem): void
  (e: 'model-toggle-angle', node: ModelNodeItem, key: string): void
  (e: 'model-clear-angles', node: ModelNodeItem): void
  (e: 'model-select-background', node: ModelNodeItem, key: string): void
  (e: 'model-generate', node: ModelNodeItem): void

  (e: 'mannequin-node-mousedown', event: MouseEvent, node: MannequinNodeItem): void
  (e: 'mannequin-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'mannequin-delete', nodeId: number): void
  (e: 'mannequin-open-file-picker', node: MannequinNodeItem): void
  (e: 'mannequin-clear-image', node: MannequinNodeItem): void
  (e: 'mannequin-stage-drop', event: DragEvent, node: MannequinNodeItem): void
  (e: 'mannequin-generate', node: MannequinNodeItem): void

  (e: 'viewpoint-node-mousedown', event: MouseEvent, node: ViewpointNodeItem): void
  (e: 'viewpoint-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
  (e: 'viewpoint-delete', nodeId: number): void
  (e: 'viewpoint-open-file-picker', node: ViewpointNodeItem): void
  (e: 'viewpoint-clear-images', node: ViewpointNodeItem): void
  (e: 'viewpoint-remove-image', node: ViewpointNodeItem, index: number): void
  (e: 'viewpoint-stage-drop', event: DragEvent, node: ViewpointNodeItem): void
  (e: 'viewpoint-update-category', node: ViewpointNodeItem, category: ViewpointNodeItem['activeCategory']): void
  (e: 'viewpoint-toggle-selection', node: ViewpointNodeItem, key: string): void
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
