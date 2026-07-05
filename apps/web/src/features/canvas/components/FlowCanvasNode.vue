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
    <template v-if="shouldRenderLightweightNode">
      <NodeShell
        :kind="data.kind"
        :node-id="lightweightNodeId"
        :width="lightweightNodeWidth"
        :title="lightweightNodeTitle"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :rounded-header="data.kind !== 'prompt'"
        @node-mousedown="onNodeMouseDown"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
      >
        <template #icon>
          <ChatLines v-if="data.kind === 'prompt'" />
          <MediaImage v-else-if="data.kind === 'image'" />
          <Keyframes v-else />
        </template>
        <div
          class="flow-canvas-node__lightweight-body"
          :class="`flow-canvas-node__lightweight-body--${data.kind}`"
          :style="lightweightBodyStyle"
          data-canvas-lightweight-node="true"
          :aria-label="lightweightBodyAriaLabel"
        >
          <div class="flow-canvas-node__lightweight-mark" aria-hidden="true" />
          <span class="flow-canvas-node__lightweight-label">{{ lightweightNodeKindLabel }}</span>
        </div>
        <div
          class="flow-canvas-node__handle-rail"
          :class="{ 'is-disabled': disableHandleRailInteraction }"
        >
          <FlowNodeHandles
            :kind="data.kind"
            :snapshot-node="data.snapshotNode"
            :ui-state="props.uiState"
            :show-handles="false"
            @request-internals-update="requestNodeInternalsUpdate"
          />
        </div>
      </NodeShell>
    </template>
    <template v-else-if="isPromptNode && promptSnapshot">
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
      >
        <template #handle-rail>
          <div
            class="flow-canvas-node__handle-rail"
            :class="{ 'is-disabled': disableHandleRailInteraction }"
          >
            <FlowNodeHandles
              :kind="data.kind"
              :snapshot-node="data.snapshotNode"
              :ui-state="props.uiState"
              :show-handles="showHandleControlsGated"
              @request-internals-update="requestNodeInternalsUpdate"
            />
          </div>
        </template>
      </PromptNode>
    </template>
    <template v-else-if="isImageNode && imageSnapshot">
      <div
        class="flow-canvas-node__drag-surface relative overflow-visible"
        :class="{ 'is-running': nodeRunning }"
        @mousedown="onNodeMouseDown"
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
          :action-binding="resolvedActionBinding"
          :expand-mode-active="!!props.expandModeActive"
          :expand-overlay-style="props.expandOverlayStyle ?? null"
          :expand-handle-radius="props.expandHandleRadius ?? 4"
          :image-tool-stage-binding="resolvedStageBinding"
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
          @expand-handle-pointerdown="onImageExpandHandlePointerDown"
          @expand-handle-pointermove="onImageExpandHandlePointerMove"
          @expand-handle-pointerup="onImageExpandHandlePointerUp"
          @expand-frame-pointerdown="onImageExpandFramePointerDown"
        >
          <template #handle-rail>
            <div
              class="flow-canvas-node__handle-rail"
              :class="{ 'is-disabled': disableHandleRailInteraction }"
            >
              <FlowNodeHandles
                :kind="data.kind"
                :snapshot-node="data.snapshotNode"
                :ui-state="props.uiState"
                :show-handles="showHandleControlsGated"
                @request-internals-update="requestNodeInternalsUpdate"
              />
            </div>
          </template>
        </ImageNode>
      </div>
    </template>
    <template v-else-if="isFusionNode && fusionSnapshot">
      <div
        class="flow-canvas-node__drag-surface relative overflow-visible"
        :class="{ 'is-running': nodeRunning }"
        @mousedown="onNodeMouseDown"
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
        >
          <template #handle-rail>
            <div
              class="flow-canvas-node__handle-rail"
              :class="{ 'is-disabled': disableHandleRailInteraction }"
            >
              <FlowNodeHandles
                :kind="data.kind"
                :snapshot-node="data.snapshotNode"
                :ui-state="props.uiState"
                :show-handles="showHandleControlsGated"
                @request-internals-update="requestNodeInternalsUpdate"
              />
            </div>
          </template>
        </FusionNode>
      </div>
    </template>
    <template v-else>
      <div class="flow-canvas-node__unsupported">Unsupported node</div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, type StyleValue } from 'vue'
import { ChatLines, Keyframes, MediaImage } from '@iconoir/vue'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import type { ImageActionBinding, ImageToolStageBinding } from '../image-tools'
import { useFlowCanvasNodeAdapter } from '../composables/useFlowCanvasNodeAdapter'
import { useFlowCanvasNodeHandleRail } from '../composables/useFlowCanvasNodeHandleRail'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import FlowNodeHandles from './FlowNodeHandles.vue'
import FusionNode from './nodes/FusionNode.vue'
import ImageNode from './nodes/ImageNode.vue'
import NodeShell from './nodes/NodeShell.vue'
import PromptNode from './nodes/PromptNode.vue'

