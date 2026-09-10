<template>
  <div class="editing-controls-stack retouch-sharpen-control">
    <RetouchSlider
      :label="ranges.amount.label"
      :value="normalizedParams.amount"
      :min="ranges.amount.min"
      :max="ranges.amount.max"
      track-variant="tonal-reverse"
      @update:value="updateNumberParam('amount', $event)"
    />
    <RetouchSlider
      :label="ranges.radius.label"
      :value="normalizedParams.radius"
      :min="ranges.radius.min"
      :max="ranges.radius.max"
      :step="0.1"
      track-variant="tonal"
      @update:value="updateNumberParam('radius', $event)"
    />
    <RetouchSlider
      :label="ranges.threshold.label"
      :value="normalizedParams.threshold"
      :min="ranges.threshold.min"
      :max="ranges.threshold.max"
      track-variant="tonal"
      @update:value="updateNumberParam('threshold', $event)"
    />
    <div class="editing-controls-toggle-row">
      <span>{{ t('canvas.retouch.protectColors') }}</span>
      <button
        class="editing-controls-toggle-button canvas-tool-tooltip-trigger"
        type="button"
        :aria-pressed="normalizedParams.protectColor ? 'true' : 'false'"
        :aria-label="t('canvas.retouch.protectSharpenColorsAria')"
        @click="updateProtectColor(!normalizedParams.protectColor)"
      >
        <ColorFilter aria-hidden="true" />
        <span class="canvas-tool-tooltip" aria-hidden="true">{{
          t('canvas.retouch.protectColors')
        }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import { ColorFilter } from '@iconoir/vue'
import type { RetouchSharpenParams } from '../model.ts'
import { IMAGE_RETOUCH_DEFINITIONS } from '../processing/definitions.ts'
import RetouchSlider from './Slider.vue'

const props = defineProps<{
  params: RetouchSharpenParams
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchSharpenParams): void
}>()

const { t } = useI18n()

const ranges = IMAGE_RETOUCH_DEFINITIONS.sharpen.ranges
const normalizedParams = computed(() => IMAGE_RETOUCH_DEFINITIONS.sharpen.normalize(props.params))

function updateNumberParam(
  key: Exclude<keyof RetouchSharpenParams, 'protectColor'>,
  value: number,
) {
  emitNormalizedParams({ ...normalizedParams.value, [key]: value })
}

function updateProtectColor(protectColor: boolean) {
  emitNormalizedParams({ ...normalizedParams.value, protectColor })
}

function emitNormalizedParams(params: RetouchSharpenParams) {
  emit('update:params', IMAGE_RETOUCH_DEFINITIONS.sharpen.normalize(params))
}
</script>
