<template>
  <label class="image-editor-slider-row">
    <span class="image-editor-slider-row__label">{{ label }}</span>
    <input
      class="workspace-range-input"
      type="range"
      :min="min"
      :max="max"
      step="1"
      :value="value"
      @input="onInput"
    />
    <span class="image-editor-slider-row__value">{{ value }}{{ unit }}</span>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: number
  min: number
  max: number
  unit?: string
}>()

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

function onInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update:value', Number(target?.value) || 0)
}
</script>

<style scoped>
.image-editor-slider-row {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) 36px;
  align-items: center;
  gap: 10px;
  min-height: var(--workspace-range-input-height);
  color: var(--workspace-text-body);
  font-size: var(--image-editor-panel-text-size);
  font-weight: 500;
}

.image-editor-slider-row__label {
  line-height: 1.1;
  text-align: center;
  white-space: nowrap;
}

.image-editor-slider-row__value {
  color: var(--workspace-text-muted);
  font-size: var(--image-editor-panel-meta-text-size);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}

</style>