const props = withDefaults(
  defineProps<{
    nodeId: string
    data: FlowNodeData
    selected?: boolean
    dragging?: boolean
    copyMode?: boolean
    uiState?: NodeUiState | PromptNodeUiState
    fusionPromptComposerVisible?: boolean
    imageActionBinding: ImageActionBinding
    setImageStageRef?: (nodeId: string, element: Element | null) => void
    clearImageStageRef?: (nodeId: string, element?: Element | null) => void
    expandModeActive?: boolean
    expandOverlayStyle?: Record<string, string> | null
    expandHandleRadius?: number
    imageToolStageBinding?: ImageToolStageBinding | null
    handleVisibilityRefreshToken?: number
    lightweightNodeRendering?: boolean
  }>(),
  {
    selected: false,
    dragging: false,
    copyMode: false,
    uiState: undefined,
    fusionPromptComposerVisible: false,
    setImageStageRef: () => undefined,
    clearImageStageRef: () => undefined,
    expandModeActive: false,
    expandOverlayStyle: null,
    expandHandleRadius: 4,
    imageToolStageBinding: null,
    handleVisibilityRefreshToken: 0,
    lightweightNodeRendering: false,
  },
)

const emit = defineEmits<{
  (e: 'update-prompt-text', nodeId: string, value: string): void
  (e: 'update-node-param', nodeId: string, payload: FlowSnapshotParamPatch): void
  (e: 'delete-node', nodeId: string): void
  (e: 'image-stage-drop', nodeId: string, event: DragEvent): void
  (e: 'media-input-image-drop', nodeId: string, event: DragEvent): void
  (e: 'media-input-image-open', nodeId: string): void
  (e: 'media-input-image-preview', image: { url: string; ratio: number }): void
  (e: 'media-input-image-remove', nodeId: string, imageId: string): void
  (e: 'image-open-file', nodeId: string): void
  (e: 'image-open-preview', nodeId: string): void
  (e: 'image-append-to-canvas', nodeId: string): void
  (e: 'image-reset', nodeId: string): void
  (e: 'node-mousedown', nodeId: string, event: MouseEvent): void
  (e: 'image-contextmenu', nodeId: string, event: MouseEvent): void
  (e: 'fusion-contextmenu', nodeId: string, event: MouseEvent): void
  (e: 'image-prompt-generate', nodeId: string, prompt: string, outputCount: number): void
  (e: 'image-expand-submit', nodeId: string, prompt: string, outputCount: number): void
  (
    e: 'fusion-prompt-generate',
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): void
  (
    e: 'prompt-generate',
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ): void
  (e: 'image-action', nodeId: string, action: ImageNodeAction): void
  (
    e: 'expand-handle-pointerdown',
    nodeId: string,
    anchor: CanvasExtendDragAnchor,
    event: PointerEvent,
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', nodeId: string, event: PointerEvent): void
  (e: 'request-internals-update', nodeId: string): void
}>()

const showDefaultDragZone = false
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const isImageEditorActive = computed(() => props.imageToolStageBinding?.mode != null)
const resolvedActionBinding = computed(() => props.imageActionBinding)
const resolvedStageBinding = computed<ImageToolStageBinding | null>(
  () => props.imageToolStageBinding ?? null,
)

const {
  disableHandleRailInteraction,
  displayImage,
  fusionSnapshot,
  imageEmptyIconVariant,
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
  imageDirectGenerating: () => resolvedActionBinding.value.directGenerating,
  expandModeActive: () => !!props.expandModeActive,
  isImageEditorActive: () => isImageEditorActive.value,
})

const isNodeInteractiveAtRest = computed(
  () =>
    props.selected &&
    !props.dragging &&
    !nodeRunning.value &&
    !props.copyMode &&
    !props.expandModeActive,
)
const hasActiveNodeChrome = computed(
  () =>
    resolvedActionBinding.value.actionMenuVisible ||
    resolvedActionBinding.value.promptComposerVisible ||
    !!props.fusionPromptComposerVisible ||
    isImageEditorActive.value ||
    !!props.expandModeActive ||
    !!resolvedStageBinding.value?.mode,
)
const canRenderLightweightNode = computed(
  () =>
    props.lightweightNodeRendering &&
    !hasActiveNodeChrome.value &&
    !nodeRunning.value &&
    (isPromptNode.value || isImageNode.value || isFusionNode.value),
)
const shouldRenderLightweightNode = computed(
  () => canRenderLightweightNode.value && !isNodeInteractiveAtRest.value,
)
const lightweightSnapshot = computed(
  () => promptSnapshot.value ?? imageSnapshot.value ?? fusionSnapshot.value ?? null,
)
const lightweightNodeId = computed(() => lightweightSnapshot.value?.id ?? props.nodeId)
const lightweightNodeWidth = computed(() => lightweightSnapshot.value?.width ?? 280)
const lightweightNodeTitle = computed(() => {
  const fallback = lightweightNodeKindLabel.value
  return `${lightweightSnapshot.value?.title ?? ''}`.trim() || fallback
})
const lightweightNodeKindLabel = computed(() => {
  if (props.data.kind === 'prompt') return 'Prompt'
  if (props.data.kind === 'image') return 'Image'
  return 'Fusion'
})
const lightweightBodyAriaLabel = computed(() => `${lightweightNodeKindLabel.value} 节点轻量预览`)
const lightweightBodyStyle = computed<StyleValue>(() => {
  const height =
    props.data.kind === 'image'
      ? stageHeight.value
      : props.data.kind === 'fusion'
        ? lightweightNodeWidth.value
        : 280
  return { height: `${height}px` }
})

