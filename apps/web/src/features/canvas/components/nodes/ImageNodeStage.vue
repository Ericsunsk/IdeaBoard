<template>
  <div class="node-stage-handle-wrap">
    <NodeStageFrame
      ref="stageFrameRef"
      class="image-node-stage"
      :data-image-node-id="nodeId"
      :data-flow-node-id="flowNodeId || undefined"
      :data-image-editor="isImageToolActive ? '' : undefined"
      :class="{
        'is-expand-mode': !!expandModeActive,
        'is-image-editor-active': isImageToolActive,
      }"
      layout="center"
      :height="stageHeight"
      :active="active && !expandModeActive"
      :loading="!!directGenerating && !displayImage"
      :running="!!directGenerating"
      :transparent="!!expandModeActive || isImageToolActive"
      :aria-busy="!!directGenerating"
      aria-label="图片节点输入与预览区域"
      @dragover.stop="onStageDragOver"
      @drop.stop="onStageDrop"
      @contextmenu.prevent.stop="onStageContextMenu"
    >
      <template v-if="displayImage">
        <img
          :src="stageDisplayUrl"
          class="image-node-stage-image"
          :class="{ 'is-dragging': dragging, 'has-rendered-preview': !!renderedImagePreview }"
          alt="图片节点预览"
          draggable="false"
          role="button"
          tabindex="0"
          aria-label="图片节点预览，按 Enter 打开预览，按 Space 添加到画布"
          @dblclick="emit('append-to-canvas')"
          @keydown.enter.prevent="emit('open-preview')"
          @keydown.space.prevent="emit('append-to-canvas')"
        />
        <RenderedImagePreviewCanvas v-if="renderedImagePreview" :preview="renderedImagePreview" />
        <ImageCanvasExtendOverlay
          v-if="expandModeActive && expandOverlayStyle"
          :overlay-style="expandOverlayStyle"
          :handle-radius="expandHandleRadius ?? 4"
          :direct-generating="!!directGenerating"
          :output-count="outputCountValue"
          @submit="submitExpand"
          @update:output-count="selectOutputCount"
          @handle-pointerdown="emit('expand-handle-pointerdown', $event)"
          @handle-pointermove="emit('expand-handle-pointermove', $event)"
          @handle-pointerup="emit('expand-handle-pointerup', $event)"
          @frame-pointerdown="emit('expand-frame-pointerdown', $event)"
        />
        <ImageMaskEditorOverlay
          v-if="isMaskEditorActive && imageToolStageBinding?.maskOverlayStyle"
          :style="imageToolStageBinding.maskOverlayStyle"
          :canvas-width="imageToolStageBinding.maskCanvasWidth"
          :canvas-height="imageToolStageBinding.maskCanvasHeight"
          :mask-layer-opacity="imageToolStageBinding.maskLayerOpacity"
          :image-layer-visible="imageToolStageBinding.imageLayerVisible"
          :brush-size="imageToolStageBinding.maskBrushSize"
          :brush-shape="imageToolStageBinding.maskBrushShape"
          :set-canvas-ref="imageToolStageBinding.setMaskCanvasRef"
          @canvas-pointerdown="imageToolStageBinding.onMaskCanvasPointerDown"
        />
        <div class="image-node-stage-actions">
          <NodeActionButton
            variant="stage-ghost"
            aria-label="打开图片预览"
            @click="emit('open-preview')"
            >预览</NodeActionButton
          >
          <NodeActionButton
            v-if="!inputLocked"
            variant="stage-ghost"
            aria-label="清空图片输入"
            @click="emit('reset')"
          >
            清空
          </NodeActionButton>
        </div>
      </template>
      <button
        v-if="!displayImage && !emptyStateDisabled"
        type="button"
        class="image-node-stage-empty"
        :aria-label="emptyStageAriaLabel"
        @click="onEmptyStateClick"
      >
        <NodeEmptyStateIcon
          :variant="emptyIconVariantComputed"
          :class="{ 'image-node-stage-loader-logo': directGenerating }"
        />
        <span>{{ emptyStateLabel }}</span>
      </button>
      <div
        v-else-if="!displayImage"
        class="image-node-stage-empty is-readonly"
        :class="{ 'is-loading': directGenerating }"
        role="status"
        :aria-label="emptyStageAriaLabel"
      >
        <NodeEmptyStateIcon
          :variant="emptyIconVariantComputed"
          :class="{ 'image-node-stage-loader-logo': directGenerating }"
        />
        <span v-if="!directGenerating">{{ emptyStateLabel }}</span>
      </div>
    </NodeStageFrame>
    <slot name="handle-rail" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { clampImageNodeOutputCount } from '../../../canvas-board/domain'
import { useCanvasViewportContext } from '../../composables/useCanvasViewportContext'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import type { ImageToolStageBinding } from '../../image-tools'
import { useCanvasImagePreviewQuality } from '../../composables/useCanvasImagePreviewQuality'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import { ImageMaskEditorOverlay } from '../../mask-editor'
import ImageCanvasExtendOverlay from './ImageCanvasExtendOverlay.vue'
import NodeActionButton from './NodeActionButton.vue'
import NodeEmptyStateIcon from './NodeEmptyStateIcon.vue'
import NodeStageFrame from './NodeStageFrame.vue'
import RenderedImagePreviewCanvas from './RenderedImagePreviewCanvas.vue'

