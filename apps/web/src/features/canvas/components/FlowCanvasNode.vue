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
        :generation-available="props.generationAvailable"
        @update:text="onPromptTextEdited"
        @update:output-count="onPromptParameterChanged('outputCount', $event)"
        @update:aspect-ratio="onPromptParameterChanged('aspectRatio', $event)"
        @update:input-images="onPromptParameterChanged('inputImages', $event)"
        @update:title="onPromptParameterChanged('title', $event)"
        @input-image-drop="onPromptMediaInputDrop"
        @input-image-open="onPromptMediaInputOpen"
        @input-image-preview="onPromptMediaInputPreview"
        @input-image-remove="onPromptMediaInputRemove"
        @node-mousedown="noopMouse"
        @delete="onPromptDeleteRequest"
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
              :disabled="disableHandleRailInteraction"
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
          :action-binding="props.imagePresentation.actionBinding"
          :expand-mode-active="!!props.expandModeActive"
          :expand-overlay-style="props.expandOverlayStyle ?? null"
          :expand-handle-radius="props.expandHandleRadius ?? 4"
          :image-tool-stage-binding="props.imagePresentation.stageBinding"
          @node-mousedown="noopMouse"
          @update:title="onImageParameterChanged('title', $event)"
          @update:output-count="onImageParameterChanged('outputCount', $event)"
          @delete="onImageDeleteRequest"
          @stage-drop="onImageStageDropped"
          @open-file="onImageFileOpenRequest"
          @open-preview="onImagePreviewRequest"
          @append-to-canvas="onImageAppendToCanvasRequest"
          @reset="onImageResetRequest"
          @contextmenu="onImageContextMenuRequest"
          @prompt-generate="onImagePromptGenerationRequest"
          @expand-submit="onImageCanvasExtendSubmit"
          @action="onImageActionRequest"
          @expand-handle-pointerdown="onImageCanvasExtendHandlePointerDown"
          @expand-handle-pointermove="onImageCanvasExtendHandlePointerMove"
          @expand-handle-pointerup="onImageCanvasExtendHandlePointerUp"
          @expand-frame-pointerdown="onImageCanvasExtendFramePointerDown"
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
                :disabled="disableHandleRailInteraction"
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
          :prompt-composer-visible="props.fusionPresentation?.promptComposerVisible ?? false"
          :generation-available="props.generationAvailable"
          :ui-state="props.uiState"
          @node-mousedown="noopMouse"
          @update:title="onFusionParameterChanged('title', $event)"
          @update:output-count="onFusionParameterChanged('outputCount', $event)"
          @update:aspect-ratio="onFusionParameterChanged('aspectRatio', $event)"
          @update:blend-point="onFusionParameterChanged('blendPoint', $event)"
          @delete="onFusionDeleteRequest"
          @update:input-images="onFusionParameterChanged('inputImages', $event)"
          @input-image-drop="onFusionMediaInputDrop"
          @input-image-remove="onFusionMediaInputRemove"
          @contextmenu="onFusionContextMenuRequest"
          @prompt-generate="onFusionGenerationRequest"
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
                :disabled="disableHandleRailInteraction"
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
import { computed, ref, watch } from 'vue'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import { useFlowCanvasNodeAdapter } from '../composables/useFlowCanvasNodeAdapter'
import { useFlowCanvasNodeHandleRail } from '../composables/useFlowCanvasNodeHandleRail'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type {
  CanvasNodePresentationIntent,
  FusionNodePresentationBinding,
  ImageNodePresentationBinding,
  ImageNodePresentationIntent,
} from '../node-presentation'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import FlowNodeHandles from './FlowNodeHandles.vue'
import FusionNode from './nodes/FusionNode.vue'
import ImageNode from './nodes/ImageNode.vue'
import PromptNode from './nodes/PromptNode.vue'

const props = withDefaults(
  defineProps<{
    nodeId: string
    data: FlowNodeData
    selected?: boolean
    dragging?: boolean
    copyMode?: boolean
    uiState?: NodeUiState | PromptNodeUiState
    fusionPresentation?: FusionNodePresentationBinding
    generationAvailable?: boolean
    imagePresentation: ImageNodePresentationBinding
    setImageStageRef?: (nodeId: string, element: Element | null) => void
    clearImageStageRef?: (nodeId: string, element?: Element | null) => void
    expandModeActive?: boolean
    expandOverlayStyle?: Record<string, string> | null
    expandHandleRadius?: number
    handleVisibilityRefreshToken?: number
  }>(),
  {
    selected: false,
    dragging: false,
    copyMode: false,
    uiState: undefined,
    fusionPresentation: undefined,
    generationAvailable: true,
    setImageStageRef: () => undefined,
    clearImageStageRef: () => undefined,
    expandModeActive: false,
    expandOverlayStyle: null,
    expandHandleRadius: 4,
    handleVisibilityRefreshToken: 0,
  },
)

const emit = defineEmits<{
  (e: 'node-intent', intent: CanvasNodePresentationIntent): void
  (e: 'node-mousedown', nodeId: string, event: MouseEvent): void
  (e: 'request-internals-update', nodeId: string): void
}>()

const showDefaultDragZone = false
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const isImageEditorActive = computed(() => props.imagePresentation.stageBinding?.mode != null)

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
  imageDirectGenerating: () => props.imagePresentation.actionBinding.directGenerating,
  expandModeActive: () => !!props.expandModeActive,
  isImageEditorActive: () => isImageEditorActive.value,
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

function onPromptTextEdited(text: string) {
  emit('node-intent', { nodeKind: 'prompt', type: 'text-edited', nodeId: props.nodeId, text })
}

function onPromptParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
  key: Key,
  value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
) {
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'parameter-changed',
    nodeId: props.nodeId,
    patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
  })
}

function onImageParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
  key: Key,
  value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'parameter-changed',
    nodeId: props.nodeId,
    patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
  })
}

function onImageDeleteRequest() {
  emit('node-intent', { nodeKind: 'image', type: 'delete-requested', nodeId: props.nodeId })
}

function onPromptDeleteRequest() {
  emit('node-intent', { nodeKind: 'prompt', type: 'delete-requested', nodeId: props.nodeId })
}

function onImageStageDropped(event: DragEvent) {
  emit('node-intent', { nodeKind: 'image', type: 'stage-dropped', nodeId: props.nodeId, event })
}

function onImageFileOpenRequest() {
  emit('node-intent', { nodeKind: 'image', type: 'file-open-requested', nodeId: props.nodeId })
}

function onImagePreviewRequest() {
  emit('node-intent', { nodeKind: 'image', type: 'preview-requested', nodeId: props.nodeId })
}

function onImageAppendToCanvasRequest() {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'append-to-canvas-requested',
    nodeId: props.nodeId,
  })
}

function onImageResetRequest() {
  emit('node-intent', { nodeKind: 'image', type: 'reset-requested', nodeId: props.nodeId })
}

function onImageContextMenuRequest(event: MouseEvent) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'context-menu-requested',
    nodeId: props.nodeId,
    event,
  })
}

function onPromptMediaInputDrop(event: DragEvent) {
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'media-input-dropped',
    nodeId: props.nodeId,
    event,
  })
}

function onPromptMediaInputOpen() {
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'media-input-open-requested',
    nodeId: props.nodeId,
  })
}

function onPromptMediaInputPreview(image: { url: string; ratio: number }) {
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'media-input-preview-requested',
    nodeId: props.nodeId,
    image,
  })
}

function onPromptMediaInputRemove(image: { id: string }) {
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'media-input-remove-requested',
    nodeId: props.nodeId,
    imageId: image.id,
  })
}

function onNodeMouseDown(event: MouseEvent) {
  emit('node-mousedown', props.nodeId, event)
}

function onImagePromptGenerationRequest(prompt: string, outputCount: number) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'prompt-generation-requested',
    nodeId: props.nodeId,
    prompt,
    outputCount,
  })
}
function onImageCanvasExtendSubmit(prompt: string, outputCount: number) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'canvas-extend-submitted',
    nodeId: props.nodeId,
    prompt,
    outputCount,
  })
}
function onFusionParameterChanged<Key extends FlowSnapshotParamPatch['key']>(
  key: Key,
  value: Extract<FlowSnapshotParamPatch, { key: Key }>['value'],
) {
  emit('node-intent', {
    nodeKind: 'fusion',
    type: 'parameter-changed',
    nodeId: props.nodeId,
    patch: { key, value } as Extract<FlowSnapshotParamPatch, { key: Key }>,
  })
}

function onFusionDeleteRequest() {
  emit('node-intent', { nodeKind: 'fusion', type: 'delete-requested', nodeId: props.nodeId })
}

function onFusionMediaInputDrop(event: DragEvent) {
  emit('node-intent', {
    nodeKind: 'fusion',
    type: 'media-input-dropped',
    nodeId: props.nodeId,
    event,
  })
}

function onFusionMediaInputRemove(image: { id: string }) {
  emit('node-intent', {
    nodeKind: 'fusion',
    type: 'media-input-remove-requested',
    nodeId: props.nodeId,
    imageId: image.id,
  })
}

function onFusionContextMenuRequest(event: MouseEvent) {
  emit('node-intent', {
    nodeKind: 'fusion',
    type: 'context-menu-requested',
    nodeId: props.nodeId,
    event,
  })
}

function onFusionGenerationRequest(
  prompt: string,
  outputCount: number,
  aspectRatio: PromptAspectRatio,
) {
  emit('node-intent', {
    nodeKind: 'fusion',
    type: 'generation-requested',
    nodeId: props.nodeId,
    prompt,
    outputCount,
    aspectRatio,
  })
}
function onPromptGenerate() {
  if (!promptSnapshot.value) return
  emit('node-intent', {
    nodeKind: 'prompt',
    type: 'generation-requested',
    nodeId: props.nodeId,
    prompt: promptSnapshot.value.text,
    outputCount: promptSnapshot.value.outputCount ?? 1,
    aspectRatio: promptSnapshot.value.aspectRatio ?? 'auto',
  })
}

type ImageActionRequest = Extract<ImageNodePresentationIntent, { type: 'action-requested' }>

function onImageActionRequest(action: ImageActionRequest['action']) {
  emit('node-intent', { nodeKind: 'image', type: 'action-requested', nodeId: props.nodeId, action })
}
function onImageCanvasExtendHandlePointerDown(payload: {
  anchor: CanvasExtendDragAnchor
  event: PointerEvent
}) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'canvas-extend-handle-pointerdown',
    nodeId: props.nodeId,
    anchor: payload.anchor,
    event: payload.event,
  })
}

function onImageCanvasExtendHandlePointerMove(event: PointerEvent) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'canvas-extend-handle-pointermove',
    nodeId: props.nodeId,
    event,
  })
}

function onImageCanvasExtendHandlePointerUp(event: PointerEvent) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'canvas-extend-handle-pointerup',
    nodeId: props.nodeId,
    event,
  })
}

function onImageCanvasExtendFramePointerDown(event: PointerEvent) {
  emit('node-intent', {
    nodeKind: 'image',
    type: 'canvas-extend-frame-pointerdown',
    nodeId: props.nodeId,
    event,
  })
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
  inset: 0;
  z-index: 24;
  pointer-events: none;
  overflow: visible;
}
</style>
