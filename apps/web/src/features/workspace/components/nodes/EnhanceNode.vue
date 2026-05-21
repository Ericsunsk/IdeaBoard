<template>
  <div
    class="workflow-node enhance-node"
    data-node-kind="enhance"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      :title="node.title || 'ENHANCE'"
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
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
          <path d="M12 3l2.7 5.3L20 11l-5.3 2.7L12 19l-2.7-5.3L4 11l5.3-2.7L12 3z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="enhance-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="enhance-mode-card node-mode-card">
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
      </div>

      <NodeExecutionFooter
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '增强'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { type EnhanceNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

import {
  ENHANCE_OUTPUT_FORMAT_OPTIONS,
  ENHANCE_UPSCALE_FACTOR_OPTIONS,
  ENHANCE_UPSCALE_MODE_OPTIONS,
} from './config/nodeOptionConfig'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: EnhanceNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:upscale-mode', value: EnhanceNodeItem['upscaleMode']): void
  (e: 'update:upscale-factor', value: EnhanceNodeItem['upscaleFactor']): void
  (e: 'update:output-format', value: EnhanceNodeItem['outputFormat']): void
  (e: 'update:output-quality', value: number): void
  (e: 'generate'): void
}>()

const enhanceUpscaleModeOptions = ENHANCE_UPSCALE_MODE_OPTIONS
const enhanceUpscaleFactorOptions = ENHANCE_UPSCALE_FACTOR_OPTIONS
const enhanceOutputFormatOptions = ENHANCE_OUTPUT_FORMAT_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)
</script>

<style scoped src="./styles/node-shared.css"></style>
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
