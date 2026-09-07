<template>
  <section class="editing-controls-panel retouch-properties-panel">
    <div v-if="effectLayer?.kind === 'hsl'" class="editing-controls-panel__body">
      <RetouchSlider
        :label="ranges.hsl.ranges.hue.label"
        :value="effectLayer.params.hue"
        :min="ranges.hsl.ranges.hue.min"
        :max="ranges.hsl.ranges.hue.max"
        track-variant="hue"
        @update:value="emit('update-param', 'hue', $event)"
      />
      <RetouchSlider
        :label="ranges.hsl.ranges.saturation.label"
        :value="effectLayer.params.saturation"
        :min="ranges.hsl.ranges.saturation.min"
        :max="ranges.hsl.ranges.saturation.max"
        track-variant="tonal-reverse"
        @update:value="emit('update-param', 'saturation', $event)"
      />
      <RetouchSlider
        :label="ranges.hsl.ranges.lightness.label"
        :value="effectLayer.params.lightness"
        :min="ranges.hsl.ranges.lightness.min"
        :max="ranges.hsl.ranges.lightness.max"
        track-variant="tonal"
        @update:value="emit('update-param', 'lightness', $event)"
      />
    </div>

    <div v-else-if="effectLayer?.kind === 'threshold'" class="editing-controls-panel__body">
      <RetouchThresholdControl
        :params="effectLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="effectLayer?.kind === 'curves'" class="editing-controls-panel__body">
      <RetouchCurvesControl
        :params="effectLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="effectLayer?.kind === 'levels'" class="editing-controls-panel__body">
      <RetouchLevelsControl
        :params="effectLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="effectLayer?.kind === 'sharpen'" class="editing-controls-panel__body">
      <RetouchSharpenControl
        :params="effectLayer.params"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else class="editing-controls-empty-state">
      {{ t('canvas.retouch.selectEditingLayer') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type {
  RetouchEffectWholeParams,
  RetouchEffectLayer,
  RetouchParamKey,
  RetouchParamValue,
} from '../types'
import RetouchCurvesControl from './RetouchCurvesControl.vue'
import RetouchLevelsControl from './RetouchLevelsControl.vue'
import RetouchSharpenControl from './RetouchSharpenControl.vue'
import RetouchSlider from './RetouchSlider.vue'
import RetouchThresholdControl from './RetouchThresholdControl.vue'
import { IMAGE_RETOUCH_DEFINITIONS } from '../utils/imageRetouchDefinitions'

defineProps<{
  effectLayer: RetouchEffectLayer | null
  histogramSource: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update-param', key: RetouchParamKey, value: RetouchParamValue): void
  (e: 'update-params', value: RetouchEffectWholeParams): void
}>()

const { t } = useI18n()

const ranges = IMAGE_RETOUCH_DEFINITIONS
</script>

<style scoped>
.retouch-properties-panel {
  min-height: 0;
}
</style>
