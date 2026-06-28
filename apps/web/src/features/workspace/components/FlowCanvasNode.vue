<template>
  <div
    ref="flowCanvasNodeRef"
    class="flow-canvas-node"
    :class="[`is-${data.kind}`, { 'is-selected': selected, 'is-running': nodeRunning }]"
    @mouseenter="onNodeMouseEnter"
    @mouseleave="onNodeMouseLeave"
    @focusin="onNodeFocusIn"
    @focusout="onNodeFocusOut"
    @pointerdown.capture="onNodePointerDown"
  >
    <div
      v-if="showDefaultDragZone"
      class="flow-canvas-node__drag-zone flow-canvas-node__drag-zone--default"
    />
    <template v-if="isPromptNode && promptSnapshot">
      <PromptNode
        :node="promptSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="promptUiState"
        @update:text="onPromptTextUpdate"
        @update:output-count="emitNodeParam('outputCount', $event)"
        @update:aspect-ratio="emitNodeParam('aspectRatio', $event)"
        @update:input-images="emitNodeParam('inputImages', $event)"
        @update:title="emitNodeParam('title', $event)"
        @input-image-drop="onMediaInputImageDrop"
        @input-image-open="onMediaInputImageOpen"
        @input-image-preview="onMediaInputImagePreview"
        @input-image-remove="onMediaInputImageRemove"
        @node-mousedown="noopMouse"
        @delete="onDeleteRequest"
        @focus="noop"
        @generate="onPromptGenerate"
      />
    </template>
    <template v-else-if="isImageNode && imageSnapshot">
      <div
        class="flow-canvas-node__drag-surface relative overflow-visible"
        :class="{ 'is-running': nodeRunning }"
        @mousedown="onNodeMouseDown"
        @click.stop
        @contextmenu.stop
      >
        <ImageNode
          :node="imageSnapshot"
          :flow-node-id="props.nodeId"
          :set-image-stage-ref="props.setImageStageRef"
          :clear-image-stage-ref="props.clearImageStageRef"
          :active="selected"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :stage-height="stageHeight"
          :display-image="displayImage"
          :empty-icon-variant="imageEmptyIconVariant"
          :input-locked="imageInputLocked"
          :action-menu-visible="!!props.imageActionVisible"
          :prompt-composer-visible="!!props.imagePromptComposerVisible"
          :prompt-placeholder="props.imagePromptPlaceholder"
          :direct-generating="imageGenerating"
          :show-strength-control="!!props.showStrengthControl"
          :strength-pct="props.strengthPct ?? 50"
          :set-strength-track-ref="props.setStrengthTrackRef ?? (() => {})"
          :expand-mode-active="!!props.expandModeActive"
          :expand-overlay-style="props.expandOverlayStyle ?? null"
          :expand-handle-radius="props.expandHandleRadius ?? 4"
          :image-editor-mode="props.imageEditorMode ?? null"
          :image-editor-rendered-preview="props.imageEditorRenderedPreview ?? null"
          :mask-editor-overlay-style="props.maskEditorOverlayStyle ?? null"
          :mask-editor-canvas-width="props.maskEditorCanvasWidth ?? 0"
          :mask-editor-canvas-height="props.maskEditorCanvasHeight ?? 0"
          :mask-layer-opacity="props.maskLayerOpacity ?? 72"
          :image-layer-visible="props.imageLayerVisible ?? true"
          :mask-editor-brush-size="props.maskEditorBrushSize ?? 18"
          :mask-editor-brush-shape="props.maskEditorBrushShape ?? 'round'"
          :set-mask-canvas-ref="
            (element) => (props.setMaskCanvasRef ?? (() => {}))(props.nodeId, element)
          "
          @node-mousedown="noopMouse"
          @update:title="emitNodeParam('title', $event)"
          @update:output-count="emitNodeParam('outputCount', $event)"
          @delete="onDeleteRequest"
          @stage-drop="onImageStageDrop"
          @open-file="onImageOpenFile"
          @open-preview="onImageOpenPreview"
          @append-to-canvas="onImageAppendToCanvas"
          @reset="onImageReset"
          @contextmenu="onImageContextMenu"
          @prompt-generate="onImagePromptGenerate"
          @expand-submit="onImageExpandSubmit"
          @action="onImageAction"
          @strength-action-mousedown="onStrengthActionMouseDown"
          @strength-track-mousedown="onStrengthTrackMouseDown"
          @expand-handle-pointerdown="onImageExpandHandlePointerDown"
          @expand-handle-pointermove="onImageExpandHandlePointerMove"
          @expand-handle-pointerup="onImageExpandHandlePointerUp"
          @expand-frame-pointerdown="onImageExpandFramePointerDown"
          @mask-canvas-pointerdown="emit('mask-canvas-pointerdown', $event)"
        />
      </div>
    </template>
    <template v-else-if="isFusionNode && fusionSnapshot">
      <div
        class="flow-canvas-node__drag-surface relative overflow-visible"
        :class="{ 'is-running': nodeRunning }"
        @mousedown="onNodeMouseDown"
        @click.stop
        @contextmenu.stop
      >
        <FusionNode
          :node="fusionSnapshot"
          :active="selected"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :prompt-composer-visible="!!props.fusionPromptComposerVisible"
          :ui-state="props.uiState"
          @node-mousedown="noopMouse"
          @update:title="emitNodeParam('title', $event)"
          @update:output-count="emitNodeParam('outputCount', $event)"
          @update:aspect-ratio="emitNodeParam('aspectRatio', $event)"
          @update:blend-point="emitNodeParam('blendPoint', $event)"
          @delete="onDeleteRequest"
          @update:input-images="emitNodeParam('inputImages', $event)"
          @input-image-drop="onMediaInputImageDrop"
          @input-image-remove="onMediaInputImageRemove"
          @contextmenu="onFusionContextMenu"
          @prompt-generate="onFusionPromptGenerate"
        />
      </div>
    </template>
    <template v-else>
      <div class="flow-canvas-node__unsupported">Unsupported node</div>
    </template>

    <div
      class="flow-canvas-node__handle-rail"
      :class="{ 'is-disabled': disableHandleRailInteraction }"
      :style="handleRailStyle"
    >
      <FlowNodeHandles
        :kind="data.kind"
        :snapshot-node="data.snapshotNode"
        :ui-state="props.uiState"
        :show-handles="showHandleControlsGated"
        @request-internals-update="emit('request-internals-update')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import { computed, nextTick, ref, watch } from 'vue'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { ImageStrengthAction } from '../execution'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import type { ImageEditorMode, ImageEditorRenderedPreview } from '../image-editor'
