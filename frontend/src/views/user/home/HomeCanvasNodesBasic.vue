<template>
  <template v-for="node in imageNodes" :key="node.id">
    <ImageNode
      :node="node"
      :active="isNodeSelected('image', node.id)"
      :dragging="isNodeDragging('image', node.id)"
      :copy-mode="copyMode"
      :stage-height="getImageNodeStageHeight(node)"
      :display-image="getResolvedImageNodeDisplay(node).image"
      :display-mode="getResolvedImageNodeDisplay(node).mode"
      :ui-state="getImageNodeUiState(node)"
      @node-mousedown="emit('image-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('image-port-mousedown', event, node.id, port)"
      @delete="emit('image-delete', node.id)"
      @stage-drop="emit('image-stage-drop', $event, node)"
      @open-file="emit('image-open-file', node)"
      @append-to-canvas="emit('image-append-to-canvas', node)"
      @reset="emit('image-reset', node)"
      @display-dragstart="emit('image-display-dragstart', $event, node)"
    />
  </template>

  <template v-for="node in promptNodes" :key="node.id">
    <PromptNode
      :node="node"
      :active="isNodeSelected('prompt', node.id)"
      :dragging="isNodeDragging('prompt', node.id)"
      :copy-mode="copyMode"
      :ui-state="getPromptNodeUiState(node)"
      @node-mousedown="emit('prompt-node-mousedown', $event, node)"
      @port-mousedown="(event, port) => emit('prompt-port-mousedown', event, node.id, port)"
      @delete="emit('prompt-delete', node.id)"
      @focus="emit('prompt-focus', node.id)"
      @update:text="emit('prompt-update-text', node, $event)"
    />
  </template>
</template>

<script setup lang="ts">
import ImageNode from '../../../components/canvas/nodes/ImageNode.vue'
import PromptNode from '../../../components/canvas/nodes/PromptNode.vue'
import type { ImageNodeItem, PromptNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../../../types/canvasNodeUi'

type ImageNodeDisplay = {
  image: { url: string; ratio: number } | null
  mode: 'source' | 'output' | 'mask'
}

const props = defineProps<{
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  copyMode: boolean
  isNodeSelected: (kind: 'image' | 'prompt', id: number) => boolean
  isNodeDragging: (kind: 'image' | 'prompt', id: number) => boolean
  getImageNodeUiState: (node: ImageNodeItem) => NodeUiState | undefined
  getPromptNodeUiState: (node: PromptNodeItem) => PromptNodeUiState | undefined
  getImageNodeStageHeight: (node: ImageNodeItem) => number
  getResolvedImageNodeDisplay: (node: ImageNodeItem) => ImageNodeDisplay
}>()

const emit = defineEmits<{
  (e: 'image-node-mousedown', event: MouseEvent, node: ImageNodeItem): void
  (e: 'image-port-mousedown', event: MouseEvent, nodeId: number, port: 'image'): void
  (e: 'image-delete', nodeId: number): void
  (e: 'image-stage-drop', event: DragEvent, node: ImageNodeItem): void
  (e: 'image-open-file', node: ImageNodeItem): void
  (e: 'image-append-to-canvas', node: ImageNodeItem): void
  (e: 'image-reset', node: ImageNodeItem): void
  (e: 'image-display-dragstart', event: DragEvent, node: ImageNodeItem): void
  (e: 'prompt-node-mousedown', event: MouseEvent, node: PromptNodeItem): void
  (e: 'prompt-port-mousedown', event: MouseEvent, nodeId: number, port: 'prompt'): void
  (e: 'prompt-delete', nodeId: number): void
  (e: 'prompt-focus', nodeId: number): void
  (e: 'prompt-update-text', node: PromptNodeItem, value: string): void
}>()
</script>
