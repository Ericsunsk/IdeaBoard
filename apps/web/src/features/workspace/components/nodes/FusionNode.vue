<template>
  <div
    class="workflow-node fusion-node"
    data-node-kind="fusion"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Fusion"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.1"
        >
          <path
            d="M8 6.5h4l4 4v7a1.5 1.5 0 0 1-1.5 1.5h-6A1.5 1.5 0 0 1 7 17.5v-9A2 2 0 0 1 8 6.5z"
          />
          <path d="M12 6.5v4h4" />
          <path d="M9 14c1.5-1.4 4.5-1.4 6 0" />
        </svg>
      </template>
    </NodeHeader>

    <div class="fusion-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="fusion-control-group node-control-group">
        <div class="fusion-control-row">
          <span class="node-control-label">Image A</span>
          <input
            class="fusion-slider node-inline-range"
            :style="{ '--slider-pct': `${imageAWeight}%` }"
            type="range"
            min="0"
            max="100"
            step="1"
            :value="imageAWeight"
            @input="
              emit(
                'update:blend-ratio',
                toNumber(($event.target as HTMLInputElement).value, imageAWeight),
              )
            "
          />
          <span class="fusion-slider-value">{{ imageAWeight }}%</span>
        </div>
        <div class="fusion-control-row">
          <span class="node-control-label">Image B</span>
          <input
            class="fusion-slider node-inline-range"
            :style="{ '--slider-pct': `${imageBWeight}%` }"
            type="range"
            min="0"
            max="100"
            step="1"
            :value="imageBWeight"
            @input="
              emit(
                'update:blend-ratio',
                100 - toNumber(($event.target as HTMLInputElement).value, imageBWeight),
              )
            "
          />
          <span class="fusion-slider-value">{{ imageBWeight }}%</span>
        </div>
      </div>

      <NodeExecutionFooter
        :items="connectionChips"
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '融合'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { FusionNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

import { buildNodeIoFromSlots, FUSION_NODE_IO_SLOTS } from './config/nodeIoConfig'
import { toNumber } from './utils/numberUtils'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: FusionNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:blend-ratio', value: number): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(FUSION_NODE_IO_SLOTS, null, {
    connected: {
      imageA: props.uiState?.connection?.connected?.imageA,
      imageB: props.uiState?.connection?.connected?.imageB,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      imageA: props.uiState?.connection?.pending?.imageA,
      imageB: props.uiState?.connection?.pending?.imageB,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const imageAWeight = computed(() => props.node.blendRatio)
const imageBWeight = computed(() => 100 - props.node.blendRatio)
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.fusion-node-body {
  gap: 8px;
}

.fusion-control-group {
  gap: 6px;
}

.fusion-control-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.fusion-control-row .node-control-label {
  min-width: 46px;
}

.fusion-slider {
  flex: 1;
  min-width: 0;
}

.fusion-slider-value {
  min-width: 42px;
  text-align: right;
  font-size: 10px;
  font-weight: 700;
  color: #7b8798;
}
</style>