const props = defineProps<{
  nodeId: string | number
  flowNodeId?: string
  stageWidth: number
  stageHeight: number
  displayImage: { url: string } | null
  inputLocked?: boolean
  emptyIconVariant?: 'image' | 'fusion'
  directGenerating?: boolean
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  imageToolStageBinding?: ImageToolStageBinding | null
  outputCount?: number
  active: boolean
  dragging: boolean
}>()

const emit = defineEmits<{
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'open-preview'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'expand-submit', prompt: string, outputCount: number): void
  (e: 'update:output-count', value: number): void
  (
    e: 'expand-handle-pointerdown',
    payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent },
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const isImageToolActive = computed(() => props.imageToolStageBinding?.mode != null)
const isMaskEditorActive = computed(() => props.imageToolStageBinding?.mode === 'inpaint')
const forceFullPreviewQuality = computed(() => !!props.expandModeActive || isImageToolActive.value)
const canvasViewport = useCanvasViewportContext()
const { maxSide: previewMaxSide } = useCanvasImagePreviewQuality({
  stageWidth: () => props.stageWidth,
  stageHeight: () => props.stageHeight,
  zoomLevel: canvasViewport.zoomLevel,
  forceFullQuality: forceFullPreviewQuality,
})
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, {
  maxSide: previewMaxSide,
})
const renderedImagePreview = computed(() => props.imageToolStageBinding?.renderedPreview ?? null)
const emptyStateLabel = computed(() =>
  props.inputLocked ? 'Waiting...' : '上传或拖拽图片作为输入',
)
const emptyStateDisabled = computed(() => !!props.inputLocked || !!props.directGenerating)
const emptyStageAriaLabel = computed(() => {
  if (props.directGenerating) return '图片正在生成，暂时无法上传'
  if (props.inputLocked) return '等待生成完成后可上传图片'
  return '上传图片作为输入'
})
const emptyIconVariantComputed = computed(() => props.emptyIconVariant ?? 'image')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.outputCount))
type NodeStageFrameExpose = { stageElement: HTMLElement | null }
const stageFrameRef = ref<NodeStageFrameExpose | null>(null)

defineExpose({
  get stageElement() {
    return stageFrameRef.value?.stageElement ?? null
  },
})

function onStageDragOver(event: DragEvent) {
  event.preventDefault()
}

function onStageDrop(event: DragEvent) {
  event.preventDefault()
  if (props.inputLocked) return
  emit('stage-drop', event)
}

function onEmptyStateClick() {
  if (emptyStateDisabled.value) return
  emit('open-file')
}

function onStageContextMenu(event: MouseEvent) {
  if (!props.displayImage) return
  emit('contextmenu', event)
}

function submitExpand(prompt: string, outputCount: number) {
  if (props.directGenerating) return
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
}
</script>

<style scoped>
.node-stage-handle-wrap {
  position: relative;
  overflow: visible;
}

.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #fff;
  cursor: grab;
  user-select: none;
}

.image-node-stage-image--preview {
  position: absolute;
  inset: 0;
  z-index: 1;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}

.image-node-stage-image--preview + .image-node-stage-actions {
  z-index: 2;
}

.image-node-stage-image:focus-visible {
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: -4px;
}

.image-node-stage-image.is-dragging {
  cursor: grabbing;
}

.image-node-stage-image.has-rendered-preview {
  visibility: hidden;
}

.image-node-stage.is-running .image-node-stage-image,
.image-node-stage.is-running .image-node-stage-image.is-dragging {
  cursor: default;
}

.image-node-stage.is-expand-mode .image-node-stage-image,
.image-node-stage.is-image-editor-active .image-node-stage-image {
  pointer-events: none;
}

.image-node-stage-empty {
  position: relative;
  z-index: 2;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 11px;
  background: #ffffff;
  color: #a8b3be;
  gap: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  outline: none;
  transition:
    background-color var(--canvas-motion-standard) ease,
    color var(--canvas-motion-standard) ease;
}

.image-node-stage-empty:hover {
  background: #ffffff;
  color: #8894a5;
}

.image-node-stage-empty:focus-visible {
  color: #6f5ed1;
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: calc(-1 * var(--ib-focus-outline-width));
}

.image-node-stage-empty .node-empty-state-icon--image {
  width: 84px;
  height: 84px;
  stroke-width: 0.85;
}

.image-node-stage-empty .node-empty-state-icon--fusion {
  width: 126px;
  height: 126px;
}

.image-node-stage-empty span {
  display: none;
}

.image-node-stage-empty.is-readonly {
  cursor: default;
}

.image-node-stage-empty.is-loading {
  background: rgba(255, 255, 255, 0.56);
  color: #a8b3be;
  cursor: default;
}

.image-node-stage-loader-logo {
  animation: image-node-logo-spin-cycle 2s linear infinite;
}

.image-node-stage-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--canvas-motion-fast) ease;
}

.image-node-stage:hover .image-node-stage-actions,
.image-node-stage:focus-within .image-node-stage-actions {
  opacity: 1;
  pointer-events: auto;
}

.image-node-stage.is-expand-mode .image-node-stage-actions {
  opacity: 0;
  pointer-events: none;
}

.image-node-stage.is-image-editor-active .image-node-stage-actions {
  opacity: 0;
  pointer-events: none;
}

@keyframes image-node-logo-spin-cycle {
  0% {
    transform: rotate(0deg);
  }
  50% {
    transform: rotate(1turn);
  }
  100% {
    transform: rotate(1turn);
  }
}
</style>
