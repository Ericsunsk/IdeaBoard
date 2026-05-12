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
      title="Enhance"
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
          <path d="M12 3l2.7 5.3L20 11l-5.3 2.7L12 19l-2.7-5.3L4 11l5.3-2.7L12 3z" />
        </svg>
      </template>
    </NodeHeader>

    <div class="node-execution-body" @mousedown.stop @contextmenu.stop>
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
          <div class="mt-px flex items-center">
            <input
              class="node-inline-range m-0 min-w-0 flex-1"
              :style="{ '--slider-pct': `${((node.outputQuality - 30) / 70) * 100}%` }"
              type="range"
              min="30"
              max="100"
              step="1"
              :value="node.outputQuality"
              :disabled="node.outputFormat === 'png'"
              @input="
                emit('update:output-quality', Number(($event.target as HTMLInputElement).value))
              "
            />
          </div>
        </template>
        <div v-else class="node-control-group">
          <div class="flex items-center gap-1.5">
            <span class="node-control-label min-w-9 text-[10.5px] tracking-[0.02em]">Top</span>
            <input
              class="node-inline-range min-w-0 flex-1"
              :style="{ '--slider-pct': `${toExtendSliderPct(node.extendTop)}%` }"
              type="range"
              :min="extendMin"
              :max="extendMax"
              step="1"
              :value="node.extendTop"
              @input="emit('update:extend-top', Number(($event.target as HTMLInputElement).value))"
            />
            <span class="min-w-8 text-right text-[10px] text-[#a1a1aa]">{{ node.extendTop }}%</span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="node-control-label min-w-9 text-[10.5px] tracking-[0.02em]">Right</span>
            <input
              class="node-inline-range min-w-0 flex-1"
              :style="{ '--slider-pct': `${toExtendSliderPct(node.extendRight)}%` }"
              type="range"
              :min="extendMin"
              :max="extendMax"
              step="1"
              :value="node.extendRight"
              @input="
                emit('update:extend-right', Number(($event.target as HTMLInputElement).value))
              "
            />
            <span class="min-w-8 text-right text-[10px] text-[#a1a1aa]"
              >{{ node.extendRight }}%</span
            >
          </div>
          <div class="flex items-center gap-1.5">
            <span class="node-control-label min-w-9 text-[10.5px] tracking-[0.02em]">Bottom</span>
            <input
              class="node-inline-range min-w-0 flex-1"
              :style="{ '--slider-pct': `${toExtendSliderPct(node.extendBottom)}%` }"
              type="range"
              :min="extendMin"
              :max="extendMax"
              step="1"
              :value="node.extendBottom"
              @input="
                emit('update:extend-bottom', Number(($event.target as HTMLInputElement).value))
              "
            />
            <span class="min-w-8 text-right text-[10px] text-[#a1a1aa]"
              >{{ node.extendBottom }}%</span
            >
          </div>
          <div class="flex items-center gap-1.5">
            <span class="node-control-label min-w-9 text-[10.5px] tracking-[0.02em]">Left</span>
            <input
              class="node-inline-range min-w-0 flex-1"
              :style="{ '--slider-pct': `${toExtendSliderPct(node.extendLeft)}%` }"
              type="range"
              :min="extendMin"
              :max="extendMax"
              step="1"
              :value="node.extendLeft"
              @input="emit('update:extend-left', Number(($event.target as HTMLInputElement).value))"
            />
            <span class="min-w-8 text-right text-[10px] text-[#a1a1aa]"
              >{{ node.extendLeft }}%</span
            >
          </div>
        </div>
      </div>

      <NodeExecutionFooter
        :items="connectionChips"
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '增强'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  ENHANCE_EXTEND_PERCENT_MAX,
  ENHANCE_EXTEND_PERCENT_MIN,
  type EnhanceNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

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
  (e: 'update:extend-top', value: number): void
  (e: 'update:extend-right', value: number): void
  (e: 'update:extend-bottom', value: number): void
  (e: 'update:extend-left', value: number): void
  (e: 'generate'): void
}>()

const enhanceModeOptions = ENHANCE_MODE_OPTIONS
const enhanceUpscaleModeOptions = ENHANCE_UPSCALE_MODE_OPTIONS
const enhanceUpscaleFactorOptions = ENHANCE_UPSCALE_FACTOR_OPTIONS
const enhanceOutputFormatOptions = ENHANCE_OUTPUT_FORMAT_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)

const extendMin = ENHANCE_EXTEND_PERCENT_MIN
const extendMax = ENHANCE_EXTEND_PERCENT_MAX
const isUpscaleMode = computed(() => props.node.mode === 'hd-upscale')
const supportsPrompt = computed(() => props.node.mode === 'canvas-extend')
const supportsMask = computed(() => props.node.mode === 'hd-upscale')
const io = computed(() =>
  buildNodeIoFromSlots(
    ENHANCE_NODE_IO_SLOTS,
    { supportsPrompt: supportsPrompt.value, supportsMask: supportsMask.value },
    {
      connected: {
        image: props.uiState?.connection?.connected?.image,
        mask: props.uiState?.connection?.connected?.mask,
        prompt: props.uiState?.connection?.connected?.prompt,
        output: props.uiState?.connection?.connected?.output,
      },
      pending: {
        image: props.uiState?.connection?.pending?.image,
        mask: props.uiState?.connection?.pending?.mask,
      },
      errorPortKeys: props.errorPortKeys,
    },
  ),
)
const connectionChips = computed(() => io.value.connectionChips)

function toExtendSliderPct(value: number): number {
  return ((value - extendMin) / (extendMax - extendMin)) * 100
}
</script>

<style scoped src="./node-shared.css"></style>
