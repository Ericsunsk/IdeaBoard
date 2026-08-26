<template>
  <ImageToolChromeFrame mode="inpaint" :anchor-rect="anchorRect">
    <template #tools>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'paint'"
        :aria-label="t('canvas.maskEditor.brushTool')"
        :tooltip="t('canvas.maskEditor.brushTool')"
        @click="emit('set-draw-mode', 'paint')"
      >
        <EditPencil />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-mask-editor-tool-btn"
        :active="drawMode === 'erase'"
        :aria-label="t('canvas.maskEditor.eraserTool')"
        :tooltip="t('canvas.maskEditor.eraserTool')"
        @click="emit('set-draw-mode', 'erase')"
      >
        <Erase />
      </NodeToolIconButton>
    </template>
    <template #dock>
      <div
        class="editing-controls-panel editing-controls-panel--padded image-mask-editor-right-panel"
      >
        <section class="editing-controls-section">
          <label class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">{{ t("canvas.maskEditor.brushSizeLabel") }}</span>
            <div class="editing-controls-range-row">
              <div
                class="editing-controls-range-wrap"
                :style="
                  rangePresentationStyle(
                    brushSize,
                    MASK_EDITOR_MIN_BRUSH_SIZE,
                    MASK_EDITOR_MAX_BRUSH_SIZE,
                  )
                "
              >
                <input
                  id="mask-editor-brush-size"
                  name="mask-brush-size"
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
        </section>
        <section class="editing-controls-section">
          <label class="editing-controls-field image-mask-editor-field">
            <span class="editing-controls-label image-mask-editor-field-label">{{ t("canvas.maskEditor.maskOpacityLabel") }}</span>
            <div class="editing-controls-range-row">
              <div
                class="editing-controls-range-wrap"
                :style="
                  rangePresentationStyle(
                    maskLayerOpacity,
                    MASK_EDITOR_MIN_LAYER_OPACITY,
                    MASK_EDITOR_MAX_LAYER_OPACITY,
                  )
                "
              >
                <input
                  id="mask-editor-layer-opacity"
                  name="mask-layer-opacity"
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
      <div class="image-mask-editor-right-actions node-tool-surface">
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          :aria-label="t('canvas.maskEditor.clearMask')"
          :tooltip="t('canvas.maskEditor.clearMask')"
          @click="emit('clear-mask')"
        >
          <XmarkCircle />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          :disabled="!canUndo"
          :aria-label="t('canvas.maskEditor.undo')"
          :tooltip="t('canvas.maskEditor.undo')"
          @click="emit('undo-mask')"
        >
          <Undo />
        </NodeToolIconButton>
        <NodeToolIconButton
          class="image-mask-editor-tool-btn"
          :disabled="!canRedo"
          :aria-label="t('canvas.maskEditor.redo')"
          :tooltip="t('canvas.maskEditor.redo')"
          @click="emit('redo-mask')"
        >
          <Redo />
        </NodeToolIconButton>
      </div>
      <PromptTextComposer
        v-model="promptDraft"
        class="image-mask-editor-composer"
        variant="floating"
        :placeholder="t('canvas.tools.maskPromptPlaceholder')"
        :disabled="directGenerating"
        :output-count="1"
        :show-aspect-ratio="false"
        :show-output-count="false"
        :show-output-resolution="false"
        :billing-state="billingQuote"
        @submit="onSubmit"
      />
    </template>
  </ImageToolChromeFrame>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref } from 'vue'
import { EditPencil, Erase, Redo, XmarkCircle, Undo } from '@iconoir/vue'
import type { ImageMaskDrawMode } from '../types'
import {
  clampMaskBrushSize,
  clampMaskLayerOpacity,
  MASK_EDITOR_MAX_BRUSH_SIZE,
  MASK_EDITOR_MAX_LAYER_OPACITY,
  MASK_EDITOR_MIN_BRUSH_SIZE,
  MASK_EDITOR_MIN_LAYER_OPACITY,
} from '../config/maskEditorConstants'
import ImageToolChromeFrame from '../../image-tools/components/ImageToolChromeFrame.vue'
import { NodeToolIconButton, PromptTextComposer } from '../../components/nodes/primitives'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'
import { useCanvasBillingQuote } from '../../execution'

defineProps<{
  drawMode: ImageMaskDrawMode
  brushSize: number
  maskLayerOpacity: number
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
  (e: 'set-brush-size', value: number): void
  (e: 'set-mask-layer-opacity', value: number): void
  (e: 'clear-mask'): void
  (e: 'undo-mask'): void
  (e: 'redo-mask'): void
  (e: 'submit', prompt: string): void
}>()

const { t } = useI18n()

const promptDraft = ref('')
const billingQuote = useCanvasBillingQuote(() => WORKFLOW_FEATURE.inpaint)

function onBrushSizeInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-brush-size', clampMaskBrushSize(Number(target?.value)))
}

function onMaskOpacityInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-mask-layer-opacity', clampMaskLayerOpacity(Number(target?.value)))
}

function rangePresentationStyle(value: number, min: number, max: number) {
  const ratio = max === min ? 0 : (value - min) / (max - min)
  const percent = `${Math.min(1, Math.max(0, ratio)) * 100}%`
  return {
    '--editing-controls-range-value': percent,
    '--canvas-range-track-background': `linear-gradient(90deg, var(--canvas-accent) 0%, var(--canvas-accent) ${percent}, var(--canvas-accent-outline) ${percent}, var(--canvas-accent-outline) 100%)`,
  }
}

function onSubmit(prompt: string) {
  emit('submit', prompt)
  promptDraft.value = ''
}
</script>

<style scoped>
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
  justify-content: flex-start;
  gap: var(--node-tool-button-gap);
}

.image-mask-editor-right-actions .image-mask-editor-tool-btn {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
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
  color: var(--canvas-text-option);
  text-align: center;
}
</style>
