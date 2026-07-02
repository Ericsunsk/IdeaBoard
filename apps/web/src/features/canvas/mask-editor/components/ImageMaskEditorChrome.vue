<template>
  <ImageToolChromeFrame mode="inpaint" :anchor-rect="anchorRect">
    <template #tools>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'paint'"
        aria-label-value="画笔"
        tooltip="画笔"
        @click="emit('set-draw-mode', 'paint')"
      >
        <EditPencil />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'erase'"
        aria-label-value="橡皮擦"
        tooltip="橡皮擦"
        @click="emit('set-draw-mode', 'erase')"
      >
        <Erase />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="填充（待支持）"
        tooltip="填充（待支持）"
      >
        <FillColor />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="魔棒（待支持）"
        tooltip="魔棒（待支持）"
      >
        <CircleSpark />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        disabled
        aria-label-value="智能蒙版（待支持）"
        tooltip="智能蒙版（待支持）"
      >
        <Sparks />
      </NodeToolIconButton>
    </template>
    <template #dock>
      <div
        class="editing-controls-panel editing-controls-panel--padded image-mask-editor-right-panel"
      >
        <section class="editing-controls-section">
          <div class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">笔刷</span>
            <div class="editing-controls-segmented">
              <button
                type="button"
                class="editing-controls-segmented-button node-tool-tooltip-trigger"
                :class="{ 'is-active': brushShape === 'round' }"
                :aria-pressed="brushShape === 'round'"
                aria-label="圆形笔尖"
                @click="emit('set-brush-shape', 'round')"
              >
                <Circle class="editing-controls-segmented-icon" />
                <span class="node-tool-tooltip" aria-hidden="true">圆形笔尖</span>
              </button>
              <button
                type="button"
                class="editing-controls-segmented-button node-tool-tooltip-trigger"
                :class="{ 'is-active': brushShape === 'square' }"
                :aria-pressed="brushShape === 'square'"
                aria-label="方形笔尖"
                @click="emit('set-brush-shape', 'square')"
              >
                <Square class="editing-controls-segmented-icon" />
                <span class="node-tool-tooltip" aria-hidden="true">方形笔尖</span>
              </button>
            </div>
          </div>
          <label class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">粗细</span>
            <div class="editing-controls-range-row">
              <div
                class="editing-controls-range-wrap"
                :style="
                  rangeTooltipStyle(
                    brushSize,
                    MASK_EDITOR_MIN_BRUSH_SIZE,
                    MASK_EDITOR_MAX_BRUSH_SIZE,
                  )
                "
              >
                <input
                  class="canvas-range-input"
                  type="range"
                  :min="MASK_EDITOR_MIN_BRUSH_SIZE"
                  :max="MASK_EDITOR_MAX_BRUSH_SIZE"
                  step="1"
                  :value="brushSize"
                  @input="onBrushSizeInput"
                />
                <span class="editing-controls-range-tooltip">{{ brushSize }}</span>
              </div>
            </div>
          </label>
          <label class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">硬度</span>
            <div class="editing-controls-range-row">
              <div
                class="editing-controls-range-wrap"
                :style="
                  rangeTooltipStyle(
                    brushHardness,
                    MASK_EDITOR_MIN_BRUSH_HARDNESS,
                    MASK_EDITOR_MAX_BRUSH_HARDNESS,
                  )
                "
              >
                <input
                  class="canvas-range-input"
                  type="range"
                  :min="MASK_EDITOR_MIN_BRUSH_HARDNESS"
                  :max="MASK_EDITOR_MAX_BRUSH_HARDNESS"
                  step="1"
                  :value="brushHardness"
                  @input="onBrushHardnessInput"
                />
                <span class="editing-controls-range-tooltip">{{ brushHardness }}</span>
              </div>
            </div>
          </label>
        </section>
        <section class="editing-controls-section">
          <label class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">蒙版</span>
            <div class="editing-controls-range-row">
              <div
                class="editing-controls-range-wrap"
                :style="
                  rangeTooltipStyle(
                    maskLayerOpacity,
                    MASK_EDITOR_MIN_LAYER_OPACITY,
                    MASK_EDITOR_MAX_LAYER_OPACITY,
                  )
                "
              >
                <input
                  class="canvas-range-input"
                  type="range"
                  :min="MASK_EDITOR_MIN_LAYER_OPACITY"
                  :max="MASK_EDITOR_MAX_LAYER_OPACITY"
                  step="1"
                  :value="maskLayerOpacity"
                  @input="onMaskOpacityInput"
                />
                <span class="editing-controls-range-tooltip">{{ maskLayerOpacity }}</span>
              </div>
            </div>
          </label>
        </section>
      </div>
      <div
        class="image-mask-editor-right-actions node-tool-surface"
        :class="{ 'is-feather-strength-mode': featherStrengthVisible }"
      >
        <NodeToolIconButton
          v-show="!featherStrengthVisible"
          class="image-mask-editor-tool-btn"
          aria-label-value="清空蒙版"
          tooltip="清空蒙版"
          @click="emit('clear-mask')"
        >
          <XmarkCircle />
        </NodeToolIconButton>
        <NodeToolIconButton
          v-show="!featherStrengthVisible"
          class="image-mask-editor-tool-btn"
          aria-label-value="反选蒙版"
          tooltip="反选蒙版"
          @click="emit('invert-mask')"
        >
          <PlusSquareDashed />
        </NodeToolIconButton>
        <div class="image-mask-editor-feather-control">
          <NodeToolIconButton
            v-show="!featherStrengthVisible"
            class="image-mask-editor-tool-btn"
            aria-label-value="羽化边缘"
            tooltip="羽化边缘"
            @click="onFeatherButtonClick"
          >
            <CenterAlign />
          </NodeToolIconButton>
          <input
            :ref="setFeatherStrengthRangeRef"
            class="image-mask-editor-feather-range canvas-range-input"
            type="range"
            :min="MASK_EDITOR_MIN_FEATHER_STRENGTH"
            :max="MASK_EDITOR_MAX_FEATHER_STRENGTH"
            step="1"
            :value="featherStrength"
            :tabindex="featherStrengthVisible ? 0 : -1"
            aria-label="羽化强度"
            @pointerdown.stop="onFeatherButtonPointerDown"
            @input="onFeatherStrengthInput"
            @change="onFeatherStrengthChange"
            @pointerup="onFeatherStrengthPointerUp"
            @click.stop="onFeatherButtonClick"
          />
        </div>
        <NodeToolIconButton
          v-show="!featherStrengthVisible"
          class="image-mask-editor-tool-btn"
          :disabled="!canUndo"
          aria-label-value="回退"
          tooltip="回退"
          @click="emit('undo-mask')"
        >
          <Undo />
        </NodeToolIconButton>
        <NodeToolIconButton
          v-show="!featherStrengthVisible"
          class="image-mask-editor-tool-btn"
          :disabled="!canRedo"
          aria-label-value="前进"
          tooltip="前进"
          @click="emit('redo-mask')"
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
        :max-image-inputs="MASK_EDITOR_REFERENCE_IMAGE_LIMIT"
        :show-aspect-ratio="false"
        show-image-input
        @update:output-count="emit('update:output-count', $event)"
        @image-input-open="emit('image-input-open')"
        @image-input-remove="emit('reference-image-remove', $event)"
        @image-input-preview="emit('reference-image-preview', $event)"
        @submit="onSubmit"
      />
    </template>
  </ImageToolChromeFrame>
