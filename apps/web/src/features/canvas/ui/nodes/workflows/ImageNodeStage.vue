<template>
  <NodeStageFrame
    ref="stageFrameRef"
    class="image-node-stage"
    :data-image-node-id="nodeId"
    :data-flow-node-id="flowNodeId || undefined"
    :class="{
      'is-outpaint-mode': !!isOutpaintModeActive,
      'is-retouch-active': isImageToolActive,
    }"
    layout="center"
    :height="stageHeight"
    :is-active="isActive && !isOutpaintModeActive"
    :is-connection-drop-target="isConnectionDropTarget"
    :is-loading="!!isDirectGenerating && !stageDisplayImage"
    :is-running="!!isDirectGenerating || isImageImportPending"
    :is-transparent="!!isOutpaintModeActive || isImageToolActive"
    :is-aria-busy="!!isDirectGenerating || isImageImportPending"
    :aria-label="t('canvas.controls.stageLabel')"
    @dragover.stop="onStageDragOver"
    @drop.stop="onStageDrop"
    @contextmenu.prevent.stop="onStageContextMenu"
  >
    <template v-if="stageDisplayImage && renderedStageUrl">
      <StagePresentation
        v-if="imageToolStageBinding && stageDisplayUrl"
        :binding="imageToolStageBinding"
      >
        <img
          :src="renderedStageUrl"
          :loading="isStageVisible ? 'eager' : 'lazy'"
          decoding="async"
          class="image-node-stage-image image-tool-stage-media"
          :class="{ 'is-dragging': isDragging, 'is-tier-loading': !isStageImageLoaded }"
          :alt="t('canvas.controls.stagePreviewAlt')"
          draggable="false"
          role="button"
          tabindex="0"
          :aria-label="t('canvas.controls.stagePreviewHint')"
          @load="onStageImageLoad"
          @error="onStageImageError"
          @dblclick="emit('append-to-canvas')"
          @keydown.enter.prevent="emit('open-preview')"
          @keydown.space.prevent="emit('append-to-canvas')"
        />
      </StagePresentation>
      <img
        v-else
        :src="renderedStageUrl"
        :loading="isStageVisible ? 'eager' : 'lazy'"
        decoding="async"
        class="image-node-stage-image"
        :class="{ 'is-dragging': isDragging, 'is-tier-loading': !isStageImageLoaded }"
        :alt="t('canvas.controls.stagePreviewAlt')"
        draggable="false"
        role="button"
        tabindex="0"
        :aria-label="t('canvas.controls.stagePreviewHint')"
        @load="onStageImageLoad"
        @error="onStageImageError"
        @dblclick="emit('append-to-canvas')"
        @keydown.enter.prevent="emit('open-preview')"
        @keydown.space.prevent="emit('append-to-canvas')"
      />
      <ImageOutpaintOverlay
        v-if="stageDisplayUrl && isOutpaintModeActive && outpaintOverlayStyle"
        :overlay-style="outpaintOverlayStyle"
        :handle-radius="outpaintHandleRadius ?? 4"
        @submit="submitOutpaint"
        @handle-pointerdown="emit('outpaint-handle-pointerdown', $event)"
        @handle-pointermove="emit('outpaint-handle-pointermove', $event)"
        @handle-pointerup="emit('outpaint-handle-pointerup', $event)"
        @frame-pointerdown="emit('outpaint-frame-pointerdown', $event)"
      />
      <div v-if="!libraryImport && !localUpload" class="image-node-stage-actions">
        <NodeActionButton
          variant="stage-ghost"
          :aria-label="t('canvas.controls.openPreview')"
          @click="emit('open-preview')"
          >{{ t('canvas.node.preview') }}</NodeActionButton
        >
        <NodeActionButton
          v-if="!isInputLocked"
          variant="stage-ghost"
          :aria-label="t('canvas.controls.clearImageInput')"
          @click="emit('reset')"
        >
          {{ t('canvas.controls.clear') }}
        </NodeActionButton>
      </div>
    </template>
    <div
      v-if="imageImportFailureMessage"
      class="image-node-stage-import-failure"
      role="status"
      :title="imageImportFailureMessage"
    >
      <span>{{ t('canvas.states.failed') }}</span>
      <NodeActionButton
        variant="stage-ghost"
        :aria-label="t('canvas.controls.retryImageImport')"
        @click.stop="emit('retry-image-import')"
      >
        {{ t('common.actions.retry') }}
      </NodeActionButton>
    </div>
    <div
      v-else-if="(isImageImportPending || isMediaAssetPending) && !renderedStageUrl"
      class="image-node-stage-pending"
      aria-busy="true"
      :aria-label="t('canvas.controls.preparingPreview')"
    />
    <button
      v-else-if="!stageDisplayImage && !isEmptyStateDisabled"
      type="button"
      class="image-node-stage-empty"
      :aria-label="emptyStageAriaLabel"
      @click="onEmptyStateClick"
    >
      <NodeEmptyStateIcon :variant="emptyIconVariantComputed" />
      <span>{{ emptyStateLabel }}</span>
    </button>
    <div
      v-else-if="!stageDisplayImage"
      class="image-node-stage-empty is-readonly"
      :class="{ 'is-loading': isDirectGenerating }"
      role="status"
      :aria-label="emptyStageAriaLabel"
    >
      <ComposingOrb v-if="isDirectGenerating" />
      <NodeEmptyStateIcon v-else :variant="emptyIconVariantComputed" />
      <span v-if="!isDirectGenerating">{{ emptyStateLabel }}</span>
    </div>
  </NodeStageFrame>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useElementVisibility } from '@vueuse/core'
