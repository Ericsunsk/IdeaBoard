<template>
  <div
    ref="flowCanvasNodeRef"
    class="flow-canvas-node"
    :data-flow-node-id="props.nodeId"
    :class="[`is-${data.kind}`, { 'is-selected': selected, 'is-running': nodeRunning }]"
    :style="nodeStyle"
  >
    <div
      v-if="showDefaultDragZone"
      class="flow-canvas-node__drag-zone flow-canvas-node__drag-zone--default"
    />
    <template v-if="isPromptNode && promptSnapshot">
      <PromptNode
        :node="promptSnapshot"
        :active="selected"
        :connection-drop-target="props.connectionDropTarget"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="promptUiState"
        :generation-available="props.generationAvailable"
        @update:text="onPromptTextEdited"
        @update:output-count="onPromptParameterChanged('outputCount', $event)"
        @update:output-resolution="onPromptParameterChanged('outputResolution', $event)"
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
      />
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
          :connection-drop-target="props.connectionDropTarget"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :stage-height="stageHeight"
          :display-image="displayImage"
          :library-import="libraryImport"
          :local-upload="localUpload"
          :empty-icon-variant="imageEmptyIconVariant"
          :input-locked="imageInputLocked"
          :action-binding="props.imagePresentation.actionBinding"
          :outpaint-mode-active="!!props.outpaintModeActive"
          :outpaint-overlay-style="props.outpaintOverlayStyle ?? null"
          :outpaint-handle-radius="props.outpaintHandleRadius ?? 4"
          :image-tool-stage-binding="props.imagePresentation.stageBinding"
          @node-mousedown="noopMouse"
          @update:title="onImageParameterChanged('title', $event)"
          @update:output-count="onImageParameterChanged('outputCount', $event)"
          @update:output-resolution="onImageParameterChanged('outputResolution', $event)"
          @delete="onImageDeleteRequest"
          @stage-drop="onImageStageDropped"
          @open-file="onImageFileOpenRequest"
          @open-preview="onImagePreviewRequest"
          @append-to-canvas="onImageAppendToCanvasRequest"
          @reset="onImageResetRequest"
          @retry-image-import="onImageImportRetryRequest"
          @contextmenu="onImageContextMenuRequest"
          @outpaint-submit="onImageOutpaintSubmit"
          @action="onImageActionRequest"
          @outpaint-handle-pointerdown="onImageOutpaintHandlePointerDown"
          @outpaint-handle-pointermove="onImageOutpaintHandlePointerMove"
          @outpaint-handle-pointerup="onImageOutpaintHandlePointerUp"
          @outpaint-frame-pointerdown="onImageOutpaintFramePointerDown"
        />
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
          :connection-drop-target="props.connectionDropTarget"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :prompt-composer-visible="props.fusionPresentation?.promptComposerVisible ?? false"
          :generation-available="props.generationAvailable"
          :ui-state="resolvedUiState"
          @node-mousedown="noopMouse"
          @update:title="onFusionParameterChanged('title', $event)"
          @update:output-count="onFusionParameterChanged('outputCount', $event)"
          @update:aspect-ratio="onFusionParameterChanged('aspectRatio', $event)"
          @update:output-resolution="onFusionParameterChanged('outputResolution', $event)"
          @update:blend-point="onFusionParameterChanged('blendPoint', $event)"
          @delete="onFusionDeleteRequest"
          @update:input-images="onFusionParameterChanged('inputImages', $event)"
          @input-image-drop="onFusionMediaInputDrop"
          @input-image-remove="onFusionMediaInputRemove"
          @contextmenu="onFusionContextMenuRequest"
          @prompt-generate="onFusionGenerationRequest"
        />
      </div>
    </template>
    <template v-else>
      <div class="flow-canvas-node__unsupported">{{ t('canvas.node.unsupported') }}</div>
    </template>
    <FlowCanvasNodeHandleRail
      v-if="hasRenderedNode"
      :node-id="props.nodeId"
      :kind="data.kind"
      :interaction-root="flowCanvasNodeRef"
      :dragging="props.dragging"
      :disabled="disableHandleRailInteraction"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { NodeUiState, PromptNodeUiState } from '../../domain/nodes/canvasNodeUi.ts'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
import { useFlowCanvasNodeAdapter } from '../../infrastructure/vue-flow-adapter/useFlowCanvasNodeAdapter'
import type {
  CanvasNodePresentationIntent,
  FusionNodePresentationBinding,
  ImageNodePresentationBinding,
} from '.'
import { useFlowCanvasNodeIntents } from './useFlowCanvasNodeIntents'
import {
  resolveCanvasNodeUiScale,
  useCanvasViewportContext,
} from '../../infrastructure/vue-flow-adapter/useCanvasViewportContext'
import { useCanvasNodeUiStateContext } from '../../infrastructure/vue-flow-adapter/useCanvasNodeUiStateContext'
import FlowCanvasNodeHandleRail from './FlowCanvasNodeHandleRail.vue'
import { FusionNode, ImageNode, PromptNode } from './workflows'