</template>

<script setup lang="ts">
import { ref } from 'vue'
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
import type { ImageMaskBrushShape, ImageMaskDrawMode, MaskEditorReferenceImage } from '../types'
import { clampImageNodeOutputCount } from '../../../canvas-board/domain'
import {
  clampMaskBrushHardness,
  clampMaskBrushSize,
  clampMaskLayerOpacity,
  MASK_EDITOR_MAX_BRUSH_HARDNESS,
  MASK_EDITOR_MAX_BRUSH_SIZE,
  MASK_EDITOR_MAX_FEATHER_STRENGTH,
  MASK_EDITOR_MAX_LAYER_OPACITY,
  MASK_EDITOR_MIN_BRUSH_HARDNESS,
  MASK_EDITOR_MIN_BRUSH_SIZE,
  MASK_EDITOR_MIN_FEATHER_STRENGTH,
  MASK_EDITOR_MIN_LAYER_OPACITY,
  MASK_EDITOR_REFERENCE_IMAGE_LIMIT,
} from '../config/maskEditorConstants'
import { useMaskFeatherStrengthControl } from '../composables/useMaskFeatherStrengthControl'
import ImageToolChromeFrame from '../../components/nodes/ImageToolChromeFrame.vue'
import NodeToolIconButton from '../../components/nodes/NodeToolIconButton.vue'
import PromptTextComposer from '../../components/nodes/PromptTextComposer.vue'

