<template>
  <div
    ref="flowCanvasNodeRef"
    class="flow-canvas-node"
    :class="[`is-${data.kind}`, { 'is-selected': selected }]"
    @mouseenter="isPointerOverNode = true"
    @mouseleave="isPointerOverNode = false"
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
        @update:title="emitNodeParam('title', $event)"
        @node-mousedown="noopMouse"
        @delete="onDeleteRequest"
        @focus="noop"
      />
    </template>
    <template v-else-if="isImageNode && imageSnapshot">
      <div
        class="flow-canvas-node__image-wrap flow-canvas-node__drag-surface"
        @click.stop
        @contextmenu.stop
      >
        <ImageNode
          :node="imageSnapshot"
          :active="selected"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :ui-state="props.uiState"
          :stage-height="stageHeight"
          :display-image="displayImage"
          :display-mode="displayMode"
          :action-menu-visible="!!props.imageActionVisible"
          :prompt-composer-visible="!!props.imagePromptComposerVisible"
          :direct-generating="!!props.imageDirectGenerating"
          :show-style-strength-control="!!props.showStyleStrengthControl"
          :style-innovate-strength-pct="props.styleInnovateStrengthPct ?? 50"
          :set-style-strength-track-ref="props.setStyleStrengthTrackRef ?? (() => {})"
          @node-mousedown="noopMouse"
          @update:title="emitNodeParam('title', $event)"
          @delete="onDeleteRequest"
          @stage-drop="onImageStageDrop"
          @open-file="onImageOpenFile"
          @append-to-canvas="onImageAppendToCanvas"
          @reset="onImageReset"
          @contextmenu="onImageContextMenu"
          @prompt-generate="onImagePromptGenerate"
          @action="onImageAction"
          @style-innovate-mousedown="onStyleInnovateMouseDown"
          @style-strength-track-mousedown="onStyleStrengthTrackMouseDown"
        />
      </div>
    </template>
    <template v-else-if="isEditNode && editSnapshot">
      <EditNode
        :node="editSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update:mode="emitNodeParam('mode', $event)"
        @generate="onEditGenerate"
      />
    </template>
    <template v-else-if="isEnhanceNode && enhanceSnapshot">
      <EnhanceNode
        :node="enhanceSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update:mode="emitNodeParam('mode', $event)"
        @update:upscale-mode="emitNodeParam('upscaleMode', $event)"
        @update:upscale-factor="emitNodeParam('upscaleFactor', $event)"
        @update:output-format="emitNodeParam('outputFormat', $event)"
        @update:output-quality="emitNodeParam('outputQuality', $event)"
        @update:extend-top="emitNodeParam('extendTop', $event)"
        @update:extend-right="emitNodeParam('extendRight', $event)"
        @update:extend-bottom="emitNodeParam('extendBottom', $event)"
        @update:extend-left="emitNodeParam('extendLeft', $event)"
        @generate="onEnhanceGenerate"
      />
    </template>
    <template v-else-if="isColorNode && colorSnapshot">
      <ColorNode
        :node="colorSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @apply-preset="onColorApplyPreset"
        @remove-color="onColorRemove"
        @add-color="onColorAdd"
        @strength-input="emitNodeParam('strength', $event)"
        @generate="onColorGenerate"
      />
    </template>
    <template v-else-if="isFusionNode && fusionSnapshot">
      <FusionNode
        :node="fusionSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update:blend-ratio="emitNodeParam('blendRatio', $event)"
        @generate="onFusionGenerate"
      />
    </template>
    <template v-else-if="isWearNode && wearSnapshot">
      <WearNode
        :node="wearSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update:tile-mode="emitNodeParam('tileMode', $event)"
        @update:rotation="emitNodeParam('rotation', $event)"
        @update:scale="emitNodeParam('scale', $event)"
        @update:opacity="emitNodeParam('opacity', $event)"
        @generate="onWearGenerate"
      />
    </template>
    <template v-else-if="isMaskNode && maskSnapshot">
      <MaskNode
        :node="maskSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :stage-height="maskStageHeight"
        :canvas-width="maskCanvasWidth"
        :source-image-url="props.maskSourceImageUrl"
        :has-connected-image-input="props.maskHasConnectedImageInput"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @open-file-picker="onMaskOpenFile"
        @clear-image="onMaskClearImage"
        @stage-drop="onMaskStageDrop"
        @canvas-mousedown="onMaskCanvasMouseDown"
        @set-draw-mode="emitNodeParam('drawMode', $event)"
        @update:brush-size="emitNodeParam('brushSize', $event)"
        @update:refine-amount="emitNodeParam('refineAmount', $event)"
        @toggle-invert="onMaskToggleInvert"
        @clear-mask="onMaskClearMask"
        @set-canvas-ref="onMaskSetCanvasRef"
      />
    </template>
    <template v-else-if="isModelNode && modelSnapshot">
      <ModelNode
        :node="modelSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        :background-options="modelBackgroundOptions"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update-angle-yaw="emitNodeParam('angleSpec.yaw', $event)"
        @update-angle-framing="emitNodeParam('angleSpec.framing', $event)"
        @update-model-strength="emitNodeParam('modelStrength', $event)"
        @update-pose-strength="emitNodeParam('poseStrength', $event)"
        @select-background="emitNodeParam('background', $event)"
        @generate="onModelGenerate"
      />
    </template>
    <template v-else-if="isViewpointNode && viewpointSnapshot">
      <ViewpointNode
        :node="viewpointSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState"
        :error-port-keys="[]"
        @node-mousedown="noopMouse"
        @update:title="emitNodeParam('title', $event)"
        @delete="onDeleteRequest"
        @update:category="emitNodeParam('activeCategory', $event)"
        @select:viewpoint="emitNodeParam('selectedViewpoint', $event)"
        @update:lighting="emitNodeParam('lighting', $event)"
        @generate="onViewpointGenerate"
      />
    </template>
    <template v-else>
      <div class="flow-canvas-node__unsupported">Unsupported node</div>
    </template>

    <div
      v-if="showHandleRail"
      class="flow-canvas-node__handle-rail"
      :style="handleRailStyle"
    >
      <FlowNodeHandles
        :kind="data.kind"
        :snapshot-node="data.snapshotNode"
        :ui-state="props.uiState"
        :show-handles="isPointerOverNode"
        @request-internals-update="emit('request-internals-update')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import { MODEL_BACKGROUND_OPTIONS } from '../components/nodes/config/nodeOptionConfig'