const props = withDefaults(
  defineProps<{
    nodeId: string
    data: FlowNodeData
    selected?: boolean
    connectionDropTarget?: boolean
    dragging?: boolean
    copyMode?: boolean
    uiState?: NodeUiState | PromptNodeUiState | undefined
    fusionPresentation?: FusionNodePresentationBinding | undefined
    generationAvailable?: boolean
    imagePresentation: ImageNodePresentationBinding
    setImageStageRef?: ((nodeId: string, element: Element | null) => void) | undefined
    clearImageStageRef?: ((nodeId: string, element?: Element | null) => void) | undefined
    outpaintModeActive?: boolean
    outpaintOverlayStyle?: Record<string, string> | null
    outpaintHandleRadius?: number
  }>(),
  {
    selected: false,
    connectionDropTarget: false,
    dragging: false,
    copyMode: false,
    uiState: undefined,
    fusionPresentation: undefined,
    generationAvailable: true,
    setImageStageRef: () => undefined,
    clearImageStageRef: () => undefined,
    outpaintModeActive: false,
    outpaintOverlayStyle: null,
    outpaintHandleRadius: 4,
  },
)

const emit = defineEmits<{
  (e: 'node-intent', intent: CanvasNodePresentationIntent): void
  (e: 'node-mousedown', nodeId: string, event: MouseEvent): void
}>()

const { t } = useI18n()

const showDefaultDragZone = false
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const isRetouchActive = computed(() => props.imagePresentation.stageBinding?.mode != null)
const { zoomLevel } = useCanvasViewportContext()
const nodeStyle = computed(() => ({
  '--canvas-node-ui-scale': String(resolveCanvasNodeUiScale(zoomLevel.value)),
}))

// Per-node uiState subscription: resolving the shared cache here keeps the
// dependency inside this node's own render effect instead of the parent
// CanvasScreen slot scope, so one node's uiState change cannot re-render
// sibling subtrees. An explicit uiState prop (tests, previews) still wins.
const nodeUiStateContext = useCanvasNodeUiStateContext()
const resolvedUiState = computed(
  () => props.uiState ?? nodeUiStateContext.getFlowNodeUiState(props.nodeId),
)

const {
  disableHandleRailInteraction,
  displayImage,
  fusionSnapshot,
  imageEmptyIconVariant,
  imageInputLocked,
  libraryImport,
  localUpload,
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
  uiState: () => resolvedUiState.value,
  imageDirectGenerating: () => props.imagePresentation.actionBinding.directGenerating,
  outpaintModeActive: () => !!props.outpaintModeActive,
  isRetouchActive: () => isRetouchActive.value,
})
const hasRenderedNode = computed(
  () =>
    promptSnapshot.value !== null || imageSnapshot.value !== null || fusionSnapshot.value !== null,
)

const {
  onPromptTextEdited,
  onPromptParameterChanged,
  onImageParameterChanged,
  onImageDeleteRequest,
  onPromptDeleteRequest,
  onImageStageDropped,
  onImageFileOpenRequest,
  onImagePreviewRequest,
  onImageAppendToCanvasRequest,
  onImageResetRequest,
  onImageImportRetryRequest,
  onImageContextMenuRequest,
  onPromptMediaInputDrop,
  onPromptMediaInputOpen,
  onPromptMediaInputPreview,
  onPromptMediaInputRemove,
  onNodeMouseDown,
  onImageOutpaintSubmit,
  onFusionParameterChanged,
  onFusionDeleteRequest,
  onFusionMediaInputDrop,
  onFusionMediaInputRemove,
  onFusionContextMenuRequest,
  onFusionGenerationRequest,
  onPromptGenerate,
  onImageActionRequest,
  onImageOutpaintHandlePointerDown,
  onImageOutpaintHandlePointerMove,
  onImageOutpaintHandlePointerUp,
  onImageOutpaintFramePointerDown,
} = useFlowCanvasNodeIntents({
  nodeId: () => props.nodeId,
  promptSnapshot: () => promptSnapshot.value,
  emit: {
    nodeIntent: (intent) => emit('node-intent', intent),
    nodeMousedown: (nodeId, event) => emit('node-mousedown', nodeId, event),
  },
})

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
  border: 1px dashed var(--canvas-node-border);
  border-radius: var(--canvas-radius-md);
  background: var(--canvas-node-bg);
  color: var(--canvas-text-muted);
  font-size: 12px;
}
</style>