import { useFlowCanvasNodeAdapter } from '../composables/useFlowCanvasNodeAdapter'
import { useFlowCanvasNodeHandleRail } from '../composables/useFlowCanvasNodeHandleRail'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

const props = withDefaults(
  defineProps<{
    nodeId: string
    data: FlowNodeData
    selected?: boolean
    dragging?: boolean
    copyMode?: boolean
    uiState?: NodeUiState | PromptNodeUiState
    imageActionVisible?: boolean
    imagePromptComposerVisible?: boolean
    imagePromptPlaceholder?: string
    fusionPromptComposerVisible?: boolean
    imageDirectGenerating?: boolean
    showStrengthControl?: boolean
    strengthPct?: number
    setStrengthTrackRef?: (el: Element | ComponentPublicInstance | null) => void
    setImageStageRef?: (nodeId: string, element: Element | null) => void
    clearImageStageRef?: (nodeId: string, element?: Element | null) => void
    expandModeActive?: boolean
    expandOverlayStyle?: Record<string, string> | null
    expandHandleRadius?: number
    imageEditorMode?: ImageEditorMode | null
    imageEditorRenderedPreview?: ImageEditorRenderedPreview | null
    maskEditorOverlayStyle?: Record<string, string> | null
    maskEditorCanvasWidth?: number
    maskEditorCanvasHeight?: number
    maskLayerOpacity?: number
    imageLayerVisible?: boolean
    maskEditorBrushSize?: number
    maskEditorBrushShape?: 'round' | 'square'
    setMaskCanvasRef?: (nodeId: string, element: Element | null) => void
    handleVisibilityRefreshToken?: number
  }>(),
  {
    selected: false,
    dragging: false,
    copyMode: false,
    uiState: undefined,
    imageActionVisible: false,
    imagePromptComposerVisible: false,
    imagePromptPlaceholder: '输入提示词改造这张图',
    fusionPromptComposerVisible: false,
    imageDirectGenerating: false,
    showStrengthControl: false,
    strengthPct: 50,
    setStrengthTrackRef: () => undefined,
    setImageStageRef: () => undefined,
    clearImageStageRef: () => undefined,
    expandModeActive: false,
    expandOverlayStyle: null,
    expandHandleRadius: 4,
    imageEditorMode: null,
    imageEditorRenderedPreview: null,
    maskEditorOverlayStyle: null,
    maskEditorCanvasWidth: 0,
    maskEditorCanvasHeight: 0,
    maskLayerOpacity: 72,
    imageLayerVisible: true,
    maskEditorBrushSize: 18,
    maskEditorBrushShape: 'round',
    setMaskCanvasRef: () => undefined,
    handleVisibilityRefreshToken: 0,
  },
)

