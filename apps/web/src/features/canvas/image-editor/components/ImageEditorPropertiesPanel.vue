<template>
  <section class="editing-controls-panel image-editor-properties-panel">
    <div v-if="adjustmentLayer?.kind === 'hsl'" class="editing-controls-panel__body">
      <ImageEditorSlider
        :label="ranges.hsl.ranges.hue.label"
        :value="adjustmentLayer.params.hue"
        :min="ranges.hsl.ranges.hue.min"
        :max="ranges.hsl.ranges.hue.max"
        track-variant="hue"
        @update:value="emit('update-param', 'hue', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.saturation.label"
        :value="adjustmentLayer.params.saturation"
        :min="ranges.hsl.ranges.saturation.min"
        :max="ranges.hsl.ranges.saturation.max"
        track-variant="tonal-reverse"
        @update:value="emit('update-param', 'saturation', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.lightness.label"
        :value="adjustmentLayer.params.lightness"
        :min="ranges.hsl.ranges.lightness.min"
        :max="ranges.hsl.ranges.lightness.max"
        track-variant="tonal"
        @update:value="emit('update-param', 'lightness', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'threshold'" class="editing-controls-panel__body">
      <ImageEditorThresholdControl
        :params="adjustmentLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'curves'" class="editing-controls-panel__body">
      <ImageEditorCurvesControl
        :params="adjustmentLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'levels'" class="editing-controls-panel__body">
      <ImageEditorLevelsControl
        :params="adjustmentLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'sharpen'" class="editing-controls-panel__body">
      <ImageEditorSharpenControl
        :params="adjustmentLayer.params"
        @update:params="emit('update-params', $event)"
      />
    </div>

    <div v-else class="editing-controls-empty-state">{{ t("canvas.imageEditor.selectEditingLayer") }}</div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type {
  ImageEditorAdjustmentWholeParams,
  ImageEditorAdjustmentLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
} from '../types'
import ImageEditorCurvesControl from './ImageEditorCurvesControl.vue'
import ImageEditorLevelsControl from './ImageEditorLevelsControl.vue'
import ImageEditorSharpenControl from './ImageEditorSharpenControl.vue'
import ImageEditorSlider from './ImageEditorSlider.vue'
import ImageEditorThresholdControl from './ImageEditorThresholdControl.vue'
import { IMAGE_ADJUSTMENT_DEFINITIONS } from '../utils/imageAdjustmentDefinitions'

defineProps<{
  adjustmentLayer: ImageEditorAdjustmentLayer | null
  histogramSource: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update-param', key: ImageEditorParamKey, value: ImageEditorParamValue): void
  (e: 'update-params', value: ImageEditorAdjustmentWholeParams): void
}>()

const { t } = useI18n()

const ranges = IMAGE_ADJUSTMENT_DEFINITIONS
</script>

<style scoped>
.image-editor-properties-panel {
  min-height: 0;
}
</style>
