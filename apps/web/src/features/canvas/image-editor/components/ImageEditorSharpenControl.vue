<template>
  <div class="editing-controls-stack image-editor-sharpen-control">
    <ImageEditorSlider
      :label="ranges.amount.label"
      :value="normalizedParams.amount"
      :min="ranges.amount.min"
      :max="ranges.amount.max"
      track-variant="tonal-reverse"
      @update:value="updateNumberParam('amount', $event)"
    />
    <ImageEditorSlider
      :label="ranges.radius.label"
      :value="normalizedParams.radius"
      :min="ranges.radius.min"
      :max="ranges.radius.max"
      :step="0.1"
      @update:value="updateNumberParam('radius', $event)"
    />
    <ImageEditorSlider
      :label="ranges.threshold.label"
      :value="normalizedParams.threshold"
      :min="ranges.threshold.min"
      :max="ranges.threshold.max"
      @update:value="updateNumberParam('threshold', $event)"
    />
    <div class="editing-controls-toggle-row">
      <span>保护颜色</span>
      <button
        class="editing-controls-toggle-button node-tool-tooltip-trigger"
        type="button"
        :aria-pressed="normalizedParams.protectColor ? 'true' : 'false'"
        aria-label="保护锐化颜色"
        @click="updateProtectColor(!normalizedParams.protectColor)"
      >
        <ColorFilter aria-hidden="true" />
        <span class="node-tool-tooltip" aria-hidden="true">保护颜色</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ColorFilter } from '@iconoir/vue'
import type { ImageEditorSharpenParams } from '../types'
import { IMAGE_ADJUSTMENT_DEFINITIONS } from '../utils/imageAdjustmentDefinitions'
import ImageEditorSlider from './ImageEditorSlider.vue'

const props = defineProps<{
  params: ImageEditorSharpenParams
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorSharpenParams): void
}>()

const ranges = IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.ranges
const normalizedParams = computed(() => IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.normalize(props.params))

function updateNumberParam(
  key: Exclude<keyof ImageEditorSharpenParams, 'protectColor'>,
  value: number,
) {
  emitNormalizedParams({ ...normalizedParams.value, [key]: value })
}

function updateProtectColor(protectColor: boolean) {
  emitNormalizedParams({ ...normalizedParams.value, protectColor })
}

function emitNormalizedParams(params: ImageEditorSharpenParams) {
  emit('update:params', IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.normalize(params))
}
</script>
