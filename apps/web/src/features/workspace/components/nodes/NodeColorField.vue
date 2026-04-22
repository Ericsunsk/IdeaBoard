<template>
  <div class="node-color-field">
    <button
      type="button"
      class="node-color-field-swatch-btn"
      :style="{ background: value }"
      :aria-label="swatchAriaLabel"
      @click.stop="emit('swatch-click')"
    >
      <NodeColorSwatch :color="value" :size="30" borderless />
    </button>
    <input
      :value="value"
      class="node-color-field-input"
      type="text"
      maxlength="7"
      :placeholder="placeholder"
      @input="emit('update:value', ($event.target as HTMLInputElement).value)"
      @change="emit('change:value', ($event.target as HTMLInputElement).value)"
    />
  </div>
</template>

<script setup lang="ts">
import NodeColorSwatch from './NodeColorSwatch.vue'

withDefaults(defineProps<{
  value: string
  placeholder?: string
  swatchAriaLabel?: string
}>(), {
  placeholder: '#000000',
  swatchAriaLabel: '选择颜色',
})

const emit = defineEmits<{
  (e: 'swatch-click'): void
  (e: 'update:value', value: string): void
  (e: 'change:value', value: string): void
}>()
</script>

<style scoped>
.node-color-field {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
}

.node-color-field-swatch-btn {
  width: 30px;
  height: 30px;
  border: 1px solid #d6dcec;
  border-radius: 999px;
  background: #fff;
  appearance: none;
  padding: 0;
  overflow: hidden;
  cursor: pointer;
  flex-shrink: 0;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.node-color-field-swatch-btn:hover {
  border-color: #b6a6f0;
  box-shadow: 0 0 0 3px rgba(122, 107, 213, 0.12);
}

.node-color-field-input {
  flex: 1;
  min-width: 0;
  height: 30px;
  border-radius: 8px;
  border: 1px solid #d6dcec;
  background: #fff;
  color: #425067;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  padding: 0 8px;
  outline: none;
  text-transform: uppercase;
}

.node-color-field-input:focus {
  border-color: #9278e6;
}
</style>
