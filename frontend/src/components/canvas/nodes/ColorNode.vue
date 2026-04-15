<template>
  <div
    class="workflow-node color-node"
    data-node-kind="color"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <div class="workflow-node-header" @mousedown.stop="emit('node-mousedown', $event)">
      <div class="node-header-title">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" style="margin-right: 6px; opacity: 0.8">
          <circle cx="12" cy="12" r="4.8" />
          <path d="M12 2.8v3M12 18.2v3M2.8 12h3M18.2 12h3M5.2 5.2l2.1 2.1M16.7 16.7l2.1 2.1M5.2 18.8l2.1-2.1M16.7 7.3l2.1-2.1" />
        </svg>
        Color
      </div>
      <button class="node-delete-btn" title="删除" @mousedown.stop @click.stop="emit('delete')">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>
    <div class="color-node-body" @mousedown.stop @contextmenu.stop>
      <div class="color-node-row">
        <div class="color-node-color-wrap">
          <button
            type="button"
            class="color-node-picker"
            :style="{ background: node.targetColor }"
            @click.stop="emit('toggle-popover')"
            aria-label="Open color editor"
          ></button>
          <input
            :value="node.targetColor"
            class="color-node-input"
            type="text"
            maxlength="7"
            @change="emit('target-color-change', $event)"
          />
          <div
            v-if="isPopoverOpen"
            class="color-popover"
            @mousedown.stop
            @click.stop
          >
            <div class="color-popover-top">
              <div class="color-popover-preview" :style="{ background: popoverHex }"></div>
              <input
                :value="popoverHex"
                class="color-popover-hex"
                type="text"
                maxlength="7"
                @input="emit('popover-hex-input', $event)"
                @blur="emit('popover-hex-commit')"
                @keydown.enter.prevent="emit('popover-hex-commit')"
              />
            </div>
            <label class="color-popover-control">
              <span>Hue</span>
              <input
                :value="popoverHue"
                class="color-popover-range hue"
                type="range"
                min="0"
                max="360"
                @input="emit('popover-hue-input', toNumber(($event.target as HTMLInputElement).value, popoverHue))"
              />
            </label>
            <label class="color-popover-control">
              <span>Saturation</span>
              <input
                :value="popoverSaturation"
                class="color-popover-range"
                type="range"
                min="0"
                max="100"
                @input="emit('popover-saturation-input', toNumber(($event.target as HTMLInputElement).value, popoverSaturation))"
              />
            </label>
            <label class="color-popover-control">
              <span>Lightness</span>
              <input
                :value="popoverLightness"
                class="color-popover-range"
                type="range"
                min="0"
                max="100"
                @input="emit('popover-lightness-input', toNumber(($event.target as HTMLInputElement).value, popoverLightness))"
              />
            </label>
            <div
              ref="spectrumRefEl"
              class="color-popover-spectrum"
              :style="spectrumStyle"
              @pointerdown.stop.prevent="emit('spectrum-pointerdown', $event)"
            >
              <span class="color-popover-spectrum-cursor" :style="cursorStyle"></span>
            </div>
            <div class="color-popover-swatches">
              <button
                v-for="swatch in quickSwatches"
                :key="swatch"
                type="button"
                class="color-popover-swatch"
                :style="{ background: swatch }"
                @click="emit('select-swatch', swatch)"
              ></button>
            </div>
          </div>
        </div>
      </div>
      <div class="color-node-tools">
        <NodeIconButton
          tone="accent"
          :size="30"
          :disabled="generating"
          title="吸管取色"
          @click="emit('eyedropper')"
        >
          <svg class="color-node-tool-icon" viewBox="0 0 242.63 240.1" fill="none" aria-hidden="true">
            <rect x="94.33" y="57.68" width="124.37" height="55.2" transform="translate(205.65 256.62) rotate(-134.59)" fill="currentColor" />
            <rect x="67.51" y="38.09" width="55.26" height="215.36" rx="27.63" ry="27.63" transform="translate(58.11 315.87) rotate(-134.59)" stroke="currentColor" stroke-width="21" />
            <rect x="162.01" y="-10.5" width="55.53" height="125.99" rx="27.77" ry="27.77" transform="translate(285.61 224.51) rotate(-134.59)" fill="currentColor" />
          </svg>
        </NodeIconButton>
        <input
          :value="node.strength"
          class="color-node-strength-range"
          :style="{ '--strength-pct': `${node.strength}%` }"
          type="range"
          min="0"
          max="100"
          @input="emit('strength-input', toNumber(($event.target as HTMLInputElement).value, node.strength))"
        />
      </div>
      <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
        应用换色
      </NodeActionButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ColorNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'
import NodeIconButton from './NodeIconButton.vue'

const props = defineProps<{
  node: ColorNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  isPopoverOpen: boolean
  popoverHex: string
  popoverHue: number
  popoverSaturation: number
  popoverLightness: number
  spectrumStyle: Record<string, string>
  cursorStyle: Record<string, string>
  quickSwatches: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'toggle-popover'): void
  (e: 'target-color-change', event: Event): void
  (e: 'popover-hex-input', event: Event): void
  (e: 'popover-hex-commit'): void
  (e: 'popover-hue-input', value: number): void
  (e: 'popover-saturation-input', value: number): void
  (e: 'popover-lightness-input', value: number): void
  (e: 'spectrum-pointerdown', event: PointerEvent): void
  (e: 'select-swatch', value: string): void
  (e: 'eyedropper'): void
  (e: 'strength-input', value: number): void
  (e: 'generate'): void
  (e: 'set-spectrum-ref', value: HTMLElement | null): void
}>()

