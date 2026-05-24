<template>
  <div
    class="image-mask-editor-chrome"
    :style="chromeStyle"
    data-image-mask-editor
    @pointerdown.stop
  >
    <div class="image-mask-editor-left-panel node-tool-surface">
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'paint'"
        aria-label-value="画笔"
        title="画笔"
        @click="emit('set-draw-mode', 'paint')"
      >
        <EditPencil />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'erase'"
        aria-label-value="橡皮擦"
        title="橡皮擦"
        @click="emit('set-draw-mode', 'erase')"
      >
        <Erase />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="填充（待支持）"
        title="填充（待支持）"
      >
        <FillColor />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="魔棒（待支持）"
        title="魔棒（待支持）"
      >
        <CircleSpark />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="智能蒙版（待支持）"
        title="智能蒙版（待支持）"
      >
        <Sparks />
      </NodeToolIconButton>
    </div>
    <div class="image-mask-editor-right-dock">
      <div class="image-mask-editor-panel image-mask-editor-right-panel">
        <section class="image-mask-editor-section">
          <div class="image-mask-editor-field">
            <span class="image-mask-editor-field-label image-mask-editor-field-label-muted">笔刷</span>
            <div class="image-mask-editor-segmented">
              <button
                type="button"
                class="image-mask-editor-segmented-btn"
                disabled
                aria-disabled="true"
                aria-label="圆形笔尖（待支持）"
                title="圆形笔尖（待支持）"
              >
                <Circle class="image-mask-editor-segmented-icon" />
              </button>
              <button
                type="button"
                class="image-mask-editor-segmented-btn"
                disabled
                aria-disabled="true"
                aria-label="方形笔尖（待支持）"
                title="方形笔尖（待支持）"
              >
                <Square class="image-mask-editor-segmented-icon" />
              </button>
            </div>
          </div>
          <label class="image-mask-editor-field">
            <span class="image-mask-editor-field-label">粗细</span>
            <div class="image-mask-editor-size-input-row">
              <div class="image-mask-editor-range-wrap" :style="rangeTooltipStyle(brushSize, 4, 48)">
                <input
                  type="range"
                  min="4"
                  max="48"
                  step="1"
                  :value="brushSize"
                  @input="onBrushSizeInput"
                />
                <span class="image-mask-editor-range-tooltip">{{ brushSize }}</span>
              </div>
            </div>
          </label>
          <label class="image-mask-editor-field image-mask-editor-field-disabled">
            <span class="image-mask-editor-field-label image-mask-editor-field-label-muted">硬度</span>
            <div class="image-mask-editor-size-input-row">
              <div class="image-mask-editor-range-wrap">
                <input type="range" min="0" max="100" step="1" :value="70" disabled />
              </div>
            </div>
          </label>
        </section>
        <section class="image-mask-editor-section">
          <label class="image-mask-editor-field">
            <span class="image-mask-editor-field-label">蒙版</span>
            <div class="image-mask-editor-size-input-row">
              <div
                class="image-mask-editor-range-wrap"
                :style="rangeTooltipStyle(maskLayerOpacity, 15, 100)"
              >
                <input
                  type="range"
                  min="15"
                  max="100"
                  step="1"
                  :value="maskLayerOpacity"
                  @input="onMaskOpacityInput"
                />
                <span class="image-mask-editor-range-tooltip">{{ maskLayerOpacity }}</span>
              </div>
            </div>
          </label>
        </section>
      </div>
      <div class="image-mask-editor-right-actions node-tool-surface">
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          aria-label-value="清空蒙版"
          title="清空蒙版"
          @click="emit('clear-mask')"
        >
          <XmarkCircle />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          aria-label-value="反选蒙版"
          title="反选蒙版"
          @click="emit('invert-mask')"
        >
          <PlusSquareDashed />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          disabled
          aria-label-value="羽化边缘（待支持）"
          title="羽化边缘（待支持）"
        >
          <CenterAlign />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          disabled
          aria-label-value="回退（待支持）"
          title="回退（待支持）"
        >
          <Undo />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          disabled
          aria-label-value="前进（待支持）"
          title="前进（待支持）"
        >
          <Redo />
        </NodeToolIconButton>
      </div>
      <PromptTextComposer
        v-model="promptDraft"
        class="image-mask-editor-composer"
        variant="floating"
        placeholder="描述如何重绘选区"
        :disabled="directGenerating"
        :output-count="outputCount"
        :image-inputs="referenceImages"
        :max-image-inputs="4"
        :show-aspect-ratio="false"
        show-image-input
        @update:output-count="emit('update:output-count', $event)"
        @image-input-open="emit('image-input-open')"
        @image-input-remove="emit('reference-image-remove', $event)"
        @image-input-preview="emit('reference-image-preview', $event)"
        @submit="onSubmit"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  Circle,
  CircleSpark,
  EditPencil,
  Erase,
  FillColor,
  CenterAlign,
  PlusSquareDashed,
  Redo,
  XmarkCircle,
  Sparks,
  Square,
  Undo,
} from '@iconoir/vue'
import type { ImageMaskDrawMode } from '../../composables/useImageMaskEditorOverlay'
import { clampImageNodeOutputCount } from '../../types/canvasNodes'
import { MASK_EDITOR_CHROME_LAYOUT } from '../../config/maskEditorChrome'
import NodeToolIconButton from './NodeToolIconButton.vue'
import PromptTextComposer from './PromptTextComposer.vue'