const {
  showHandleControls,
  refreshTransientHandleVisibility,
  onNodeMouseEnter,
  onNodeMouseLeave,
  onNodeFocusIn,
  onNodeFocusOut,
  onNodePointerDown,
} = useFlowCanvasNodeHandleRail(flowCanvasNodeRef)

watch(
  () => props.handleVisibilityRefreshToken,
  () => {
    refreshTransientHandleVisibility()
  },
)

const showHandleControlsGated = computed(
  () => !disableHandleRailInteraction.value && showHandleControls.value,
)

function requestNodeInternalsUpdate() {
  emit('request-internals-update', props.nodeId)
}

function onPromptTextUpdate(value: string) {
  emit('update-prompt-text', props.nodeId, value)
}

function emitNodeParam<Key extends FlowSnapshotParamPatch['key']>(
  key: Key,
  value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
) {
  emit('update-node-param', props.nodeId, { key, value } as Extract<
    FlowSnapshotParamPatch,
    { key: Key }
  >)
}

function onDeleteRequest() {
  emit('delete-node', props.nodeId)
}

function onImageStageDrop(event: DragEvent) {
  emit('image-stage-drop', props.nodeId, event)
}

function onImageOpenFile() {
  emit('image-open-file', props.nodeId)
}

function onImageOpenPreview() {
  emit('image-open-preview', props.nodeId)
}

function onImageAppendToCanvas() {
  emit('image-append-to-canvas', props.nodeId)
}

function onImageReset() {
  emit('image-reset', props.nodeId)
}

function onImageContextMenu(event: MouseEvent) {
  emit('image-contextmenu', props.nodeId, event)
}
function onFusionContextMenu(event: MouseEvent) {
  emit('fusion-contextmenu', props.nodeId, event)
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
  emit('node-mousedown', props.nodeId, event)
}

function onImagePromptGenerate(prompt: string, outputCount: number) {
  emit('image-prompt-generate', props.nodeId, prompt, outputCount)
}
function onImageExpandSubmit(prompt: string, outputCount: number) {
  emit('image-expand-submit', props.nodeId, prompt, outputCount)
}
function onFusionPromptGenerate(
  prompt: string,
  outputCount: number,
  aspectRatio: PromptAspectRatio,
) {
  emit('fusion-prompt-generate', props.nodeId, prompt, outputCount, aspectRatio)
}
function onPromptGenerate() {
  if (!promptSnapshot.value) return
  emit(
    'prompt-generate',
    props.nodeId,
    promptSnapshot.value.text,
    promptSnapshot.value.outputCount ?? 1,
    promptSnapshot.value.aspectRatio ?? 'auto',
  )
}

function onImageAction(action: ImageNodeAction) {
  emit('image-action', props.nodeId, action)
}
function onImageExpandHandlePointerDown(payload: {
  anchor: CanvasExtendDragAnchor
  event: PointerEvent
}) {
  emit('expand-handle-pointerdown', props.nodeId, payload.anchor, payload.event)
}

function onImageExpandHandlePointerMove(event: PointerEvent) {
  emit('expand-handle-pointermove', event)
}

function onImageExpandHandlePointerUp(event: PointerEvent) {
  emit('expand-handle-pointerup', event)
}

function onImageExpandFramePointerDown(event: PointerEvent) {
  emit('expand-frame-pointerdown', props.nodeId, event)
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

.flow-canvas-node__lightweight-body {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  width: 100%;
  min-height: 120px;
  border: 1.5px solid var(--color-canvas-node-border-alt);
  border-radius: 11px;
  background: var(--color-canvas-node-bg);
  color: var(--color-canvas-text-muted);
  overflow: hidden;
  user-select: none;
}

.flow-canvas-node__lightweight-body--prompt {
  border-radius: 16px;
}

.flow-canvas-node__lightweight-mark {
  position: absolute;
  inset: 12px;
  border: 1px dashed color-mix(in srgb, var(--color-canvas-text-muted) 42%, transparent);
  border-radius: inherit;
  opacity: 0.42;
}

.flow-canvas-node__lightweight-label {
  position: relative;
  z-index: 1;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0;
  text-transform: uppercase;
  opacity: 0.58;
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
  inset: 0;
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
