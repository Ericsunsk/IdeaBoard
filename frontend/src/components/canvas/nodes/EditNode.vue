<template>
  <div
    class="workflow-node edit-node"
    data-node-kind="edit"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Edit"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-4-4L4 16v4z" />
          <path d="M13.5 6.5l4 4" />
        </svg>
      </template>
    </NodeHeader>

    <div class="edit-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="edit-mode-card node-mode-card">
        <div class="edit-mode-row node-option-row">
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'partial-edit'"
            @click="emit('update:mode', 'partial-edit')"
          >
            Detail
          </NodeActionButton>
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'bg-swap'"
            @click="emit('update:mode', 'bg-swap')"
          >
            Background
          </NodeActionButton>
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'face-swap'"
            @click="emit('update:mode', 'face-swap')"
          >
            Face
          </NodeActionButton>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '生成' }}
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="edit"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { EditNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildEditNodeIoContext, buildNodeIoFromSlots, EDIT_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: EditNodeItem
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
  (e: 'update:mode', value: 'partial-edit' | 'bg-swap' | 'face-swap'): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image' | 'imageB' | 'mask' | 'prompt'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(EDIT_NODE_IO_SLOTS, buildEditNodeIoContext(props.node), {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      imageB: props.uiState?.connection?.connected?.imageB,
      mask: props.uiState?.connection?.connected?.mask,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
      imageB: props.uiState?.connection?.pending?.imageB,
      mask: props.uiState?.connection?.pending?.mask,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image' | 'imageB' | 'mask' | 'prompt')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.edit-node-body {
  gap: 8px;
}
</style>
