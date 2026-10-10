<template>
  <section class="editing-controls-panel retouch-properties-panel">
    <div v-if="effectLayer?.kind === 'hsl'" class="editing-controls-panel__body">
      <RetouchSlider
        :label="t(ranges.hsl.ranges.hue.labelKey)"
        :value="effectLayer.params.hue"
        :min="ranges.hsl.ranges.hue.min"
        :max="ranges.hsl.ranges.hue.max"
        track-variant="hue"
        @update:value="emitSingleParam('hue', $event)"
      />
      <RetouchSlider
        :label="t(ranges.hsl.ranges.saturation.labelKey)"
        :value="effectLayer.params.saturation"
        :min="ranges.hsl.ranges.saturation.min"
        :max="ranges.hsl.ranges.saturation.max"
        track-variant="tonal-reverse"
        @update:value="emitSingleParam('saturation', $event)"
      />
      <RetouchSlider
        :label="t(ranges.hsl.ranges.lightness.labelKey)"
        :value="effectLayer.params.lightness"
        :min="ranges.hsl.ranges.lightness.min"
        :max="ranges.hsl.ranges.lightness.max"
        track-variant="tonal"
        @update:value="emitSingleParam('lightness', $event)"
      />
    </div>

    <div v-else-if="activeControl" class="editing-controls-panel__body">
      <component
        :is="activeControl"
        :params="effectLayer?.params"
        :histogram-source="histogramSourceForActiveControl"
        @update:params="emit('update:params', $event)"
      />
    </div>

    <div v-else class="editing-controls-empty-state">
      {{ t('canvas.retouch.selectEditingLayer') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  RetouchNonHslWholeParams,
  RetouchEffectLayer,
  RetouchEffectParamKey,
  RetouchEffectParamMap,
} from '../../core/model.ts'
import RetouchCurvesControl from '../curves/CurvesControl.vue'
import RetouchLevelsControl from '../levels/LevelsControl.vue'
import RetouchSharpenControl from '../sharpen/SharpenControl.vue'
import RetouchSlider from '../shared/Slider.vue'
import RetouchThresholdControl from '../threshold/ThresholdControl.vue'
import { IMAGE_RETOUCH_DEFINITIONS } from '../../core/definitions.ts'

const props = defineProps<{
  effectLayer: RetouchEffectLayer | null
  histogramSource: ImageData | null
}>()

const emit = defineEmits<{
  (
    e: 'update-param',
    key: RetouchEffectParamKey,
    value: RetouchEffectParamMap[RetouchEffectParamKey],
  ): void
  (e: 'update:params', value: RetouchNonHslWholeParams): void
}>()

const { t } = useI18n()

const ranges = IMAGE_RETOUCH_DEFINITIONS

/*
 * kind -> control component map for the four effect controls behind one
 * `component :is`. The hsl branch above stays inline (three sliders); TS
 * narrowing is intentionally looser here since the dynamic component accepts
 * any of the four params shapes. Sharpen takes no histogram input, so its
 * binding resolves to undefined (no attr rendered) while keeping the
 * `:histogram-source` wiring the contract spec pins.
 */
const RETOUCH_CONTROL_COMPONENTS: Record<string, Component> = {
  threshold: RetouchThresholdControl,
  curves: RetouchCurvesControl,
  levels: RetouchLevelsControl,
  sharpen: RetouchSharpenControl,
}

const activeControl = computed(() => {
  const kind = props.effectLayer?.kind
  if (!kind || kind === 'hsl') return null
  return RETOUCH_CONTROL_COMPONENTS[kind] ?? null
})

const histogramSourceForActiveControl = computed(() =>
  props.effectLayer?.kind === 'sharpen' ? undefined : props.histogramSource,
)

function emitSingleParam<TKey extends RetouchEffectParamKey>(
  key: TKey,
  value: RetouchEffectParamMap[TKey],
) {
  emit('update-param', key, value)
}
</script>

<style scoped>
.retouch-properties-panel {
  min-height: 0;
}
</style>
