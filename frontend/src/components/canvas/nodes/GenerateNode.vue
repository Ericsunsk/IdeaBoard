<template>
  <div
    class="workflow-node generate-node"
    data-node-kind="generate"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Generate"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M4 6.5h7l2.5 3h6.5v8a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z" />
          <path d="M12 11v5" />
          <path d="M9.5 13.5h5" />
        </svg>
      </template>
    </NodeHeader>

    <div class="generate-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="generate-mode-card node-mode-card">
        <div class="generate-mode-row node-option-row">
          <NodeActionButton
            v-for="option in generateModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.mode === option.value"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>

        <div v-if="supportsStyleModel" class="generate-select-row">
          <select
            class="generate-style-select"
            :value="node.styleModel"
            @change="emit('update:style-model', ($event.target as HTMLSelectElement).value)"
          >
            <option value="">{{ stylesLoading ? 'Loading styles...' : 'Select style' }}</option>
            <option v-for="style in styleOptions" :key="style" :value="style">{{ style }}</option>
          </select>
        </div>

        <div class="generate-option-row node-option-row">
          <NodeActionButton
            v-for="ratio in ratioOptions"
            :key="ratio"
            variant="chip"
            :active="node.ratio === ratio"
            @click="emit('update:ratio', ratio)"
          >
            {{ ratio }}
          </NodeActionButton>
        </div>
        <div class="generate-option-row node-option-row">
          <NodeActionButton
            v-for="count in countOptions"
            :key="count"
            variant="chip"
            :active="node.count === count"
            @click="emit('update:count', count)"
          >
            {{ count }}
          </NodeActionButton>
          <NodeActionButton
            v-for="quality in qualityOptions"
            :key="quality"
            variant="chip"
            :active="node.quality === quality"
            @click="emit('update:quality', quality)"
          >
            {{ quality.toUpperCase() }}
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
      node-kind="generate"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { GenerateNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, GENERATE_NODE_IO_SLOTS } from './nodeIoConfig'
import {
  GENERATE_COUNT_OPTIONS,
  GENERATE_MODE_OPTIONS,
  GENERATE_QUALITY_OPTIONS,
  GENERATE_RATIO_OPTIONS,
} from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: GenerateNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
  styleOptions: string[]
  stylesLoading: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:mode', value: GenerateNodeItem['mode']): void
  (e: 'update:style-model', value: string): void
  (e: 'update:ratio', value: GenerateNodeItem['ratio']): void
  (e: 'update:count', value: GenerateNodeItem['count']): void
  (e: 'update:quality', value: GenerateNodeItem['quality']): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const generateModeOptions = GENERATE_MODE_OPTIONS
const ratioOptions = GENERATE_RATIO_OPTIONS
const countOptions = GENERATE_COUNT_OPTIONS
const qualityOptions = GENERATE_QUALITY_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)

const supportsStyleModel = computed(() => props.node.mode === 'pattern-gen' || props.node.mode === 'style-clone')
const io = computed(() =>
  buildNodeIoFromSlots(GENERATE_NODE_IO_SLOTS, null, {
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

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.generate-node-body {
  gap: 8px;
}

.generate-select-row {
  display: flex;
  align-items: center;
}

.generate-style-select {
  width: 100%;
  height: 28px;
  border: 1px solid #d8deea;
  border-radius: 10px;
  background: #fff;
  padding: 0 10px;
  font-size: 11px;
  color: #5f6b7a;
  outline: none;
}

.generate-style-select:focus {
  border-color: #7a6bd5;
}

</style>
