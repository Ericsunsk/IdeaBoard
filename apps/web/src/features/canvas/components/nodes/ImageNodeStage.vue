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
    :running="!!directGenerating || imageImportPending"
    :transparent="!!expandModeActive || isImageToolActive"
    :aria-busy="!!directGenerating || imageImportPending"
    aria-label="图片节点输入与预览区域"
    @dragover.stop="onStageDragOver"
    @drop.stop="onStageDrop"
    @contextmenu.prevent.stop="onStageContextMenu"
  >
    <template v-if="displayImage">
      <ImageToolStagePresentation
        v-if="imageToolStageBinding && stageDisplayUrl"
        :binding="imageToolStageBinding"
      >
        <img
          :src="stageDisplayUrl || undefined"
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
        v-else-if="stageDisplayUrl"
        :src="stageDisplayUrl || undefined"
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
      <div v-else class="image-node-stage-placeholder" role="status" aria-label="图片预览正在准备" />
      <ImageCanvasExtendOverlay
        v-if="stageDisplayUrl && expandModeActive && expandOverlayStyle"
        :overlay-style="expandOverlayStyle"
        :handle-radius="expandHandleRadius ?? 4"
        @submit="submitExpand"
        @handle-pointerdown="emit('expand-handle-pointerdown', $event)"
        @handle-pointermove="emit('expand-handle-pointermove', $event)"
        @handle-pointerup="emit('expand-handle-pointerup', $event)"
        @frame-pointerdown="emit('expand-frame-pointerdown', $event)"
      />
      <div v-if="!libraryImport && !localUpload" class="image-node-stage-actions">
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
    <div
      v-if="imageImportFailureMessage"
      class="image-node-stage-import-failure"
      role="status"
      :title="imageImportFailureMessage"
    >
      <span>Failed</span>
      <NodeActionButton
        variant="stage-ghost"
        aria-label="重试导入图片"
        @click.stop="emit('retry-image-import')"
      >
        Retry
      </NodeActionButton>
    </div>
    <div
      v-else-if="imageImportPending && !displayImage"
      class="image-node-stage-pending"
      aria-busy="true"
      aria-label="图片预览正在准备"
    />
    <button
      v-else-if="!displayImage && !emptyStateDisabled"
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
import { useElementVisibility } from '@vueuse/core'
import { useCanvasViewportContext } from '../../composables/useCanvasViewportContext'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import type { ImageToolStageBinding } from '../../image-tools'
import type { LibraryImportFact } from '../../../canvas-board/domain'
import type { FlowLocalUploadState } from '../../types/flowTypes'
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
  displayImage: { url: string; mediaAssetId?: number } | null
  libraryImport?: LibraryImportFact
  localUpload?: FlowLocalUploadState
  inputLocked?: boolean
  emptyIconVariant?: 'image' | 'fusion'
  directGenerating?: boolean
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  imageToolStageBinding?: ImageToolStageBinding | null
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
  (e: 'retry-image-import'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'expand-submit', prompt: string): void
  (
    e: 'expand-handle-pointerdown',
    payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent },
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

type NodeStageFrameExpose = { stageElement: HTMLElement | null }
const stageFrameRef = ref<NodeStageFrameExpose | null>(null)
const stageElement = computed(() => stageFrameRef.value?.stageElement ?? null)
const isStageVisible = useElementVisibility(stageElement)

const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const imageImportFailureMessage = computed(() => {
  if (props.libraryImport?.state === 'failed') return props.libraryImport.failureMessage
  return props.localUpload?.failureMessage?.trim() ?? ''
})
const imageImportPending = computed(
  () =>
    props.libraryImport?.state === 'pending' ||
    (!!props.localUpload && !imageImportFailureMessage.value),
)
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
  mediaAssetId: () => props.displayImage?.mediaAssetId,
  isVisible: isStageVisible,
  keepPreviousOnSourceChange: true,
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

function submitExpand(prompt: string) {
  emit('expand-submit', prompt)
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

.image-node-stage-placeholder {
  width: 100%;
  height: 100%;
  background: var(--canvas-node-bg);
}

.image-node-stage-import-failure {
  position: absolute;
  right: 8px;
  bottom: 8px;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  max-width: calc(100% - 16px);
  padding: 4px 6px 4px 8px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--canvas-node-bg) 82%, var(--color-board-danger) 18%);
  color: var(--color-board-danger);
  font-size: 12px;
  font-weight: 600;
  text-align: center;
  pointer-events: auto;
}

.image-node-stage-pending {
  position: absolute;
  inset: 0;
  background: var(--canvas-node-bg);
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
  color: var(--canvas-text-subtle);
  gap: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  outline: none;
}

.image-node-stage-empty:hover {
  background: var(--canvas-node-bg);
  color: var(--canvas-text-subtle);
}

.image-node-stage-empty:focus-visible {
  color: var(--canvas-accent-hover);
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: calc(-1 * var(--ib-focus-outline-width));
}

.image-node-stage-empty .node-empty-state-icon--image,
.image-node-stage-empty .node-empty-state-icon--fusion {
  width: 112px;
  height: 112px;
  stroke-width: 0.6;
}

.image-node-stage-empty span {
  display: none;
}

.image-node-stage-empty.is-readonly {
  cursor: default;
}

.image-node-stage-empty.is-loading {
  background: rgba(255, 255, 255, 0.56);
  color: var(--canvas-text-subtle);
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
