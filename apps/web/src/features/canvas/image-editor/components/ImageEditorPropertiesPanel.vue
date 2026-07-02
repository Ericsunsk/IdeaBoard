<template>
  <section class="editing-controls-panel image-editor-properties-panel">
    <div v-if="adjustmentLayer?.kind === 'hsl'" class="editing-controls-panel__body">
      <ImageEditorSlider
        :label="ranges.hsl.ranges.hue.label"
        :value="adjustmentLayer.params.hue"
        :min="ranges.hsl.ranges.hue.min"
        :max="ranges.hsl.ranges.hue.max"
        @update:value="emit('update-param', 'hue', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.saturation.label"
        :value="adjustmentLayer.params.saturation"
        :min="ranges.hsl.ranges.saturation.min"
        :max="ranges.hsl.ranges.saturation.max"
        @update:value="emit('update-param', 'saturation', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.lightness.label"
        :value="adjustmentLayer.params.lightness"
        :min="ranges.hsl.ranges.lightness.min"
        :max="ranges.hsl.ranges.lightness.max"
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

    <div v-else-if="adjustmentLayer?.kind === 'invert'" class="editing-controls-panel__body">
      <label class="editing-controls-toggle-row">
        <span>启用反相</span>
        <input
          type="checkbox"
          :checked="adjustmentLayer.params.enabled"
          @change="onBooleanInput('enabled', $event)"
        />
      </label>
    </div>

    <div v-else class="editing-controls-empty-state">选择一个编辑图层</div>
  </section>
</template>

<script setup lang="ts">
import type {
  ImageEditorAdjustmentWholeParams,
  ImageEditorAdjustmentLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
} from '../types'
import ImageEditorCurvesControl from './ImageEditorCurvesControl.vue'
import ImageEditorLevelsControl from './ImageEditorLevelsControl.vue'
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

const ranges = IMAGE_ADJUSTMENT_DEFINITIONS

function onBooleanInput(key: ImageEditorParamKey, event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update-param', key, !!target?.checked)
}
</script>

<style src="../../components/styles/editing-controls.css"></style>
<style scoped>
.image-editor-properties-panel {
  min-height: 0;
}
</style>
