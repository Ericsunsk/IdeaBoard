<template>
  <section class="image-editor-panel image-editor-properties-panel">
    <div v-if="adjustmentLayer?.kind === 'hsl'" class="image-editor-panel__body">
      <ImageEditorSlider
        label="色相"
        :value="adjustmentLayer.params.hue"
        :min="-180"
        :max="180"
        unit="deg"
        @update:value="emit('update-param', 'hue', $event)"
      />
      <ImageEditorSlider
        label="饱和度"
        :value="adjustmentLayer.params.saturation"
        :min="-100"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'saturation', $event)"
      />
      <ImageEditorSlider
        label="亮度"
        :value="adjustmentLayer.params.brightness"
        :min="-100"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'brightness', $event)"
      />
      <ImageEditorSlider
        label="对比度"
        :value="adjustmentLayer.params.contrast"
        :min="-100"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'contrast', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'threshold'" class="image-editor-panel__body">
      <ImageEditorSlider
        label="阈值"
        :value="adjustmentLayer.params.threshold"
        :min="0"
        :max="255"
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
      <ImageEditorSlider
        label="输入"
        :value="adjustmentLayer.params.input"
        :min="0"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'input', $event)"
      />
      <ImageEditorSlider
        label="输出"
        :value="adjustmentLayer.params.output"
        :min="0"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'output', $event)"
      />
    </div>

    <div v-else-if="adjustmentLayer?.kind === 'levels'" class="image-editor-panel__body">
      <ImageEditorSlider
        label="黑场"
        :value="adjustmentLayer.params.shadows"
        :min="0"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'shadows', $event)"
      />
      <ImageEditorSlider
        label="中间"
        :value="adjustmentLayer.params.midtones"
        :min="0"
        :max="100"
        unit="%"
        @update:value="emit('update-param', 'midtones', $event)"
      />
      <ImageEditorSlider
        label="白场"
        :value="adjustmentLayer.params.highlights"
        :min="0"
        :max="100"
        unit="%"
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

    <div v-else class="image-editor-empty-state">选择一个调整图层</div>
  </section>
</template>

<script setup lang="ts">
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
} from '../../types/imageEditor'
import ImageEditorSlider from './ImageEditorSlider.vue'

defineProps<{
  adjustmentLayer: ImageEditorAdjustmentLayer | null
}>()

const emit = defineEmits<{
  (e: 'update-param', key: ImageEditorParamKey, value: ImageEditorParamValue): void
}>()

function onBooleanInput(key: ImageEditorParamKey, event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update-param', key, !!target?.checked)
}
</script>

<style scoped src="./styles/image-editor-panel.css"></style>
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