const emit = defineEmits<{
  (e: 'update-prompt-text', value: string): void
  (e: 'update-node-param', payload: FlowSnapshotParamPatch): void
  (e: 'delete-node'): void
  (e: 'image-stage-drop', event: DragEvent): void
  (e: 'media-input-image-drop', nodeId: string, event: DragEvent): void
  (e: 'media-input-image-open', nodeId: string): void
  (e: 'media-input-image-preview', image: { url: string; ratio: number }): void
  (e: 'media-input-image-remove', nodeId: string, imageId: string): void
  (e: 'image-open-file'): void
  (e: 'image-open-preview'): void
  (e: 'image-append-to-canvas'): void
  (e: 'image-reset'): void
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'image-contextmenu', event: MouseEvent): void
  (e: 'fusion-contextmenu', event: MouseEvent): void
  (e: 'image-prompt-generate', prompt: string, outputCount: number): void
  (e: 'image-expand-submit', prompt: string, outputCount: number): void
  (
    e: 'fusion-prompt-generate',
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): void
  (e: 'prompt-generate', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
  (e: 'image-action', action: ImageNodeAction): void
  (e: 'strength-action-mousedown', action: ImageStrengthAction, event: MouseEvent): void
  (e: 'strength-track-mousedown', event: MouseEvent): void
  (
    e: 'expand-handle-pointerdown',
    payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent },
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
  (e: 'mask-canvas-pointerdown', event: PointerEvent): void
  (e: 'request-internals-update'): void
}>()

const showDefaultDragZone = false
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const isImageEditorActive = computed(() => props.imageEditorMode != null)

const {
  disableHandleRailInteraction,
  displayImage,
  fusionSnapshot,
  imageEmptyIconVariant,
  imageGenerating,
  imageInputLocked,
  imageSnapshot,
  isFusionNode,
  isImageNode,
  isPromptNode,
  nodeRunning,
  promptSnapshot,
  promptUiState,
  stageHeight,
} = useFlowCanvasNodeAdapter({
  data: () => props.data,
  uiState: () => props.uiState,
  imageDirectGenerating: () => !!props.imageDirectGenerating,
  expandModeActive: () => !!props.expandModeActive,
  isImageEditorActive: () => isImageEditorActive.value,
})

const {
  handleRailStyle,
  showHandleControls,
  refreshTransientHandleVisibility,
  onNodeMouseEnter,
  onNodeMouseLeave,
  onNodeFocusIn,
  onNodeFocusOut,
  onNodePointerDown,
} = useFlowCanvasNodeHandleRail(flowCanvasNodeRef)

// Notify parent when the handle rail geometry changes (was in setHandleRailBounds)
watch(handleRailStyle, () => {
  void nextTick(() => emit('request-internals-update'))
})

watch(
  () => props.handleVisibilityRefreshToken,
  () => {
    refreshTransientHandleVisibility()
  },
)

const showHandleControlsGated = computed(
  () => !disableHandleRailInteraction.value && showHandleControls.value,
)

function onPromptTextUpdate(value: string) {
  emit('update-prompt-text', value)
}

