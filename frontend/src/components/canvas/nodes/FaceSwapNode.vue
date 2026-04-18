<template>
  <div
    class="workflow-node faceswap-node"
    data-node-kind="faceswap"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="FaceSwap"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="8" cy="8" r="3.2" />
          <path d="M3.5 18.5a4.8 4.8 0 0 1 9 0" />
          <circle cx="16.2" cy="9.5" r="2.4" />
          <path d="M13.5 17.8c.8-1.7 2.1-2.8 3.9-3.3" />
        </svg>
      </template>
    </NodeHeader>

    <div class="faceswap-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <NodeConnectionChips :items="connectionChips" />
      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '生成' }}
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="faceswap"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { FaceSwapNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, FACESWAP_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: FaceSwapNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image' | 'mask'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(FACESWAP_NODE_IO_SLOTS, null, {
    connected: {
      imageA: props.uiState?.connection?.connected?.imageA,
      imageB: props.uiState?.connection?.connected?.imageB,
      mask: props.uiState?.connection?.connected?.mask,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      imageA: props.uiState?.connection?.pending?.imageA,
      imageB: props.uiState?.connection?.pending?.imageB,
      mask: props.uiState?.connection?.pending?.mask,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image' | 'mask')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.faceswap-node-body {
  gap: 10px;
}
</style>
