<template>
  <div
    class="workflow-node prompt-node"
    data-node-kind="prompt"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Prompt"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
        </svg>
      </template>
    </NodeHeader>
    <textarea
      :value="node.text"
      class="workflow-node-input"
      placeholder="type your idea..."
      @input="onInput"
      @focus="emit('focus')"
      @mousedown.stop
      @contextmenu.stop
    />
    <NodePorts
      node-kind="prompt"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { PromptNodeItem } from '../../../types/canvasNodes'
import type { PromptNodeUiState } from '../../../types/canvasNodeUi'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, PROMPT_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: PromptNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: PromptNodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'focus'): void
  (e: 'update:text', value: string): void
  (e: 'port-mousedown', event: MouseEvent, port: 'prompt'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const resolvedUiState = computed<PromptNodeUiState>(() => ({
  connection: {
    hasAnyConnection: props.uiState?.connection?.hasAnyConnection ?? false,
    connected: {
      output: props.uiState?.connection?.connected?.output ?? false,
    },
    pending: props.uiState?.connection?.pending,
  },
  primaryAction: props.uiState?.primaryAction,
}))

const ports = computed(() =>
  buildNodeIoFromSlots(PROMPT_NODE_IO_SLOTS, null, {
    connected: resolvedUiState.value.connection.connected,
    pending: resolvedUiState.value.connection.pending,
  }).ports,
)

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
}

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'prompt')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.workflow-node-input {
  width: 100%;
  min-height: 100px;
  border: none;
  resize: none;
  outline: none;
  padding: 12px;
  font-size: 14px;
  line-height: 1.6;
  color: #334155;
  background: transparent;
  font-family: inherit;
}

.workflow-node-input::placeholder {
  color: #94a3b8;
}
</style>