import { useCanvasViewportContext } from '../../../session/flow-adapter'
import type { OutpaintDragAnchor } from '../../../tools/outpaint/session/useImageOutpaintOverlay.ts'
import { ImageToolStagePresentation as StagePresentation } from '../../../tools/image-tools/ui/stage'
import type { ImageToolStageBinding } from '../../../tools/image-tools'
import type { LibraryImportFact } from '../../../../board/ports/canvas'
import type { FlowLocalUploadState } from '../../../core/nodes/flowTypes.ts'
import { useCanvasImagePreviewQuality } from '../../../session/flow-adapter'
import { useDisplayImagePreview, useTierFallback } from '../../../session/flow-adapter'
import ImageOutpaintOverlay from '../../../tools/outpaint/ui/ImageOutpaintOverlay.vue'
import ComposingOrb from './ComposingOrb.vue'
import { NodeActionButton, NodeEmptyStateIcon, NodeStageFrame } from '../primitives'

const props = defineProps<{
  nodeId: string | number
  flowNodeId?: string | undefined
  stageWidth: number
  stageHeight: number
  displayImage: { url: string; assetId?: string | undefined } | null
  libraryImport?: LibraryImportFact | undefined
  localUpload?: FlowLocalUploadState | undefined
  isInputLocked?: boolean
  emptyIconVariant?: 'image' | 'fusion'
  isDirectGenerating?: boolean
  isOutpaintModeActive?: boolean
  outpaintOverlayStyle?: Record<string, string> | null | undefined
  outpaintHandleRadius?: number | undefined
  imageToolStageBinding?: ImageToolStageBinding | null | undefined
  isActive: boolean
  isConnectionDropTarget?: boolean
  isDragging: boolean
}>()

const emit = defineEmits<{
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'open-preview'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'retry-image-import'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'outpaint-submit', prompt: string): void
  (
    e: 'outpaint-handle-pointerdown',
    payload: { anchor: OutpaintDragAnchor; event: PointerEvent },
  ): void
  (e: 'outpaint-handle-pointermove', event: PointerEvent): void
  (e: 'outpaint-handle-pointerup', event: PointerEvent): void
  (e: 'outpaint-frame-pointerdown', event: PointerEvent): void
}>()

type NodeStageFrameExpose = { stageElement: HTMLElement | null }
const stageFrameRef = ref<NodeStageFrameExpose | null>(null)
const stageElement = computed(() => stageFrameRef.value?.stageElement ?? null)
const isStageVisible = useElementVisibility(stageElement)