const nodeStyle = computed(() => ({
  left: '50%',
  top: '50%',
  transform: `translate(${props.node.x}px, ${props.node.y}px)`,
  width: `${props.node.width}px`,
}))

const spectrumRefEl = ref<HTMLElement | null>(null)

watch(spectrumRefEl, (value) => {
  emit('set-spectrum-ref', value ?? null)
}, { immediate: true })

function toNumber(rawValue: string, fallback: number): number {
  const value = Number(rawValue)
  return Number.isFinite(value) ? value : fallback
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.workflow-node.color-node {
  overflow: visible;
}

.workflow-node.color-node .workflow-node-header {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}

.color-node-body {
  padding: 10px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.color-node-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.color-node-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}

.color-node-strength-range {
  flex: 1;
  min-width: 0;
  margin: 0;
  height: 14px;
  appearance: none;
  -webkit-appearance: none;
  background: transparent;
  cursor: pointer;
}

.color-node-strength-range::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 999px;
  background: linear-gradient(
    90deg,
    #9278e6 0%,
    #9278e6 var(--strength-pct),
    #EEF2F5 var(--strength-pct),
    #EEF2F5 100%
  );
}

.color-node-strength-range::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 14px;
  height: 14px;
  margin-top: -4px;
  border: none;
  border-radius: 999px;
  background: #9278e6;
  box-shadow: none;
}

.color-node-strength-range::-moz-range-track {
  height: 6px;
  border: none;
  border-radius: 999px;
  background: #EEF2F5;
}

.color-node-strength-range::-moz-range-progress {
  height: 6px;
  border: none;
  border-radius: 999px;
  background: #9278e6;
}

.color-node-strength-range::-moz-range-thumb {
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 999px;
  background: #9278e6;
  box-shadow: none;
}

.color-node-strength-range:focus {
  outline: none;
}

.color-node-strength-range:active,
.color-node-strength-range:focus,
.color-node-strength-range::-webkit-slider-runnable-track:active,
.color-node-strength-range::-moz-range-track:active,
.color-node-strength-range::-moz-range-thumb:active {
  opacity: 1;
  filter: none;
}

.color-node-tool-icon {
  width: 14px;
  height: 14px;
  display: block;
  object-fit: contain;
}

.color-node-color-wrap {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
}

.color-node-picker {
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
}

.color-node-picker:hover {
  border-color: #b6a6f0;
  box-shadow: 0 0 0 3px rgba(122, 107, 213, 0.12);
}

.color-node-input {
  flex: 1;
  height: 30px;
  border-radius: 8px;
  border: 1px solid #d6dcec;
  background: #fff;
  color: #425067;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  padding: 0 8px;
  outline: none;
  text-transform: uppercase;
}

.color-node-input:focus {
  border-color: #9278e6;
}

.color-popover {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  width: 210px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid #dce2f1;
  background: #ffffff;
  box-shadow: 0 12px 30px rgba(23, 31, 56, 0.12);
  z-index: 24;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.color-popover-top {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.color-popover-preview {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  flex-shrink: 0;
}

.color-popover-hex {
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
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  padding: 0 8px;
  outline: none;
  text-transform: uppercase;
}

.color-popover-hex:focus {
  border-color: #9278e6;
  background: #fff;
}

.color-popover-control {
  display: grid;
  grid-template-columns: 72px 1fr;
  align-items: center;
  gap: 8px;
}

.color-popover-control > span {
  font-size: 11px;
  color: #6b7688;
  font-weight: 600;
}

.color-popover-range {
  width: 100%;
  height: 14px;
  appearance: none;
  background: transparent;
  cursor: pointer;
}

.color-popover-range.hue {
  --range-accent: #a855f7;
}

.color-popover-range:not(.hue) {
  --range-accent: #7a6bd5;
}

.color-popover-range::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--range-accent) 0%, color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%);
}

.color-popover-range::-webkit-slider-thumb {
  appearance: none;
  width: 11px;
  height: 11px;
  border-radius: 999px;
  margin-top: -2.5px;
  background: var(--range-accent);
}

.color-popover-range::-webkit-slider-thumb:active {
  background: var(--range-accent);
}

.color-popover-range::-moz-range-track {
  height: 6px;
  border: none;
  border-radius: 999px;
  background: linear-gradient(90deg, var(--range-accent) 0%, color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%);
}

.color-popover-range::-moz-range-thumb {
  width: 11px;
  height: 11px;
  border: none;
  border-radius: 999px;
  background: var(--range-accent);
}

.color-popover-range:active,
.color-popover-range:focus,
.color-popover-range::-webkit-slider-runnable-track:active,
.color-popover-range::-moz-range-track:active,
.color-popover-range::-moz-range-thumb:active {
  opacity: 1;
  filter: none;
}

.color-popover-spectrum {
  position: relative;
  width: 100%;
  height: 94px;
  border: 1px solid #d6dcec;
  border-radius: 10px;
  overflow: hidden;
  cursor: crosshair;
}

.color-popover-spectrum-cursor {
  position: absolute;
  width: 12px;
  height: 12px;
  border-radius: 999px;
  border: 2px solid #ffffff;
  box-shadow: 0 0 0 1px rgba(25, 31, 45, 0.48);
  transform: translate(-50%, -50%);
  pointer-events: none;
}

.color-popover-swatches {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 6px;
  padding-top: 2px;
}

.color-popover-swatch {
  width: 16px;
  height: 16px;
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.08);
  padding: 0;
  cursor: pointer;
  transition: transform 0.16s ease;
}

.color-popover-swatch:hover {
  transform: scale(1.08);
}

</style>
