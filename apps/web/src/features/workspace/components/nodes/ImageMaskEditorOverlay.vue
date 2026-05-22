<template>
  <div class="image-mask-editor-overlay" data-image-mask-editor>
    <canvas
      ref="canvasRef"
      class="image-mask-editor-canvas"
      :width="canvasWidth"
      :height="canvasHeight"
      @pointerdown="emit('canvas-pointerdown', $event)"
    />
    <div class="image-mask-editor-panel image-mask-editor-left-panel" @pointerdown.stop>
      <button
        type="button"
        class="image-mask-editor-btn"
        :class="{ active: drawMode === 'paint' }"
        :aria-pressed="drawMode === 'paint'"
        title="画笔"
        @click="emit('set-draw-mode', 'paint')"
      >
        画笔
      </button>
      <button
        type="button"
        class="image-mask-editor-btn"
        :class="{ active: drawMode === 'erase' }"
        :aria-pressed="drawMode === 'erase'"
        title="擦除"
        @click="emit('set-draw-mode', 'erase')"
      >
        擦除
      </button>
    </div>
    <div class="image-mask-editor-panel image-mask-editor-right-panel" @pointerdown.stop>
      <label class="image-mask-editor-size">
        <span class="image-mask-editor-size-label">笔刷</span>
        <div class="image-mask-editor-size-input-row">
          <input
            type="range"
            min="4"
            max="48"
            step="1"
            :value="brushSize"
            @input="onBrushSizeInput"
          />
          <span class="image-mask-editor-size-value">{{ brushSize }}</span>
        </div>
      </label>
      <NodePromptComposer
        v-model="promptDraft"
        offset="0"
        class="image-mask-editor-composer"
        placeholder="输入局部重绘提示词"
        :output-count="outputCount"
        @update:output-count="emit('update:output-count', $event)"
        @submit="onSubmit"
      />
      <div class="image-mask-editor-actions">
        <button type="button" class="image-mask-editor-btn" @click="emit('clear')">清空</button>
        <button type="button" class="image-mask-editor-btn" @click="emit('cancel')">取消</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import type { ImageMaskDrawMode } from '../../composables/useImageMaskEditorOverlay'
import NodePromptComposer from './NodePromptComposer.vue'

const props = defineProps<{
  canvasWidth: number
  canvasHeight: number
  drawMode: ImageMaskDrawMode
  brushSize: number
  outputCount: number
  setCanvasRef: (element: Element | null) => void
}>()

const emit = defineEmits<{
  (e: 'canvas-pointerdown', event: PointerEvent): void
  (e: 'set-draw-mode', mode: ImageMaskDrawMode): void
  (e: 'set-brush-size', value: number): void
  (e: 'update:output-count', value: number): void
  (e: 'clear'): void
  (e: 'cancel'): void
  (e: 'done', payload: { prompt: string; outputCount: number }): void
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const promptDraft = ref('')

watch(
  canvasRef,
  (canvas, prev) => {
    if (prev) props.setCanvasRef(null)
    props.setCanvasRef(canvas)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  props.setCanvasRef(null)
})

function onBrushSizeInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('set-brush-size', Number(target?.value) || 18)
}

function onSubmit(prompt: string, outputCount: number) {
  emit('done', { prompt, outputCount })
  promptDraft.value = ''
}
</script>

<style scoped>
.image-mask-editor-overlay {
  --mask-editor-panel-gap: 10px;
  --mask-editor-left-panel-width: 52px;
  --mask-editor-right-panel-width: 236px;

  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: auto;
  overflow: visible;
}

.image-mask-editor-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border-radius: 4px;
  touch-action: none;
  cursor: crosshair;
}

.image-mask-editor-panel {
  position: absolute;
  border: 1px solid rgba(148, 163, 184, 0.5);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(8px);
  box-sizing: border-box;
  z-index: 2;
}

.image-mask-editor-left-panel {
  top: 50%;
  left: calc((var(--mask-editor-left-panel-width) + var(--mask-editor-panel-gap)) * -1);
  width: var(--mask-editor-left-panel-width);
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 5px;
  transform: translateY(-50%);
}

.image-mask-editor-right-panel {
  top: 50%;
  right: calc((var(--mask-editor-right-panel-width) + var(--mask-editor-panel-gap)) * -1);
  width: var(--mask-editor-right-panel-width);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  transform: translateY(-50%);
}

.image-mask-editor-btn {
  border: 1px solid #d4dbe7;
  border-radius: 6px;
  background: #ffffff;
  color: #2f3a4a;
  font-size: 12px;
  line-height: 1;
  height: 30px;
  padding: 0 8px;
  cursor: pointer;
  white-space: nowrap;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease;
}

.image-mask-editor-btn.active {
  border-color: #9278e6;
  color: #6f4fd0;
  background: #f7f3ff;
}

.image-mask-editor-composer {
  width: 100%;
  position: static;
}

:deep(.image-mask-editor-composer.image-node-prompt-composer) {
  position: static;
  width: 100%;
  animation: none;
}

.image-mask-editor-size {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.image-mask-editor-size-label {
  font-size: 12px;
  color: #4b5565;
}

.image-mask-editor-size-input-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.image-mask-editor-size input {
  flex: 1;
  accent-color: #9278e6;
}

.image-mask-editor-size-value {
  width: 28px;
  font-size: 12px;
  color: #4b5565;
  text-align: right;
}

.image-mask-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}
</style>