const stageDisplayImage = computed(() => {
  const previewUrl = props.localUpload?.previewUrl?.trim()
  if (previewUrl) return { url: previewUrl }
  const displayImageUrl = props.displayImage?.url?.trim()
  return displayImageUrl ? props.displayImage : null
})
const sourceDisplayUrl = computed(() => stageDisplayImage.value?.url ?? null)
const imageImportFailureMessage = computed(() => {
  if (props.libraryImport?.state === 'failed') return props.libraryImport.failureMessage
  return props.localUpload?.failureMessage?.trim() ?? ''
})
const isImageImportPending = computed(
  () =>
    props.libraryImport?.state === 'pending' ||
    (!!props.localUpload && !imageImportFailureMessage.value),
)
const isImageToolActive = computed(() => !!props.imageToolStageBinding?.isActive)
const shouldForceFullPreviewQuality = computed(
  () => !!props.isOutpaintModeActive || isImageToolActive.value,
)
const canvasViewport = useCanvasViewportContext()
const { maxSide: previewMaxSide } = useCanvasImagePreviewQuality({
  stageWidth: () => props.stageWidth,
  stageHeight: () => props.stageHeight,
  zoomLevel: canvasViewport.qualityZoomLevel,
  shouldForceFullQuality: shouldForceFullPreviewQuality,
})
const { displayUrl: stageDisplayUrl, contentFallbackUrl: stageContentFallbackUrl } =
  useDisplayImagePreview(sourceDisplayUrl, {
    maxSide: previewMaxSide,
    assetId: () => stageDisplayImage.value?.assetId,
    isVisible: isStageVisible,
    shouldKeepPreviousOnSourceChange: true,
  })
// Media assets are authenticated API resources. Do not bind their API URL as
// an optimistic <img> fallback while the signed delivery URL is loading: the
// API URL is not a durable cross-origin image source and can fail before the
// authenticated delivery metadata resolves. Local and external sources retain
// the immediate optimistic fallback behavior.
const renderedStageBase = computed(() => {
  if (stageDisplayImage.value?.assetId) return stageDisplayUrl.value
  return stageDisplayUrl.value || sourceDisplayUrl.value
})
const {
  src: renderedStageUrl,
  isLoaded: isStageImageLoaded,
  onLoad: onStageImageLoad,
  onError: onStageImageError,
} = useTierFallback(renderedStageBase, stageContentFallbackUrl, () => [
  sourceDisplayUrl.value,
  stageDisplayImage.value?.assetId,
])
const isMediaAssetPending = computed(
  () => !!stageDisplayImage.value?.assetId && !stageDisplayUrl.value,
)
const { t } = useI18n()
const emptyStateLabel = computed(() =>
  props.isInputLocked ? t('canvas.controls.waiting') : t('canvas.controls.uploadOrDropImage'),
)
const isEmptyStateDisabled = computed(() => !!props.isInputLocked || !!props.isDirectGenerating)
const emptyStageAriaLabel = computed(() => {
  if (props.isDirectGenerating) return t('canvas.controls.generatingUploadBlocked')
  if (props.isInputLocked) return t('canvas.controls.waitingToUpload')
  return t('canvas.controls.uploadImageInput')
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
  if (props.isInputLocked) return
  emit('stage-drop', event)
}

function onEmptyStateClick() {
  if (isEmptyStateDisabled.value) return
  emit('open-file')
}

function onStageContextMenu(event: MouseEvent) {
  if (!props.displayImage) return
  emit('contextmenu', event)
}

function submitOutpaint(prompt: string) {
  emit('outpaint-submit', prompt)
}
</script>

<style scoped>
.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: var(--canvas-node-bg);
  cursor: grab;
  user-select: none;
  transition: filter var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.image-node-stage-image.is-tier-loading {
  filter: blur(12px);
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
  border-radius: var(--radius-control);
  background: color-mix(in srgb, var(--canvas-node-bg) 82%, var(--destructive) 18%);
  color: var(--destructive);
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
  outline: var(--focus-outline-width) solid var(--focus-outline-color);
  outline-offset: -4px;
}

.image-node-stage-image.is-dragging {
  cursor: grabbing;
}

.image-node-stage.is-running .image-node-stage-image,
.image-node-stage.is-running .image-node-stage-image.is-dragging {
  cursor: default;
}

.image-node-stage.is-outpaint-mode .image-node-stage-image,
.image-node-stage.is-retouch-active .image-node-stage-image {
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
  outline: var(--focus-outline-width) solid var(--focus-outline-color);
  outline-offset: calc(-1 * var(--focus-outline-width));
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

.image-node-stage.is-outpaint-mode .image-node-stage-actions {
  opacity: 0;
  pointer-events: none;
}

.image-node-stage.is-retouch-active .image-node-stage-actions {
  opacity: 0;
  pointer-events: none;
}
</style>
