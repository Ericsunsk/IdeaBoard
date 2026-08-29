<template>
  <div class="retouch-levels-output" :aria-label="t('canvas.retouch.outputLevelsLabel')">
    <div class="editing-controls-label retouch-levels-output__header">
      <span>{{ t("canvas.retouch.outputLevelsLabel") }}</span>
    </div>
    <EditingTwoHandleSlider
      class="retouch-levels-output__slider"
      :aria-label="t('canvas.retouch.outputLevelsRangeAria')"
      :min="RETOUCH_CHANNEL_MIN"
      :max="RETOUCH_CHANNEL_MAX"
      :step="1"
      :shift-step="10"
      :handles="outputSliderHandles"
      @update:handle="onOutputSliderHandleUpdate"
    />
    <div class="editing-controls-fields retouch-levels-output__fields">
      <label
        class="editing-controls-field editing-controls-field--compact retouch-levels-output__field"
      >
        <input
          name="retouch-output-black"
          class="editing-controls-control editing-controls-number-input"
          type="number"
          :min="constraints.outputBlack.min"
          :max="constraints.outputBlack.max"
          step="1"
          inputmode="numeric"
          :aria-label="t('canvas.retouch.params.outputBlack')"
          :aria-valuemin="constraints.outputBlack.min"
          :aria-valuemax="constraints.outputBlack.max"
          :value="params.outputBlack"
          @input="onOutputInput('outputBlack', $event)"
        />
      </label>
      <label
        class="editing-controls-field editing-controls-field--compact retouch-levels-output__field"
      >
        <input
          name="retouch-output-white"
          class="editing-controls-control editing-controls-number-input"
          type="number"
          :min="constraints.outputWhite.min"
          :max="constraints.outputWhite.max"
          step="1"
          inputmode="numeric"
          :aria-label="t('canvas.retouch.params.outputWhite')"
          :aria-valuemin="constraints.outputWhite.min"
          :aria-valuemax="constraints.outputWhite.max"
          :value="params.outputWhite"
          @input="onOutputInput('outputWhite', $event)"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import EditingTwoHandleSlider from '../../components/EditingTwoHandleSlider.vue'
import type { RetouchLevelsParams } from '../types'
import {
  moveLevelsOutputHandle,
  type RetouchLevelsConstraint,
  type RetouchLevelsOutputHandleId,
} from '../utils/retouchLevels'
import {
  RETOUCH_CHANNEL_MAX,
  RETOUCH_CHANNEL_MIN,
} from '../utils/retouchChannelScale'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  params: RetouchLevelsParams
  constraints: Record<RetouchLevelsOutputHandleId, RetouchLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchLevelsParams): void
}>()

const { t } = useI18n()

const outputSliderHandles = computed(() => [
  {
    id: 'outputBlack',
    value: props.params.outputBlack,
    min: props.constraints.outputBlack.min,
    max: props.constraints.outputBlack.max,
    ariaLabel: t('canvas.retouch.params.outputBlack'),
  },
  {
    id: 'outputWhite',
    value: props.params.outputWhite,
    min: props.constraints.outputWhite.min,
    max: props.constraints.outputWhite.max,
    ariaLabel: t('canvas.retouch.params.outputWhite'),
  },
])

function onOutputInput(handleId: RetouchLevelsOutputHandleId, event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:params', moveLevelsOutputHandle(props.params, handleId, value))
}

function onOutputSliderHandleUpdate(handleId: string, value: number) {
  if (!isOutputHandleId(handleId)) return
  emit('update:params', moveLevelsOutputHandle(props.params, handleId, value))
}

function isOutputHandleId(handleId: string): handleId is RetouchLevelsOutputHandleId {
  return handleId === 'outputBlack' || handleId === 'outputWhite'
}
</script>