defineProps<{
  drawMode: ImageMaskDrawMode
  brushShape: ImageMaskBrushShape
  brushSize: number
  brushHardness: number
  maskLayerOpacity: number
  outputCount: number
  referenceImages: MaskEditorReferenceImage[]
  directGenerating?: boolean
  canUndo?: boolean
  canRedo?: boolean
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
}>()

const emit = defineEmits<{
  (e: 'set-draw-mode', mode: ImageMaskDrawMode): void
  (e: 'set-brush-shape', value: ImageMaskBrushShape): void
  (e: 'set-brush-size', value: number): void
  (e: 'set-brush-hardness', value: number): void
  (e: 'set-mask-layer-opacity', value: number): void
  (e: 'clear-mask'): void
  (e: 'invert-mask'): void
  (e: 'feather-mask', strength: number): void
  (e: 'undo-mask'): void
  (e: 'redo-mask'): void
  (e: 'update:output-count', value: number): void
  (e: 'image-input-open'): void
  (e: 'reference-image-remove', imageId: string): void
  (e: 'reference-image-preview', image: MaskEditorReferenceImage): void
  (e: 'submit', prompt: string, outputCount: number): void
}>()

const promptDraft = ref('')
const {
  strength: featherStrength,
  visible: featherStrengthVisible,
  setRangeRef: setFeatherStrengthRangeRef,
  onButtonPointerDown: onFeatherButtonPointerDown,
  onButtonClick: onFeatherButtonClick,
  onStrengthInput: onFeatherStrengthInput,
  onStrengthChange: onFeatherStrengthChange,
  onStrengthPointerUp: onFeatherStrengthPointerUp,
} = useMaskFeatherStrengthControl({
  onApply: (strength) => emit('feather-mask', strength),
})

function onBrushSizeInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-brush-size', clampMaskBrushSize(Number(target?.value)))
}

function onBrushHardnessInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-brush-hardness', clampMaskBrushHardness(Number(target?.value)))
}

function onMaskOpacityInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-mask-layer-opacity', clampMaskLayerOpacity(Number(target?.value)))
}

function rangeTooltipStyle(value: number, min: number, max: number) {
  const ratio = max === min ? 0 : (value - min) / (max - min)
  const percent = Math.min(1, Math.max(0, ratio)) * 100
  return { '--editing-controls-range-value': `${percent}%` }
}

function onSubmit(prompt: string, outputCount: number) {
  emit('submit', prompt, clampImageNodeOutputCount(outputCount))
  promptDraft.value = ''
}
</script>

<style scoped>
@import '../../components/nodes/styles/node-tool-buttons.css';
@import '../../components/styles/editing-controls.css';

.image-mask-editor-right-panel {
  width: 100%;
  min-height: 0;
}

.image-mask-editor-tool-btn {
  width: 100%;
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}

.image-mask-editor-right-actions {
  position: relative;
  display: flex;
  justify-content: flex-end;
  gap: var(--node-tool-button-gap);
}

.image-mask-editor-right-actions .image-mask-editor-tool-btn,
.image-mask-editor-feather-control {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
}

.image-mask-editor-feather-control {
  position: relative;
  height: var(--node-tool-button-size);
}

.image-mask-editor-right-actions.is-feather-strength-mode {
  width: 100%;
  height: var(--node-tool-button-size);
  gap: 0;
}

.image-mask-editor-feather-range {
  --canvas-range-track-height: 6px;
  --canvas-range-thumb-size: 14px;
  --canvas-range-thumb-offset: -4px;
  --canvas-range-fill-color: var(--canvas-accent-hover);
  --canvas-range-track-background: var(--canvas-accent-outline);
  position: absolute;
  inset: 0;
  z-index: 2;
  opacity: 0;
  cursor: pointer;
}

.image-mask-editor-right-actions.is-feather-strength-mode .image-mask-editor-feather-control {
  flex: 1 1 100%;
  width: 100%;
}

.image-mask-editor-right-actions.is-feather-strength-mode .image-mask-editor-feather-range {
  position: relative;
  opacity: 1;
}

.image-mask-editor-composer {
  width: 100%;
  min-height: var(--image-tool-right-panel-width);
  aspect-ratio: 1 / 1;
}

.image-mask-editor-field {
  min-height: 22px;
}

.image-mask-editor-field-label {
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 500;
  color: var(--color-canvas-text-option);
  text-align: center;
}
</style>
