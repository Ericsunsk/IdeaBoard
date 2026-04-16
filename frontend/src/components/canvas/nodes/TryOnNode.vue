<template>
  <div
    class="workflow-node tryon-node"
    data-node-kind="tryon"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="TryOn"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M7 4h10l2 4-2.2 2.2-1.8-1.4V20H9V8.8L7.2 10.2 5 8l2-4z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="tryon-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="tryon-mode-card node-mode-card">
        <div class="tryon-mode-row node-option-row">
          <NodeActionButton
            v-for="option in tryOnModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.mode === option.value"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
        <div class="tryon-tile-row node-option-row">
          <NodeActionButton
            v-for="option in tryOnTileModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.tileMode === option.value"
            @click="emit('update:tile-mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
      </div>

      <div class="tryon-control-group node-control-group">
        <div class="tryon-control-row">
          <span class="node-control-label">Rotate</span>
          <input
            class="tryon-slider node-inline-range"
            :style="{ '--slider-pct': `${(node.rotation / 360) * 100}%` }"
            type="range"
            min="0"
            max="360"
            step="1"
            :value="node.rotation"
            @input="emit('update:rotation', Number(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="tryon-control-row">
          <span class="node-control-label">Scale</span>
          <input
            class="tryon-slider node-inline-range"
            :style="{ '--slider-pct': `${((node.scale - 10) / 290) * 100}%` }"
            type="range"
            min="10"
            max="300"
            step="5"
            :value="node.scale"
            @input="emit('update:scale', Number(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="tryon-control-row">
          <span class="node-control-label">Opacity</span>
          <input
            class="tryon-slider node-inline-range"
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

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
          生成
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="tryon"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TryOnNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, TRYON_NODE_IO_SLOTS } from './nodeIoConfig'
import { TRYON_MODE_OPTIONS, TRYON_TILE_MODE_OPTIONS } from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: TryOnNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  imageAInputConnected?: boolean
  imageBInputConnected?: boolean
  outputConnected?: boolean
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:mode', value: TryOnNodeItem['mode']): void
  (e: 'update:tile-mode', value: TryOnNodeItem['tileMode']): void
  (e: 'update:rotation', value: number): void
  (e: 'update:scale', value: number): void
  (e: 'update:opacity', value: number): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const tryOnModeOptions = TRYON_MODE_OPTIONS
const tryOnTileModeOptions = TRYON_TILE_MODE_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)

const baseLabel = computed(() => 'Base')
const layerLabel = computed(() => props.node.mode === 'pattern-on-body' ? 'Pattern' : 'Fabric')
const io = computed(() =>
  buildNodeIoFromSlots(TRYON_NODE_IO_SLOTS, {
    baseLabel: baseLabel.value,
    layerLabel: layerLabel.value,
  }, {
    connected: {
      imageA: props.imageAInputConnected,
      imageB: props.imageBInputConnected,
      output: props.outputConnected,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.tryon-node-body {
  gap: 7px;
}

.tryon-mode-card {
  gap: 6px;
}

.tryon-control-group {
  gap: 6px;
}

.tryon-control-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.tryon-control-row .node-control-label {
  min-width: 36px;
  font-size: 10.5px;
  letter-spacing: 0.02em;
}

.tryon-slider {
  flex: 1;
  min-width: 0;
}

</style>