import type {
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  ViewpointNodeItem,
  WearNodeItem,
} from '../types/canvasNodes'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'

type ImageAction = 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'

const props = withDefaults(
  defineProps<{
    data: FlowNodeData
    selected?: boolean
    dragging?: boolean
    copyMode?: boolean
    uiState?: NodeUiState | PromptNodeUiState
    maskSourceImageUrl?: string
    maskSourceImageRatio?: number
    maskHasConnectedImageInput?: boolean
    imageActionVisible?: boolean
    imagePromptComposerVisible?: boolean
    imageDirectGenerating?: boolean
    showStyleStrengthControl?: boolean
    styleInnovateStrengthPct?: number
    setStyleStrengthTrackRef?: (el: Element | ComponentPublicInstance | null) => void
  }>(),
  {
    selected: false,
    dragging: false,
    copyMode: false,
    uiState: undefined,
    maskSourceImageUrl: '',
    maskSourceImageRatio: 1,
    maskHasConnectedImageInput: false,
    imageActionVisible: false,
    imagePromptComposerVisible: false,
    imageDirectGenerating: false,
    showStyleStrengthControl: false,
    styleInnovateStrengthPct: 50,
    setStyleStrengthTrackRef: () => undefined,
  },
)

const emit = defineEmits<{
  (e: 'update-prompt-text', value: string): void
  (e: 'update-node-param', payload: { key: string; value: unknown }): void
  (e: 'color-apply-preset', value: string[]): void
  (e: 'color-remove', index: number): void
  (e: 'color-add', value: string): void
  (e: 'mask-toggle-invert'): void
  (e: 'delete-node'): void
  (e: 'generate-edit'): void
  (e: 'generate-enhance'): void
  (e: 'generate-color'): void
  (e: 'generate-fusion'): void
  (e: 'generate-wear'): void
  (e: 'generate-model'): void
  (e: 'generate-viewpoint'): void
  (e: 'image-stage-drop', event: DragEvent): void
  (e: 'image-open-file'): void
  (e: 'image-append-to-canvas'): void
  (e: 'image-reset'): void
  (e: 'mask-open-file'): void
  (e: 'mask-clear-image'): void
  (e: 'mask-stage-drop', event: DragEvent): void
  (e: 'mask-canvas-mousedown', event: MouseEvent): void
  (e: 'mask-clear-mask'): void
  (e: 'mask-set-canvas-ref', element: Element | null): void
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'image-contextmenu', event: MouseEvent): void
  (e: 'image-prompt-generate', prompt: string): void
  (e: 'image-action', action: ImageAction): void
  (e: 'style-innovate-mousedown', event: MouseEvent): void
  (e: 'style-strength-track-mousedown', event: MouseEvent): void
  (e: 'request-internals-update'): void
}>()