type MaskEditorReferenceImage = {
  id: string
  url: string
  ratio?: number
}

const props = defineProps<{
  drawMode: ImageMaskDrawMode
  brushSize: number
  maskLayerOpacity: number
  outputCount: number
  referenceImages: MaskEditorReferenceImage[]
  directGenerating?: boolean
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
}>()

const emit = defineEmits<{
  (e: 'set-draw-mode', mode: ImageMaskDrawMode): void
  (e: 'set-brush-size', value: number): void
  (e: 'set-mask-layer-opacity', value: number): void
  (e: 'clear-mask'): void
  (e: 'invert-mask'): void
  (e: 'update:output-count', value: number): void
  (e: 'image-input-open'): void
  (e: 'reference-image-remove', imageId: string): void
  (e: 'reference-image-preview', image: MaskEditorReferenceImage): void
  (e: 'submit', prompt: string, outputCount: number): void
}>()

const promptDraft = ref('')
const chromeStyle = computed(() => ({
  top: `${props.anchorRect.top}px`,
  left: `${props.anchorRect.left}px`,
  width: `${props.anchorRect.width}px`,
  height: `${props.anchorRect.height}px`,
  '--mask-editor-image-gap': `${MASK_EDITOR_CHROME_LAYOUT.imageGap}px`,
  '--mask-editor-stack-gap': `${MASK_EDITOR_CHROME_LAYOUT.stackGap}px`,
  '--mask-editor-right-panel-width': `${MASK_EDITOR_CHROME_LAYOUT.rightPanelWidth}px`,
  '--mask-editor-tool-button-size': `${MASK_EDITOR_CHROME_LAYOUT.toolButtonSize}px`,
  '--mask-editor-tool-button-gap': `${MASK_EDITOR_CHROME_LAYOUT.toolButtonGap}px`,
}))

function onBrushSizeInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-brush-size', Number(target?.value) || 18)
}

function onMaskOpacityInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-mask-layer-opacity', Number(target?.value) || 72)
}

function rangeTooltipStyle(value: number, min: number, max: number) {
  const ratio = max === min ? 0 : (value - min) / (max - min)
  const percent = Math.min(1, Math.max(0, ratio)) * 100
  return { '--mask-editor-range-value': `${percent}%` }
}

function onSubmit(prompt: string, outputCount: number) {
  emit('submit', prompt, clampImageNodeOutputCount(outputCount))
  promptDraft.value = ''
}
</script>

