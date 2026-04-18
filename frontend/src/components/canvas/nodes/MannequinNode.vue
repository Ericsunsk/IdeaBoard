<template>
  <div
    class="workflow-node mannequin-node"
    data-node-kind="mannequin"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Mannequin"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <circle cx="12" cy="6" r="2.4" />
          <path d="M8.5 12.5c.7-2.3 2.1-3.6 3.5-3.6s2.8 1.3 3.5 3.6" />
          <path d="M9.5 20l2.1-5.2 2.4 5.2" />
        </svg>
      </template>
    </NodeHeader>

    <div class="mannequin-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div
        class="model-stage mannequin-stage"
        :data-mannequin-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
      >
        <template v-if="effectiveSourceImageUrl">
          <img :src="effectiveSourceImageUrl" class="mannequin-stage-image" />
          <div v-if="node.sourceImageUrl && !hasConnectedImageInput" class="model-stage-actions mannequin-stage-actions">
            <NodeActionButton variant="stage" @click="emit('open-file-picker')">更换图片</NodeActionButton>
            <NodeActionButton variant="stage-ghost" @click="emit('clear-image')">清空</NodeActionButton>
          </div>
        </template>
        <div v-else class="model-stage-empty mannequin-stage-empty" @click="emit('open-file-picker')">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>上传人台图（点击或拖拽）</span>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '生成人模' }}
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="mannequin"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { MannequinNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, MANNEQUIN_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: MannequinNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  stageHeight: number
  sourceImageUrl?: string
  hasConnectedImageInput?: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'open-file-picker'): void
  (e: 'clear-image'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image' | 'prompt'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(MANNEQUIN_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)
const effectiveSourceImageUrl = computed(() => props.sourceImageUrl ?? props.node.sourceImageUrl)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image' | 'prompt')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.mannequin-node-body {
  gap: 8px;
}

.mannequin-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
</style>