const isPromptNode = computed(() => props.data.kind === 'prompt')
const isImageNode = computed(() => props.data.kind === 'image')
const isEditNode = computed(() => props.data.kind === 'edit')
const isEnhanceNode = computed(() => props.data.kind === 'enhance')
const isColorNode = computed(() => props.data.kind === 'color')
const isFusionNode = computed(() => props.data.kind === 'fusion')
const isWearNode = computed(() => props.data.kind === 'wear')
const isMaskNode = computed(() => props.data.kind === 'mask')
const isModelNode = computed(() => props.data.kind === 'model')
const isViewpointNode = computed(() => props.data.kind === 'viewpoint')
const showDefaultDragZone = false
const showHandleRail = true
const modelBackgroundOptions = MODEL_BACKGROUND_OPTIONS
const promptUiState = computed<PromptNodeUiState | undefined>(
  () => props.uiState as PromptNodeUiState | undefined,
)
const isPointerOverNode = ref(false)
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const handleRailBounds = reactive({
  left: 0,
  top: 0,
  width: 0,
  height: 0,
})
let handleRailMeasureFrame = 0
let handleRailResizeObserver: ResizeObserver | null = null
let observedHandleRailRoot: HTMLElement | null = null
let observedHandleRailFrame: HTMLElement | null = null

const handleRailStyle = computed(() => ({
  left: `${handleRailBounds.left}px`,
  top: `${handleRailBounds.top}px`,
  width: `${handleRailBounds.width}px`,
  height: `${handleRailBounds.height}px`,
}))

function resolveSnapshotNode<T>() {
  return props.data.snapshotNode && typeof props.data.snapshotNode === 'object'
    ? ({ ...(props.data.snapshotNode as T), x: 0, y: 0 } as T)
    : null
}

const promptSnapshot = computed<PromptNodeItem | null>(() => {
  if (!isPromptNode.value) return null
  return resolveSnapshotNode<PromptNodeItem>()
})

const imageSnapshot = computed<ImageNodeItem | null>(() => {
  if (!isImageNode.value) return null
  return resolveSnapshotNode<ImageNodeItem>()
})
const editSnapshot = computed<EditNodeItem | null>(() =>
  isEditNode.value ? resolveSnapshotNode<EditNodeItem>() : null,
)
const enhanceSnapshot = computed<EnhanceNodeItem | null>(() =>
  isEnhanceNode.value ? resolveSnapshotNode<EnhanceNodeItem>() : null,
)
const colorSnapshot = computed<ColorNodeItem | null>(() =>
  isColorNode.value ? resolveSnapshotNode<ColorNodeItem>() : null,
)
const fusionSnapshot = computed<FusionNodeItem | null>(() =>
  isFusionNode.value ? resolveSnapshotNode<FusionNodeItem>() : null,
)
const wearSnapshot = computed<WearNodeItem | null>(() =>
  isWearNode.value ? resolveSnapshotNode<WearNodeItem>() : null,
)
const maskSnapshot = computed<MaskNodeItem | null>(() =>
  isMaskNode.value ? resolveSnapshotNode<MaskNodeItem>() : null,
)
const modelSnapshot = computed<ModelNodeItem | null>(() =>
  isModelNode.value ? resolveSnapshotNode<ModelNodeItem>() : null,
)
const viewpointSnapshot = computed<ViewpointNodeItem | null>(() =>
  isViewpointNode.value ? resolveSnapshotNode<ViewpointNodeItem>() : null,
)

const displayImage = computed<{ url: string } | null>(() => {
  if (!imageSnapshot.value) return null
  const outputs = Array.isArray(imageSnapshot.value.outputImages)
    ? imageSnapshot.value.outputImages
    : []
  const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
  const url = latestOutput?.url || imageSnapshot.value.sourceImageUrl || ''
  return url ? { url } : null
})

const maskCanvasWidth = computed(() => {
  if (!maskSnapshot.value) return 280
  return Math.max(120, Math.round(maskSnapshot.value.width - 24))
})