<style scoped>
@import './styles/node-tool-buttons.css';

.image-mask-editor-chrome {
  --mask-editor-image-gap: 12px;
  --mask-editor-stack-gap: 8px;
  --mask-editor-right-panel-width: 203px;
  --mask-editor-tool-border-width: 1.2px;
  --mask-editor-tool-button-size: 35px;
  --mask-editor-tool-button-radius: 10px;
  --mask-editor-tool-button-gap: 7px;
  --mask-editor-tool-icon-size: 21px;
  --mask-editor-tool-icon-stroke: 1.5;
  position: absolute;
  pointer-events: auto;
  z-index: 21;
  overflow: visible;
}

.image-mask-editor-left-panel {
  top: 0;
  left: calc((var(--mask-editor-tool-button-size) + var(--mask-editor-image-gap)) * -1);
  width: var(--mask-editor-tool-button-size);
  padding: 0;
  position: absolute;
  --node-tool-border-width: var(--mask-editor-tool-border-width);
  --node-tool-button-size: var(--mask-editor-tool-button-size);
  --node-tool-button-radius: var(--mask-editor-tool-button-radius);
  --node-tool-button-gap: var(--mask-editor-tool-button-gap);
  --node-tool-icon-size: var(--mask-editor-tool-icon-size);
  --node-tool-icon-stroke: var(--mask-editor-tool-icon-stroke);
  border-radius: 0;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
  justify-content: flex-start;
  animation: mask-editor-slide-left-in 220ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

.image-mask-editor-right-dock {
  top: 0;
  right: calc((var(--mask-editor-right-panel-width) + var(--mask-editor-image-gap)) * -1);
  width: var(--mask-editor-right-panel-width);
  position: absolute;
  display: flex;
  flex-direction: column;
  gap: var(--mask-editor-stack-gap);
  animation: mask-editor-slide-right-in 240ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

.image-mask-editor-panel {
  border: var(--mask-editor-tool-border-width) solid rgba(122, 107, 213, 0.22);
  border-radius: 13px;
  background: #ffffff;
  box-sizing: border-box;
}

.image-mask-editor-right-panel {
  width: 100%;
  min-height: 0;
  overflow: visible;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  animation: mask-editor-soft-pop-in 220ms cubic-bezier(0.2, 0.8, 0.2, 1) 40ms both;
}

.image-mask-editor-tool-btn {
  width: 100%;
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-primary-hover-border: #6c5cb8;
  --node-tool-icon-button-primary-hover-bg: #6c5cb8;
  --node-tool-icon-button-primary-hover-color: #ffffff;
}

.image-mask-editor-right-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--node-tool-button-gap);
  --node-tool-border-width: var(--mask-editor-tool-border-width);
  --node-tool-button-size: var(--mask-editor-tool-button-size);
  --node-tool-button-radius: var(--mask-editor-tool-button-radius);
  --node-tool-button-gap: var(--mask-editor-tool-button-gap);
  --node-tool-icon-size: var(--mask-editor-tool-icon-size);
  --node-tool-icon-stroke: var(--mask-editor-tool-icon-stroke);
  animation: mask-editor-soft-pop-in 220ms cubic-bezier(0.2, 0.8, 0.2, 1) 75ms both;
}

.image-mask-editor-right-actions .image-mask-editor-tool-btn {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
}

.image-mask-editor-composer {
  width: 100%;
  min-height: var(--mask-editor-right-panel-width);
  aspect-ratio: 1 / 1;
  animation: mask-editor-soft-pop-in 220ms cubic-bezier(0.2, 0.8, 0.2, 1) 110ms both;
}

