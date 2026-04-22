<template>
  <template v-for="entry in imageNodeEntries" :key="entry.node.id">
    <ImageNode
      :node="entry.node"
      :active="isNodeSelected('image', entry.node.id)"
      :dragging="isNodeDragging('image', entry.node.id)"
      :copy-mode="copyMode"
      :stage-height="getImageNodeStageHeight(entry.node)"
      :display-image="entry.displayImage"
      :display-mode="entry.displayMode"
      :input-locked="entry.inputLocked"
      :ui-state="entry.uiState"
      @node-mousedown="emit('image-node-mousedown', $event, entry.node)"
      @port-mousedown="(event, port) => emit('image-port-mousedown', event, entry.node.id, port)"
      @delete="emit('image-delete', entry.node.id)"
      @stage-drop="emit('image-stage-drop', $event, entry.node)"
      @open-file="emit('image-open-file', entry.node)"
      @append-to-canvas="emit('image-append-to-canvas', entry.node)"
      @reset="emit('image-reset', entry.node)"
      @display-dragstart="emit('image-display-dragstart', $event, entry.node)"
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
import { computed } from 'vue'
import ImageNode from '../nodes/ImageNode.vue'
import PromptNode from '../nodes/PromptNode.vue'
import type { ImageNodeItem, PromptNodeItem } from '../../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../../types/canvasNodeUi'

type ImageNodeDisplay = {
  image: { url: string; ratio: number } | null
  mode: 'source' | 'node-output' | 'linked-output' | 'mask' | 'pending'
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

const imageNodeEntries = computed(() =>
  props.imageNodes.map(node => {
    const uiState = props.getImageNodeUiState(node)
    const resolvedDisplay = props.getResolvedImageNodeDisplay(node)
    const hasConnectedInput = Boolean(uiState?.connection?.connected?.image)
    const hasPendingInput = Boolean(uiState?.connection?.pending?.image)

    return {
      node,
      uiState,
      displayImage: resolvedDisplay.image,
      displayMode: resolvedDisplay.mode,
      inputLocked: hasConnectedInput || hasPendingInput,
    }
  }),
)
</script>
