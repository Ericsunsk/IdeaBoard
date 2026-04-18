<template>
  <div
    class="workflow-node enhance-node"
    data-node-kind="enhance"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Enhance"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M12 3l2.7 5.3L20 11l-5.3 2.7L12 19l-2.7-5.3L4 11l5.3-2.7L12 3z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="enhance-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="enhance-mode-card node-mode-card">
        <div class="enhance-mode-row node-option-row">
          <NodeActionButton
            v-for="option in enhanceModeOptions"
            :key="option.value"
            variant="chip"
            :active="node.mode === option.value"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>

        <template v-if="isUpscaleMode">
          <div class="enhance-sub-row node-option-row">
            <NodeActionButton
              v-for="option in enhanceUpscaleModeOptions"
              :key="option.value"
              variant="chip"
              :active="node.upscaleMode === option.value"
              @click="emit('update:upscale-mode', option.value)"
            >
              {{ option.label }}
            </NodeActionButton>
          </div>
          <div class="enhance-sub-row node-option-row">
            <NodeActionButton
              v-for="factor in enhanceUpscaleFactorOptions"
              :key="factor"
              variant="chip"
              :active="node.upscaleFactor === factor"
              @click="emit('update:upscale-factor', factor as 2 | 3 | 4)"
            >
              {{ factor }}x
            </NodeActionButton>
            <NodeActionButton
              v-for="option in enhanceOutputFormatOptions"
              :key="option.value"
              variant="chip"
              :active="node.outputFormat === option.value"
              @click="emit('update:output-format', option.value)"
            >
              {{ option.label }}
            </NodeActionButton>
          </div>
          <div class="enhance-quality-row">
            <input
              class="enhance-quality-slider node-inline-range"
              :style="{ '--slider-pct': `${((node.outputQuality - 30) / 70) * 100}%` }"
              type="range"
              min="30"
              max="100"
              step="1"
              :value="node.outputQuality"
              :disabled="node.outputFormat === 'png'"
              @input="emit('update:output-quality', Number(($event.target as HTMLInputElement).value))"
            />
          </div>
        </template>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '生成' }}
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="enhance"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { EnhanceNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, ENHANCE_NODE_IO_SLOTS } from './nodeIoConfig'
import {
  ENHANCE_MODE_OPTIONS,
  ENHANCE_OUTPUT_FORMAT_OPTIONS,
  ENHANCE_UPSCALE_FACTOR_OPTIONS,
  ENHANCE_UPSCALE_MODE_OPTIONS,
} from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: EnhanceNodeItem
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
  (e: 'update:mode', value: EnhanceNodeItem['mode']): void
  (e: 'update:upscale-mode', value: EnhanceNodeItem['upscaleMode']): void
  (e: 'update:upscale-factor', value: EnhanceNodeItem['upscaleFactor']): void
  (e: 'update:output-format', value: EnhanceNodeItem['outputFormat']): void
  (e: 'update:output-quality', value: number): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const enhanceModeOptions = ENHANCE_MODE_OPTIONS
const enhanceUpscaleModeOptions = ENHANCE_UPSCALE_MODE_OPTIONS
const enhanceUpscaleFactorOptions = ENHANCE_UPSCALE_FACTOR_OPTIONS
const enhanceOutputFormatOptions = ENHANCE_OUTPUT_FORMAT_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)

const isUpscaleMode = computed(() => props.node.mode === 'pattern-upscale' || props.node.mode === 'hd-upscale')
const supportsPrompt = computed(() => props.node.mode === 'deblur' || props.node.mode === 'canvas-extend')
const io = computed(() =>
  buildNodeIoFromSlots(ENHANCE_NODE_IO_SLOTS, { supportsPrompt: supportsPrompt.value }, {
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
.enhance-node-body {
  gap: 8px;
}

.enhance-quality-row {
  display: flex;
  align-items: center;
  margin-top: 1px;
}

.enhance-quality-slider {
  flex: 1;
  margin: 0;
  min-width: 0;
}

</style>