function emitNodeParam<Key extends FlowSnapshotParamPatch['key']>(
  key: Key,
  value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
) {
  emit('update-node-param', { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>)
}

function onDeleteRequest() {
  emit('delete-node')
}

function onImageStageDrop(event: DragEvent) {
  emit('image-stage-drop', event)
}

function onImageOpenFile() {
  emit('image-open-file')
}

function onImageOpenPreview() {
  emit('image-open-preview')
}

function onImageAppendToCanvas() {
  emit('image-append-to-canvas')
}

function onImageReset() {
  emit('image-reset')
}

function onImageContextMenu(event: MouseEvent) {
  emit('image-contextmenu', event)
}
function onFusionContextMenu(event: MouseEvent) {
  emit('fusion-contextmenu', event)
}

function onMediaInputImageDrop(event: DragEvent) {
  emit('media-input-image-drop', props.nodeId, event)
}

function onMediaInputImageOpen() {
  emit('media-input-image-open', props.nodeId)
}

function onMediaInputImagePreview(image: { url: string; ratio: number }) {
  emit('media-input-image-preview', image)
}

function onMediaInputImageRemove(image: { id: string }) {
  emit('media-input-image-remove', props.nodeId, image.id)
}

function onNodeMouseDown(event: MouseEvent) {
  emit('node-mousedown', event)
}

function onImagePromptGenerate(prompt: string, outputCount: number) {
  emit('image-prompt-generate', prompt, outputCount)
}
function onImageExpandSubmit(prompt: string, outputCount: number) {
  emit('image-expand-submit', prompt, outputCount)
}
function onFusionPromptGenerate(
  prompt: string,
  outputCount: number,
  aspectRatio: PromptAspectRatio,
) {
  emit('fusion-prompt-generate', prompt, outputCount, aspectRatio)
}
function onPromptGenerate() {
  if (!promptSnapshot.value) return
  emit(
    'prompt-generate',
    promptSnapshot.value.text,
    promptSnapshot.value.outputCount ?? 1,
    promptSnapshot.value.aspectRatio ?? 'auto',
  )
}

function onImageAction(action: ImageNodeAction) {
  emit('image-action', action)
}
function onStrengthActionMouseDown(action: ImageStrengthAction, event: MouseEvent) {
  emit('strength-action-mousedown', action, event)
}

function onStrengthTrackMouseDown(event: MouseEvent) {
  emit('strength-track-mousedown', event)
}

function onImageExpandHandlePointerDown(payload: {
  anchor: CanvasExtendDragAnchor
  event: PointerEvent
}) {
  emit('expand-handle-pointerdown', payload)
}

function onImageExpandHandlePointerMove(event: PointerEvent) {
  emit('expand-handle-pointermove', event)
}

function onImageExpandHandlePointerUp(event: PointerEvent) {
  emit('expand-handle-pointerup', event)
}

function onImageExpandFramePointerDown(event: PointerEvent) {
  emit('expand-frame-pointerdown', event)
}

function noop() {}
function noopMouse() {}
</script>

<style scoped>
.flow-canvas-node {
  position: relative;
  isolation: isolate;
  min-width: 100px;
  min-height: 40px;
}

.flow-canvas-node__drag-zone {
  position: absolute;
  z-index: 20;
  pointer-events: auto;
  cursor: grab;
  touch-action: none;
}

.flow-canvas-node__drag-zone:active {
  cursor: grabbing;
}

.flow-canvas-node__drag-zone--default {
  top: 0;
  left: 0;
  right: 56px;
  height: 34px;
  min-width: 96px;
}

.flow-canvas-node__drag-surface {
  cursor: grab;
}

.flow-canvas-node__drag-surface:active {
  cursor: grabbing;
}

.flow-canvas-node__drag-surface.is-running,
.flow-canvas-node__drag-surface.is-running:active {
  cursor: default;
}

.flow-canvas-node__unsupported {
  padding: 8px 10px;
  border: 1px dashed #d0d7de;
  border-radius: 8px;
  background: #fff;
  color: #6b7280;
  font-size: 12px;
}

.flow-canvas-node__handle-rail {
  position: absolute;
  z-index: 24;
  pointer-events: none;
  overflow: visible;
}

.flow-canvas-node__handle-rail.is-disabled :deep(.flow-canvas-node__handle-aggregate),
.flow-canvas-node__handle-rail.is-disabled :deep(.flow-canvas-node__handle-plus),
.flow-canvas-node__handle-rail.is-disabled :deep(.flow-canvas-node__handle-aggregate::before) {
  pointer-events: none;
  opacity: 0;
}

.flow-canvas-node :deep(.image-action-menu) {
  top: calc(100% + 10px);
  z-index: 30;
}
</style>
