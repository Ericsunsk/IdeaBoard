<template>
  <div
    class="workflow-node wear-node"
    data-node-kind="wear"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Wear"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M7 4h10l2 4-2.2 2.2-1.8-1.4V20H9V8.8L7.2 10.2 5 8l2-4z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="wear-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="wear-tile-card node-mode-card">
        <div class="wear-tile-row node-option-row">
          <NodeActionButton
            v-for="option in wearTileModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.tileMode === option.value"
            @click="emit('update:tile-mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
      </div>

      <div class="wear-control-group node-control-group">
        <div class="wear-control-row">
          <span class="node-control-label">Direction</span>
          <input
            class="wear-slider node-inline-range"
            :style="{ '--slider-pct': `${(node.rotation / 360) * 100}%` }"
            type="range"
            min="0"
            max="360"
            step="1"
            :value="node.rotation"
            @input="emit('update:rotation', Number(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="wear-control-row">
          <span class="node-control-label">Size</span>
          <input
            class="wear-slider node-inline-range"
            :style="{ '--slider-pct': `${((node.scale - 10) / 290) * 100}%` }"
            type="range"
            min="10"
            max="300"
            step="5"
            :value="node.scale"
            @input="emit('update:scale', Number(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="wear-control-row">
          <span class="node-control-label">Blend</span>
          <input
            class="wear-slider node-inline-range"
            :style="{ '--slider-pct': `${node.opacity}%` }"
            type="range"
            min="0"
            max="100"
            step="5"
            :value="node.opacity"
            @input="emit('update:opacity', Number(($event.target as HTMLInputElement).value))"
          />
        </div>
      </div>

      <NodeExecutionFooter
        :items="connectionChips"
        :disabled="generating || uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '上身'"
        @generate="emit('generate')"
      />
    </div>

    <NodePorts
      node-kind="wear"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { WearNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeExecutionFooter from './NodeExecutionFooter.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, WEAR_NODE_IO_SLOTS } from './nodeIoConfig'
import { WEAR_TILE_MODE_OPTIONS } from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: WearNodeItem
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
  (e: 'update:tile-mode', value: WearNodeItem['tileMode']): void
  (e: 'update:rotation', value: number): void
  (e: 'update:scale', value: number): void
  (e: 'update:opacity', value: number): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image' | 'imageA' | 'imageB' | 'mask'): void
}>()

const wearTileModeOptions = WEAR_TILE_MODE_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)

const io = computed(() =>
  buildNodeIoFromSlots(WEAR_NODE_IO_SLOTS, null, {
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
  if (port === 'image' || port === 'imageA' || port === 'imageB' || port === 'mask') {
    emit('port-mousedown', event, port)
  }
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.wear-node-body {
  gap: 7px;
}

.wear-tile-card {
  gap: 6px;
}

.wear-control-group {
  gap: 6px;
}

.wear-control-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.wear-control-row .node-control-label {
  width: 56px;
  flex: 0 0 56px;
  font-size: 10.5px;
  letter-spacing: 0.02em;
}

.wear-slider {
  flex: 1;
  min-width: 0;
}

</style>
