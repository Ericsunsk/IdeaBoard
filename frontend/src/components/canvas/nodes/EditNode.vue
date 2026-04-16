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
            Partial
          </NodeActionButton>
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'pattern-inpaint'"
            @click="emit('update:mode', 'pattern-inpaint')"
          >
            Pattern
          </NodeActionButton>
        </div>
        <div class="edit-mode-meta">
          <span class="edit-mode-pill">{{ modeLabel }}</span>
          <span class="edit-mode-dot"></span>
          <span class="edit-mode-meta-text">Mask-guided edit</span>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
          生成
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
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, EDIT_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: EditNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  imageInputConnected?: boolean
  maskInputConnected?: boolean
  promptInputConnected?: boolean
  outputConnected?: boolean
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:mode', value: 'partial-edit' | 'pattern-inpaint'): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(EDIT_NODE_IO_SLOTS, null, {
    connected: {
      image: props.imageInputConnected,
      mask: props.maskInputConnected,
      prompt: props.promptInputConnected,
      output: props.outputConnected,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

const modeLabel = computed(() => (props.node.mode === 'pattern-inpaint' ? 'Pattern' : 'Partial'))

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.edit-node-body {
  gap: 8px;
}

.edit-mode-meta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 18px;
}

.edit-mode-pill {
  height: 18px;
  padding: 0 7px;
  border-radius: 999px;
  border: 1px solid rgba(146, 120, 230, 0.2);
  background: rgba(247, 243, 255, 0.9);
  display: inline-flex;
  align-items: center;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: #7b67d1;
}

.edit-mode-dot {
  width: 4px;
  height: 4px;
  border-radius: 999px;
  background: #c5cedb;
}

.edit-mode-meta-text {
  font-size: 11px;
  font-weight: 600;
  color: #7d8899;
}

</style>
