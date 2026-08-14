<template>
  <NodeStageFrame
    ref="stageFrameRef"
    class="image-node-stage"
    :data-image-node-id="nodeId"
    :data-flow-node-id="flowNodeId || undefined"
    :class="{
      'is-expand-mode': !!expandModeActive,
      'is-image-editor-active': isImageToolActive,
    }"
    layout="center"
    :height="stageHeight"
    :active="active && !expandModeActive"
    :connection-drop-target="connectionDropTarget"
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
      <ImageToolStagePresentation v-if="imageToolStageBinding" :binding="imageToolStageBinding">
        <img
          :src="stageDisplayUrl"
          class="image-node-stage-image image-tool-stage-media"
          :class="{ 'is-dragging': dragging }"
          alt="图片节点预览"
          draggable="false"
          role="button"
          tabindex="0"
          aria-label="图片节点预览，按 Enter 打开预览，按 Space 添加到画布"
          @dblclick="emit('append-to-canvas')"
          @keydown.enter.prevent="emit('open-preview')"
          @keydown.space.prevent="emit('append-to-canvas')"
        />
      </ImageToolStagePresentation>
      <img
        v-else
        :src="stageDisplayUrl"
        class="image-node-stage-image"
        alt="图片节点预览"
        draggable="false"
        role="button"
        tabindex="0"
        aria-label="图片节点预览，按 Enter 打开预览，按 Space 添加到画布"
        @dblclick="emit('append-to-canvas')"
        @keydown.enter.prevent="emit('open-preview')"
        @keydown.space.prevent="emit('append-to-canvas')"
      />
      <ImageCanvasExtendOverlay
        v-if="expandModeActive && expandOverlayStyle"
        :overlay-style="expandOverlayStyle"
        :handle-radius="expandHandleRadius ?? 4"
        :output-count="outputCountValue"
        @submit="submitExpand"
        @update:output-count="selectOutputCount"
        @handle-pointerdown="emit('expand-handle-pointerdown', $event)"
        @handle-pointermove="emit('expand-handle-pointermove', $event)"
        @handle-pointerup="emit('expand-handle-pointerup', $event)"
        @frame-pointerdown="emit('expand-frame-pointerdown', $event)"
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
      <NodeEmptyStateIcon :variant="emptyIconVariantComputed" />
      <span>{{ emptyStateLabel }}</span>
    </button>
    <div
      v-else-if="!displayImage"
      class="image-node-stage-empty is-readonly"
      :class="{ 'is-loading': directGenerating }"
      role="status"
      :aria-label="emptyStageAriaLabel"
    >
      <ComposingOrb v-if="directGenerating" />
      <NodeEmptyStateIcon v-else :variant="emptyIconVariantComputed" />
      <span v-if="!directGenerating">{{ emptyStateLabel }}</span>
    </div>
  </NodeStageFrame>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { requireImageNodeOutputCount } from '../../../canvas-board/domain'
import { useCanvasViewportContext } from '../../composables/useCanvasViewportContext'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import type { ImageToolStageBinding } from '../../image-tools'
import { useCanvasImagePreviewQuality } from '../../composables/useCanvasImagePreviewQuality'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import ImageToolStagePresentation from '../../image-tools/components/ImageToolStagePresentation.vue'
import ImageCanvasExtendOverlay from './ImageCanvasExtendOverlay.vue'
import ComposingOrb from './ComposingOrb.vue'
import NodeActionButton from './NodeActionButton.vue'
import NodeEmptyStateIcon from './NodeEmptyStateIcon.vue'
import NodeStageFrame from './NodeStageFrame.vue'

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
  connectionDropTarget?: boolean
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
const isImageToolActive = computed(() => !!props.imageToolStageBinding?.active)
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
const outputCountValue = computed(() => requireImageNodeOutputCount(props.outputCount))
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
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', requireImageNodeOutputCount(value))
}
</script>

<style scoped>
.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #fff;
  cursor: grab;
  user-select: none;
}

.image-node-stage-image:focus-visible {
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: -4px;
}

.image-node-stage-image.is-dragging {
  cursor: grabbing;
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
  background: var(--canvas-node-bg);
  color: #a8b3be;
  gap: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  outline: none;
  transition:
    background-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing);
}

.image-node-stage-empty:hover {
  background: var(--canvas-node-bg);
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

.image-node-stage-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
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
</style>