@keyframes mask-editor-slide-left-in {
  from {
    opacity: 0;
    transform: translateX(8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes mask-editor-slide-right-in {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes mask-editor-soft-pop-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .image-mask-editor-left-panel,
  .image-mask-editor-right-dock,
  .image-mask-editor-right-panel,
  .image-mask-editor-right-actions,
  .image-mask-editor-composer {
    animation: none;
  }
}

.image-mask-editor-section {
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.image-mask-editor-field {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  min-height: 22px;
}

.image-mask-editor-field-label {
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 500;
  color: #5f6877;
  line-height: 1;
  text-align: center;
  white-space: nowrap;
}

.image-mask-editor-field-label-muted {
  color: #97a2b2;
}

.image-mask-editor-segmented {
  display: flex;
  gap: 6px;
}

.image-mask-editor-segmented-btn {
  height: 28px;
  width: 28px;
  border: 0;
  border-radius: 8px;
  background: #ffffff;
  color: #5f6877;
  display: grid;
  place-items: center;
  cursor: pointer;
  transition: all 0.2s ease;
}

.image-mask-editor-segmented-btn:hover:not(:disabled):not(.active) {
  color: #272932;
}

.image-mask-editor-segmented-btn.active {
  background: #f8f7ff;
  color: #9278e6;
}

.image-mask-editor-segmented-btn:disabled {
  opacity: 0.46;
  cursor: not-allowed;
}

.image-mask-editor-segmented-icon {
  width: 15px;
  height: 15px;
}

.image-mask-editor-size-input-row {
  display: flex;
  align-items: center;
  min-width: 0;
  flex: 1;
  height: 22px;
}

.image-mask-editor-range-wrap {
  position: relative;
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  height: 22px;
}

.image-mask-editor-range-wrap input[type='range'] {
  appearance: none;
  display: block;
  width: 100%;
  height: 22px;
  margin: 0;
  min-width: 0;
  background: transparent;
  cursor: pointer;
}

.image-mask-editor-field-disabled .image-mask-editor-range-wrap input[type='range'] {
  cursor: not-allowed;
}

.image-mask-editor-field-disabled
  .image-mask-editor-range-wrap
  input[type='range']::-webkit-slider-runnable-track {
  background: #f1f3f6;
}

.image-mask-editor-field-disabled
  .image-mask-editor-range-wrap
  input[type='range']::-webkit-slider-thumb {
  background: #cbd3df;
}

.image-mask-editor-field-disabled
  .image-mask-editor-range-wrap
  input[type='range']::-moz-range-track {
  background: #f1f3f6;
}

.image-mask-editor-field-disabled
  .image-mask-editor-range-wrap
  input[type='range']::-moz-range-thumb {
  background: #cbd3df;
}

.image-mask-editor-range-wrap input[type='range']::-webkit-slider-runnable-track {
  height: 4px;
  border: none;
  border-radius: 999px;
  background: #eef2f5;
}

.image-mask-editor-range-wrap input[type='range']::-webkit-slider-thumb {
  appearance: none;
  width: 9px;
  height: 9px;
  margin-top: -2.5px;
  border: 0;
  border-radius: 999px;
  background: #9278e6;
  box-shadow: none;
  transition: background-color 0.16s ease;
}

.image-mask-editor-range-wrap input[type='range']::-moz-range-track {
  height: 4px;
  border: none;
  border-radius: 999px;
  background: #eef2f5;
}

.image-mask-editor-range-wrap input[type='range']::-moz-range-thumb {
  width: 9px;
  height: 9px;
  border: 0;
  border-radius: 999px;
  background: #9278e6;
}

.image-mask-editor-range-tooltip {
  position: absolute;
  left: var(--mask-editor-range-value, 0%);
  bottom: calc(100% - 2px);
  transform: translate(-50%, 2px);
  min-width: 18px;
  padding: 3px 5px;
  border-radius: 5px;
  background: rgba(24, 24, 27, 0.94);
  color: #ffffff;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  text-align: right;
  font-variant-numeric: tabular-nums;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 0.14s ease,
    transform 0.14s ease;
}

.image-mask-editor-range-wrap:hover .image-mask-editor-range-tooltip,
.image-mask-editor-range-wrap:focus-within .image-mask-editor-range-tooltip {
  opacity: 1;
  transform: translate(-50%, 0);
}

</style>