const maskStageHeight = computed(() => {
  if (!maskSnapshot.value) return 180
  const ratio =
    props.maskSourceImageRatio > 0
      ? props.maskSourceImageRatio
      : maskSnapshot.value.sourceImageRatio > 0
        ? maskSnapshot.value.sourceImageRatio
        : 1
  return Math.max(120, Math.min(360, Math.round(maskCanvasWidth.value * ratio)))
})

const displayMode = computed<'source' | 'node-output'>(() => {
  if (!imageSnapshot.value) return 'source'
  return (imageSnapshot.value.outputImages?.length ?? 0) > 0 ? 'node-output' : 'source'
})

const stageHeight = computed(() => {
  if (!imageSnapshot.value) return 180
  const stageWidth = Math.max(120, Math.round(imageSnapshot.value.width))
  const outputs = Array.isArray(imageSnapshot.value.outputImages)
    ? imageSnapshot.value.outputImages
    : []
  const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
  const outputRatio = Number(latestOutput?.ratio)
  const sourceRatio = Number(imageSnapshot.value.sourceImageRatio)
  const ratio =
    displayImage.value && Number.isFinite(outputRatio) && outputRatio > 0
      ? outputRatio
      : displayImage.value && Number.isFinite(sourceRatio) && sourceRatio > 0
        ? sourceRatio
        : 1
  return Math.max(120, Math.min(360, Math.round(stageWidth * ratio)))
})

function getNodeHandleFrameElement(): HTMLElement | null {
  const root = flowCanvasNodeRef.value
  const explicitFrame = root?.querySelector<HTMLElement>('[data-flow-handle-frame]')
  if (explicitFrame) return explicitFrame

  const workflowNode = root?.querySelector<HTMLElement>('.workflow-node')
  if (!workflowNode) return null

  for (const child of Array.from(workflowNode.children)) {
    if (!(child instanceof HTMLElement)) continue
    if (child.classList.contains('workflow-node-header')) continue
    return child
  }
  return null
}

function getLayoutOffsetWithinRoot(element: HTMLElement, root: HTMLElement) {
  let left = 0
  let top = 0
  let current: HTMLElement | null = element

  while (current && current !== root) {
    left += current.offsetLeft
    top += current.offsetTop
    current = current.offsetParent as HTMLElement | null
  }

  return current === root ? { left, top } : null
}

function refreshHandleRailObserver() {
  if (typeof ResizeObserver === 'undefined') return

  if (!handleRailResizeObserver) {
    handleRailResizeObserver = new ResizeObserver(() => requestHandleRailMeasure())
  }

  const root = flowCanvasNodeRef.value
  const frame = getNodeHandleFrameElement()

  if (observedHandleRailRoot !== root) {
    if (observedHandleRailRoot) handleRailResizeObserver.unobserve(observedHandleRailRoot)
    observedHandleRailRoot = root
    if (observedHandleRailRoot) handleRailResizeObserver.observe(observedHandleRailRoot)
  }

  if (observedHandleRailFrame !== frame) {
    if (observedHandleRailFrame) handleRailResizeObserver.unobserve(observedHandleRailFrame)
    observedHandleRailFrame = frame
    if (observedHandleRailFrame) handleRailResizeObserver.observe(observedHandleRailFrame)
  }
}

function setHandleRailBounds(next: {
  left: number
  top: number
  width: number
  height: number
}) {
  const changed =
    handleRailBounds.left !== next.left ||
    handleRailBounds.top !== next.top ||
    handleRailBounds.width !== next.width ||
    handleRailBounds.height !== next.height

  if (!changed) return

  handleRailBounds.left = next.left
  handleRailBounds.top = next.top
  handleRailBounds.width = next.width
  handleRailBounds.height = next.height
  void nextTick(() => emit('request-internals-update'))
}

function updateHandleRailBounds() {
  handleRailMeasureFrame = 0
  const root = flowCanvasNodeRef.value
  const frame = getNodeHandleFrameElement()

  if (!root) {
    return
  }

  if (!frame) {
    setHandleRailBounds({
      left: 0,
      top: 0,
      width: Math.max(1, root.offsetWidth),
      height: Math.max(1, root.offsetHeight),
    })
    return
  }

  const offset = getLayoutOffsetWithinRoot(frame, root)
  if (!offset) return

  setHandleRailBounds({
    left: Math.round(offset.left),
    top: Math.round(offset.top),
    width: Math.max(1, Math.round(frame.offsetWidth)),
    height: Math.max(1, Math.round(frame.offsetHeight)),
  })
}

