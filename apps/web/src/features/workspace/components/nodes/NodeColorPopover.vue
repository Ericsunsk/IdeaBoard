<template>
  <div class="node-color-popover" @mousedown.stop @click.stop>
    <div class="node-color-popover-top">
      <div class="node-color-popover-preview" :style="{ background: hex }"></div>
      <input
        :value="hex"
        class="node-color-popover-hex"
        type="text"
        maxlength="7"
        @input="emit('hex-input', $event)"
        @blur="emit('hex-commit')"
        @keydown.enter.prevent="emit('hex-commit')"
      />
    </div>
    <label class="node-color-popover-control">
      <span>Hue</span>
      <input
        :value="hue"
        class="node-color-popover-range hue"
        type="range"
        min="0"
        max="360"
        @input="emit('hue-input', toNumber(($event.target as HTMLInputElement).value, hue))"
      />
    </label>
    <label class="node-color-popover-control">
      <span>Saturation</span>
      <input
        :value="saturation"
        class="node-color-popover-range"
        type="range"
        min="0"
        max="100"
        @input="emit('saturation-input', toNumber(($event.target as HTMLInputElement).value, saturation))"
      />
    </label>
    <label class="node-color-popover-control">
      <span>Brightness</span>
      <input
        :value="brightness"
        class="node-color-popover-range"
        type="range"
        min="0"
        max="100"
        @input="emit('brightness-input', toNumber(($event.target as HTMLInputElement).value, brightness))"
      />
    </label>
    <div
      ref="spectrumRefEl"
      class="node-color-popover-spectrum"
      :style="spectrumStyle"
      @pointerdown.stop.prevent="emit('spectrum-pointerdown', $event)"
    >
      <span class="node-color-popover-spectrum-cursor" :style="cursorStyle"></span>
    </div>
    <div class="node-color-popover-swatches">
      <button
        v-for="swatch in quickSwatches"
        :key="swatch"
        type="button"
        class="node-color-popover-swatch"
        :style="{ background: swatch }"
        @click="emit('select-swatch', swatch)"
      ></button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { toNumber } from './numberUtils'

defineProps<{
  hex: string
  hue: number
  saturation: number
  brightness: number
  spectrumStyle: Record<string, string>
  cursorStyle: Record<string, string>
  quickSwatches: string[]
}>()

const emit = defineEmits<{
  (e: 'hex-input', event: Event): void
  (e: 'hex-commit'): void
  (e: 'hue-input', value: number): void
  (e: 'saturation-input', value: number): void
  (e: 'brightness-input', value: number): void
  (e: 'spectrum-pointerdown', event: PointerEvent): void
  (e: 'select-swatch', value: string): void
  (e: 'set-spectrum-ref', value: HTMLElement | null): void
}>()

const spectrumRefEl = ref<HTMLElement | null>(null)

watch(spectrumRefEl, (value) => {
  emit('set-spectrum-ref', value ?? null)
}, { immediate: true })
</script>

<style scoped>
.node-color-popover {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  width: 210px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid #dce2f1;
  background: #ffffff;
  z-index: 24;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.node-color-popover-top {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.node-color-popover-preview {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  flex-shrink: 0;
}

.node-color-popover-hex {
  flex: 1;
  min-width: 0;
  width: 100%;
  box-sizing: border-box;
  height: 28px;
  border-radius: 8px;
  border: 1px solid #d6dcec;
  background: #f9fbff;
  color: #425067;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  padding: 0 8px;
  outline: none;
  text-transform: uppercase;
}

.node-color-popover-hex:focus {
  border-color: #9278e6;
  background: #fff;
}

.node-color-popover-control {
  display: grid;
  grid-template-columns: 72px 1fr;
  align-items: center;
  gap: 8px;
}

.node-color-popover-control > span {
  font-size: 11px;
  color: #6b7688;
  font-weight: 600;
}

.node-color-popover-range {
  width: 100%;
  height: 14px;
  appearance: none;
  background: transparent;
  cursor: pointer;
}

.node-color-popover-range.hue {
  --range-accent: #a855f7;
}

.node-color-popover-range:not(.hue) {
  --range-accent: #7a6bd5;
}

.node-color-popover-range::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--range-accent) 0%, color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%);
}

.node-color-popover-range::-webkit-slider-thumb {
  appearance: none;
  width: 11px;
  height: 11px;
  border-radius: 999px;
  margin-top: -2.5px;
  background: var(--range-accent);
}

.node-color-popover-range::-webkit-slider-thumb:active {
  background: var(--range-accent);
}

.node-color-popover-range::-moz-range-track {
  height: 6px;
  border: none;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--range-accent) 0%, color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%);
}

.node-color-popover-range::-moz-range-thumb {
  width: 11px;
  height: 11px;
  border: none;
  border-radius: 999px;
  background: var(--range-accent);
}

.node-color-popover-range:active,
.node-color-popover-range:focus,
.node-color-popover-range::-webkit-slider-runnable-track:active,
.node-color-popover-range::-moz-range-track:active,
.node-color-popover-range::-moz-range-thumb:active {
  opacity: 1;
  filter: none;
}

.node-color-popover-spectrum {
  position: relative;
  width: 100%;
  height: 94px;
  border: 1px solid #d6dcec;
  border-radius: 10px;
  overflow: hidden;
  cursor: crosshair;
}

.node-color-popover-spectrum-cursor {
  position: absolute;
  width: 12px;
  height: 12px;
  border-radius: 999px;
  border: 2px solid #ffffff;
  box-shadow: 0 0 0 1px rgba(25, 31, 45, 0.48);
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.node-color-popover-swatches {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 6px;
  padding-top: 2px;
}

.node-color-popover-swatch {
  width: 16px;
  height: 16px;
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.08);
}
</style>
