<template>
  <section class="image-editor-panel image-editor-properties-panel">
    <div v-if="adjustmentLayer?.kind === 'hsl'" class="image-editor-panel__body">
      <ImageEditorSlider
        :label="ranges.hsl.ranges.hue.label"
        :value="adjustmentLayer.params.hue"
        :min="ranges.hsl.ranges.hue.min"
        :max="ranges.hsl.ranges.hue.max"
        :unit="ranges.hsl.ranges.hue.unit"
        @update:value="emit('update-param', 'hue', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.saturation.label"
        :value="adjustmentLayer.params.saturation"
        :min="ranges.hsl.ranges.saturation.min"
        :max="ranges.hsl.ranges.saturation.max"
        :unit="ranges.hsl.ranges.saturation.unit"
        @update:value="emit('update-param', 'saturation', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.brightness.label"
        :value="adjustmentLayer.params.brightness"
        :min="ranges.hsl.ranges.brightness.min"
        :max="ranges.hsl.ranges.brightness.max"
        :unit="ranges.hsl.ranges.brightness.unit"
        @update:value="emit('update-param', 'brightness', $event)"
      />
      <ImageEditorSlider
        :label="ranges.hsl.ranges.contrast.label"
        :value="adjustmentLayer.params.contrast"
        :min="ranges.hsl.ranges.contrast.min"
        :max="ranges.hsl.ranges.contrast.max"
        :unit="ranges.hsl.ranges.contrast.unit"
        @update:value="emit('update-param', 'contrast', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'threshold'" class="image-editor-panel__body">
      <ImageEditorSlider
        :label="ranges.threshold.ranges.threshold.label"
        :value="adjustmentLayer.params.threshold"
        :min="ranges.threshold.ranges.threshold.min"
        :max="ranges.threshold.ranges.threshold.max"
        @update:value="emit('update-param', 'threshold', $event)"
      />
      <label class="image-editor-toggle-row">
        <span>反相</span>
        <input
          type="checkbox"
          :checked="adjustmentLayer.params.invert"
          @change="onBooleanInput('invert', $event)"
        />
      </label>
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'curves'" class="image-editor-panel__body">
      <ImageEditorCurvesControl
        :params="adjustmentLayer.params"
        :histogram-source="histogramSource"
        @update:params="emit('update-param', 'channels', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'levels'" class="image-editor-panel__body">
      <ImageEditorSlider
        :label="ranges.levels.ranges.shadows.label"
        :value="adjustmentLayer.params.shadows"
        :min="ranges.levels.ranges.shadows.min"
        :max="ranges.levels.ranges.shadows.max"
        :unit="ranges.levels.ranges.shadows.unit"
        @update:value="emit('update-param', 'shadows', $event)"
      />
      <ImageEditorSlider
        :label="ranges.levels.ranges.midtones.label"
        :value="adjustmentLayer.params.midtones"
        :min="ranges.levels.ranges.midtones.min"
        :max="ranges.levels.ranges.midtones.max"
        :unit="ranges.levels.ranges.midtones.unit"
        @update:value="emit('update-param', 'midtones', $event)"
      />
      <ImageEditorSlider
        :label="ranges.levels.ranges.highlights.label"
        :value="adjustmentLayer.params.highlights"
        :min="ranges.levels.ranges.highlights.min"
        :max="ranges.levels.ranges.highlights.max"
        :unit="ranges.levels.ranges.highlights.unit"
        @update:value="emit('update-param', 'highlights', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'invert'" class="image-editor-panel__body">
      <label class="image-editor-toggle-row">
        <span>启用反相</span>
        <input
          type="checkbox"
          :checked="adjustmentLayer.params.enabled"
          @change="onBooleanInput('enabled', $event)"
        />
      </label>
    </div>

    <div v-else class="image-editor-empty-state">选择一个编辑图层</div>
  </section>
</template>

<script setup lang="ts">
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
} from '../types'
import ImageEditorCurvesControl from './ImageEditorCurvesControl.vue'
import ImageEditorSlider from './ImageEditorSlider.vue'
import { IMAGE_ADJUSTMENT_DEFINITIONS } from '../utils/imageAdjustmentDefinitions'

defineProps<{
  adjustmentLayer: ImageEditorAdjustmentLayer | null
  histogramSource: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update-param', key: ImageEditorParamKey, value: ImageEditorParamValue): void
}>()

const ranges = IMAGE_ADJUSTMENT_DEFINITIONS

function onBooleanInput(key: ImageEditorParamKey, event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update-param', key, !!target?.checked)
}
</script>

<style src="./styles/image-editor-panel.css"></style>
<style scoped>
.image-editor-properties-panel {
  min-height: 0;
}

.image-editor-panel__body {
  padding: var(--image-editor-panel-content-gap) 12px;
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.image-editor-toggle-row {
  min-height: var(--image-editor-panel-control-height);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: var(--workspace-text-body);
  font-size: var(--image-editor-panel-text-size);
  font-weight: 500;
}

.image-editor-toggle-row input[type='checkbox'] {
  width: 15px;
  height: 15px;
  accent-color: var(--workspace-accent);
}

.image-editor-empty-state {
  min-height: 100px;
  display: grid;
  place-items: center;
  color: var(--workspace-text-subtle);
  font-size: var(--image-editor-panel-text-size);
}
</style>