function requestHandleRailMeasure() {
  if (typeof requestAnimationFrame === 'undefined') {
    refreshHandleRailObserver()
    updateHandleRailBounds()
    return
  }

  if (handleRailMeasureFrame) cancelAnimationFrame(handleRailMeasureFrame)
  handleRailMeasureFrame = requestAnimationFrame(() => {
    refreshHandleRailObserver()
    updateHandleRailBounds()
  })
}

onMounted(() => {
  void nextTick(requestHandleRailMeasure)
})

onUpdated(() => {
  requestHandleRailMeasure()
})

onBeforeUnmount(() => {
  if (handleRailMeasureFrame) cancelAnimationFrame(handleRailMeasureFrame)
  handleRailResizeObserver?.disconnect()
})

function onPromptTextUpdate(value: string) {
  emit('update-prompt-text', value)
}

function emitNodeParam(key: string, value: unknown) {
  emit('update-node-param', { key, value })
}

function onColorApplyPreset(value: string[]) {
  emit('color-apply-preset', value)
}

function onColorRemove(index: number) {
  emit('color-remove', index)
}

function onColorAdd(value: string) {
  emit('color-add', value)
}

function onMaskToggleInvert() {
  emit('mask-toggle-invert')
}

function onDeleteRequest() {
  emit('delete-node')
}

function onEnhanceGenerate() {
  emit('generate-enhance')
}

function onEditGenerate() {
  emit('generate-edit')
}

function onColorGenerate() {
  emit('generate-color')
}

function onFusionGenerate() {
  emit('generate-fusion')
}

function onWearGenerate() {
  emit('generate-wear')
}

function onModelGenerate() {
  emit('generate-model')
}

function onViewpointGenerate() {
  emit('generate-viewpoint')
}

function onImageStageDrop(event: DragEvent) {
  emit('image-stage-drop', event)
}

function onImageOpenFile() {
  emit('image-open-file')
}

function onImageAppendToCanvas() {
  emit('image-append-to-canvas')
}

function onImageReset() {
  emit('image-reset')
}

function onMaskOpenFile() {
  emit('mask-open-file')
}

function onMaskClearImage() {
  emit('mask-clear-image')
}

function onMaskStageDrop(event: DragEvent) {
  emit('mask-stage-drop', event)
}

function onMaskCanvasMouseDown(event: MouseEvent) {
  emit('mask-canvas-mousedown', event)
}

function onMaskClearMask() {
  emit('mask-clear-mask')
}

function onMaskSetCanvasRef(element: Element | null) {
  emit('mask-set-canvas-ref', element)
}

function onImageContextMenu(event: MouseEvent) {
  emit('image-contextmenu', event)
}

function onImagePromptGenerate(prompt: string) {
  emit('image-prompt-generate', prompt)
}

function onImageAction(action: ImageAction) {
  emit('image-action', action)
}
function onStyleInnovateMouseDown(event: MouseEvent) {
  emit('style-innovate-mousedown', event)
}

function onStyleStrengthTrackMouseDown(event: MouseEvent) {
  emit('style-strength-track-mousedown', event)
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

.flow-canvas-node__image-wrap {
  position: relative;
  overflow: visible;
}

.flow-canvas-node__drag-surface {
  cursor: grab;
}

.flow-canvas-node__drag-surface:active {
  cursor: grabbing;
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

.flow-canvas-node :deep(.workflow-node) {
  position: relative;
  left: 0 !important;
  top: 0 !important;
  transform: none !important;
  z-index: 1;
  overflow: visible;
  pointer-events: auto;
}

.flow-canvas-node :deep(.workflow-node:not(.prompt-node)) {
  border: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.flow-canvas-node
  :deep(
    .workflow-node:not(.prompt-node):not(.image-node) > :not(.workflow-node-header):first-of-type
  ) {
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  background: #ffffff;
  transition: border-color 0.2s ease;
}

.flow-canvas-node
  :deep(
    .workflow-node:not(.prompt-node):not(.image-node).active
      > :not(.workflow-node-header):first-of-type
  ) {
  border-color: #9278e6;
}

.flow-canvas-node :deep(.workflow-node.dragging) {
  transition: none;
}

.flow-canvas-node :deep(.image-action-menu) {
  top: calc(100% + 10px);
  left: 0;
  transform: none;
  width: 100%;
  z-index: 30;
}

.flow-canvas-node :deep(.image-action-menu.strength-mode) {
  top: calc(100% + 10px);
  left: 0;
  transform: none;
  width: 100%;
}
</style>
