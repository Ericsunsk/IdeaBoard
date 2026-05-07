<template>
  <div ref="flowCanvasNodeRef" class="flow-canvas-node" :class="[`is-${data.kind}`, { 'is-selected': selected }]">
    <div v-if="showDefaultDragZone" class="flow-canvas-node__drag-zone flow-canvas-node__drag-zone--default" />
    <template v-if="isPromptNode && promptSnapshot">
      <PromptNode
        :node="promptSnapshot"
        :active="selected"
        :dragging="props.dragging"
        :copy-mode="props.copyMode"
        :ui-state="props.uiState as PromptNodeUiState | undefined"
        @update:text="onPromptTextUpdate"
        @node-mousedown="noopMouse"
        @delete="onDeleteRequest"
        @focus="noop"
        @port-mousedown="noopPortMouse"
      />
    </template>
    <template v-else-if="isImageNode && imageSnapshot">
      <div class="flow-canvas-node__image-wrap flow-canvas-node__drag-surface" @click.stop @contextmenu.stop>
        <ImageNode
          :node="imageSnapshot"
          :active="selected"
          :dragging="props.dragging"
          :copy-mode="props.copyMode"
          :ui-state="props.uiState"
          :stage-height="stageHeight"
          :display-image="displayImage"
          :display-mode="displayMode"
          @node-mousedown="noopMouse"
          @delete="onDeleteRequest"
          @stage-drop="onImageStageDrop"
          @open-file="onImageOpenFile"
          @append-to-canvas="onImageAppendToCanvas"
          @reset="onImageReset"
          @display-dragstart="onImageDisplayDragStart"
          @port-mousedown="noopPortMouse"
        />
      </div>
    </template>
    <template v-else-if="isMessageNode && messageSnapshot">
      <div
        class="flow-canvas-node__message"
        :class="{ 'is-pending': isMessagePending }"
        :style="{ width: `${messageDisplayWidth}px` }"
        @mousedown="onMessageMouseDown"
        @contextmenu.stop.prevent="onMessageContextMenu"
      >
        <div class="flow-canvas-node__drag-zone flow-canvas-node__drag-zone--message" />
        <img
          :src="messageSnapshot.url"
          class="flow-canvas-node__message-image flow-canvas-node__drag-surface"
          :ref="setMessageImageRef"
          draggable="false"
          @load="onMessageImageLoad"
          @dragstart="onMessageImageDragStart"
        />
        <button
          v-for="direction in MESSAGE_RESIZE_DIRECTIONS"
          v-if="selected && !isMessagePending"
          :key="`message-resize-${direction}`"
          class="flow-canvas-node__message-resize-handle"
          :class="`is-${direction}`"
          type="button"
          :aria-label="`Resize message image ${direction}`"
          @mousedown.stop.prevent="onMessageResizeMouseDown($event, direction)"
        />
        <div v-if="isMessagePending" class="flow-canvas-node__message-pending" aria-hidden="true">
          处理中
        </div>
        <HomeCanvasImageActionMenu
          v-if="showMessageActionMenu"
          :show-style-strength-control="props.showStyleStrengthControl"
          :style-strength-menu-style="props.showStyleStrengthControl ? messageStyleStrengthMenuStyle : undefined"
          :direct-generating="messageDirectGenerating"
          :display-height="messageDisplayHeight"
          :style-innovate-strength-pct="props.styleInnovateStrengthPct"
          :set-style-strength-track-ref="props.setStyleStrengthTrackRef"
          @action-remove-bg="onMessageAction('remove-bg')"
          @style-innovate-mousedown="onStyleInnovateMouseDown"
          @style-innovate-click="onMessageAction('style-innovate')"
          @action-to-vector="onMessageAction('to-vector')"
          @action-sketch-to-design="onMessageAction('sketch-to-design')"
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
        @delete="onDeleteRequest"
        @update:mode="emitNodeParam('mode', $event)"
        @generate="onEditGenerate"
        @port-mousedown="noopPortMouse"
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
        @port-mousedown="noopPortMouse"
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
        @delete="onDeleteRequest"
        @apply-preset="onColorApplyPreset"
        @remove-color="onColorRemove"
        @add-color="onColorAdd"
        @strength-input="emitNodeParam('strength', $event)"
        @generate="onColorGenerate"
        @port-mousedown="noopPortMouse"
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
        @delete="onDeleteRequest"
        @update:blend-ratio="emitNodeParam('blendRatio', $event)"
        @generate="onFusionGenerate"
        @port-mousedown="noopPortMouse"
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
        @delete="onDeleteRequest"
        @update:tile-mode="emitNodeParam('tileMode', $event)"
        @update:rotation="emitNodeParam('rotation', $event)"
        @update:scale="emitNodeParam('scale', $event)"
        @update:opacity="emitNodeParam('opacity', $event)"
        @generate="onWearGenerate"
        @port-mousedown="noopPortMouse"
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
        @port-mousedown="noopPortMouse"
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
        @delete="onDeleteRequest"
        @update-angle-yaw="emitNodeParam('angleSpec.yaw', $event)"
        @update-angle-framing="emitNodeParam('angleSpec.framing', $event)"
        @update-model-strength="emitNodeParam('modelStrength', $event)"
        @update-pose-strength="emitNodeParam('poseStrength', $event)"
        @select-background="emitNodeParam('background', $event)"
        @generate="onModelGenerate"
        @port-mousedown="noopPortMouse"
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
        @delete="onDeleteRequest"
        @update:category="emitNodeParam('activeCategory', $event)"
        @select:viewpoint="emitNodeParam('selectedViewpoint', $event)"
        @update:lighting="emitNodeParam('lighting', $event)"
        @generate="onViewpointGenerate"
        @port-mousedown="noopPortMouse"
      />
    </template>
    <template v-else>
      <div class="flow-canvas-node__unsupported">Unsupported node</div>
    </template>

    <Handle
      v-for="port in handleInputPorts"
      :id="`in:${port}`"
      :key="`canvas-in:${port}`"
      type="target"
      :position="Position.Left"
      class="flow-canvas-node__handle flow-canvas-node__handle--input"
      :style="resolveHandleStyle(port, 'input')"
    />
    <Handle
      v-for="port in handleOutputPorts"
      :id="`out:${port}`"
      :key="`canvas-out:${port}`"
      type="source"
      :position="Position.Right"
      class="flow-canvas-node__handle flow-canvas-node__handle--output"
      :style="resolveHandleStyle(port, 'output')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch, type ComponentPublicInstance } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { getCanvasNodeVisibleInputPorts, getCanvasNodeVisibleOutputPorts } from '../config/canvasNodeConnectionContract'
import {
  buildEditNodeIoContext,
  buildNodeIoFromSlots,
  COLOR_NODE_IO_SLOTS,
  EDIT_NODE_IO_SLOTS,
  ENHANCE_NODE_IO_SLOTS,
  FUSION_NODE_IO_SLOTS,
  IMAGE_NODE_IO_SLOTS,
  MASK_NODE_IO_SLOTS,
  MODEL_NODE_IO_SLOTS,
  PROMPT_NODE_IO_SLOTS,
  VIEWPOINT_NODE_IO_SLOTS,
  WEAR_NODE_IO_SLOTS,
} from '../components/nodes/nodeIoConfig'
import {
  MODEL_BACKGROUND_OPTIONS,
} from '../components/nodes/nodeOptionConfig'
import type {
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasImageItem,
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
import ColorNode from '../components/nodes/ColorNode.vue'
import EditNode from '../components/nodes/EditNode.vue'
import EnhanceNode from '../components/nodes/EnhanceNode.vue'
import FusionNode from '../components/nodes/FusionNode.vue'
import ImageNode from '../components/nodes/ImageNode.vue'
import MaskNode from '../components/nodes/MaskNode.vue'
import ModelNode from '../components/nodes/ModelNode.vue'
import PromptNode from '../components/nodes/PromptNode.vue'
import ViewpointNode from '../components/nodes/ViewpointNode.vue'
import WearNode from '../components/nodes/WearNode.vue'
import HomeCanvasImageActionMenu from '../components/home/HomeCanvasImageActionMenu.vue'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'
import type { FlowNodeData } from '../types/flowTypes'
type ResizeDirection = 'n' | 's' | 'e' | 'w' | 'nw' | 'ne' | 'sw' | 'se'

const props = withDefaults(defineProps<{
  data: FlowNodeData
  selected?: boolean
  dragging?: boolean
  copyMode?: boolean
  uiState?: NodeUiState | PromptNodeUiState
  maskSourceImageUrl?: string
  maskSourceImageRatio?: number
  maskHasConnectedImageInput?: boolean
  messageActionVisible?: boolean
  messageDirectGenerating?: boolean
  showStyleStrengthControl?: boolean
  styleInnovateStrengthPct?: number
  setStyleStrengthTrackRef?: (el: Element | ComponentPublicInstance | null) => void
}>(), {
  selected: false,
  dragging: false,
  copyMode: false,
  maskSourceImageUrl: '',
  maskSourceImageRatio: 1,
  maskHasConnectedImageInput: false,
  messageActionVisible: false,
  messageDirectGenerating: false,
  showStyleStrengthControl: false,
  styleInnovateStrengthPct: 50,
  setStyleStrengthTrackRef: () => undefined,
})

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
  (e: 'image-display-dragstart', event: DragEvent): void
  (e: 'image-reset'): void
  (e: 'mask-open-file'): void
  (e: 'mask-clear-image'): void
  (e: 'mask-stage-drop', event: DragEvent): void
  (e: 'mask-canvas-mousedown', event: MouseEvent): void
  (e: 'mask-clear-mask'): void
  (e: 'mask-set-canvas-ref', element: Element | null): void
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'message-contextmenu', event: MouseEvent): void
  (e: 'message-image-dragstart', event: DragEvent): void
  (e: 'message-resize-mousedown', payload: { event: MouseEvent; direction: ResizeDirection }): void
  (e: 'message-action', action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'): void
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
const isMessageNode = computed(() => props.data.kind === 'message')
const showDefaultDragZone = computed(() => !isMessageNode.value)
const modelBackgroundOptions = MODEL_BACKGROUND_OPTIONS
const MESSAGE_RESIZE_DIRECTIONS: ResizeDirection[] = ['n', 's', 'e', 'w', 'nw', 'ne', 'sw', 'se']

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
const editSnapshot = computed<EditNodeItem | null>(() => (isEditNode.value ? resolveSnapshotNode<EditNodeItem>() : null))
const enhanceSnapshot = computed<EnhanceNodeItem | null>(() => (isEnhanceNode.value ? resolveSnapshotNode<EnhanceNodeItem>() : null))
const colorSnapshot = computed<ColorNodeItem | null>(() => (isColorNode.value ? resolveSnapshotNode<ColorNodeItem>() : null))
const fusionSnapshot = computed<FusionNodeItem | null>(() => (isFusionNode.value ? resolveSnapshotNode<FusionNodeItem>() : null))
const wearSnapshot = computed<WearNodeItem | null>(() => (isWearNode.value ? resolveSnapshotNode<WearNodeItem>() : null))
const maskSnapshot = computed<MaskNodeItem | null>(() => (isMaskNode.value ? resolveSnapshotNode<MaskNodeItem>() : null))
const modelSnapshot = computed<ModelNodeItem | null>(() => (isModelNode.value ? resolveSnapshotNode<ModelNodeItem>() : null))
const viewpointSnapshot = computed<ViewpointNodeItem | null>(() => (isViewpointNode.value ? resolveSnapshotNode<ViewpointNodeItem>() : null))
const messageSnapshot = computed<CanvasImageItem | null>(() => (isMessageNode.value ? resolveSnapshotNode<CanvasImageItem>() : null))
const STYLE_STRENGTH_MENU_GAP = 16
const messageImageElement = ref<HTMLImageElement | null>(null)
const messageImageLayoutVersion = ref(0)
const messageDisplayWidth = computed(() => {
  const width = Number(messageSnapshot.value?.width)
  return Number.isFinite(width) && width > 0 ? width : 300
})
const messageStyleStrengthMenuStyle = computed(() => {
  messageImageLayoutVersion.value
  const imageEl = messageImageElement.value
  if (!imageEl) {
    return { top: '8px', left: 'calc(100% + 16px)' }
  }
  return {
    top: `${Math.max(0, imageEl.offsetTop)}px`,
    left: `${Math.max(0, imageEl.offsetLeft + imageEl.clientWidth + STYLE_STRENGTH_MENU_GAP)}px`,
  }
})
const messageDisplayHeight = computed(() => {
  const width = messageDisplayWidth.value
  const directRatio = Number(messageSnapshot.value?.height_ratio)
  if (Number.isFinite(directRatio) && directRatio > 0) {
    return Math.max(72, Math.round(width * directRatio))
  }

  const snapshot = messageSnapshot.value as (CanvasImageItem & { height?: unknown; width?: unknown }) | null
  const sourceHeight = Number(snapshot?.height)
  const sourceWidth = Number(snapshot?.width)
  const fallbackRatio = Number.isFinite(sourceHeight) && sourceHeight > 0 && Number.isFinite(sourceWidth) && sourceWidth > 0
    ? sourceHeight / sourceWidth
    : 1
  return Math.max(72, Math.round(width * fallbackRatio))
})
const isMessagePending = computed(() => messageSnapshot.value?.renderState === 'pending')
const showMessageActionMenu = computed(() => isMessageNode.value && !!props.messageActionVisible && !!messageSnapshot.value)

const displayImage = computed<{ url: string } | null>(() => {
  if (!imageSnapshot.value) return null
  const outputs = Array.isArray(imageSnapshot.value.outputImages) ? imageSnapshot.value.outputImages : []
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
  const ratio = props.maskSourceImageRatio > 0
    ? props.maskSourceImageRatio
    : (maskSnapshot.value.sourceImageRatio > 0 ? maskSnapshot.value.sourceImageRatio : 1)
  return Math.max(120, Math.min(360, Math.round(maskCanvasWidth.value * ratio)))
})

const displayMode = computed<'source' | 'node-output'>(() => {
  if (!imageSnapshot.value) return 'source'
  return (imageSnapshot.value.outputImages?.length ?? 0) > 0 ? 'node-output' : 'source'
})

const stageHeight = computed(() => {
  if (!imageSnapshot.value) return 180
  const stageWidth = Math.max(120, Math.round(imageSnapshot.value.width - 24))
  const ratio = imageSnapshot.value.sourceImageRatio > 0 ? imageSnapshot.value.sourceImageRatio : 1
  return Math.max(120, Math.min(360, Math.round(stageWidth * ratio)))
})

const handleInputPorts = computed(() => getCanvasNodeVisiblePortsForKind('input'))
const handleOutputPorts = computed(() => getCanvasNodeVisiblePortsForKind('output'))
const flowCanvasNodeRef = ref<HTMLElement | null>(null)
const measuredPortTopMap = ref<Record<string, number>>({})
let nodeResizeObserver: ResizeObserver | null = null
let portResizeObserver: ResizeObserver | null = null
let nodeMutationObserver: MutationObserver | null = null
let measureQueued = false

function schedulePortMeasurement() {
  if (measureQueued) return
  measureQueued = true
  void nextTick(() => {
    measureQueued = false
    measurePortTopsFromLegacyDom()
    reconnectPortResizeObserver()
    emit('request-internals-update')
  })
}

function measurePortTopsFromLegacyDom() {
  const root = flowCanvasNodeRef.value
  if (!root) {
    measuredPortTopMap.value = {}
    return
  }
  const rootRect = root.getBoundingClientRect()
  const portElements = root.querySelectorAll<HTMLElement>('.node-port[data-port-direction][data-port-name]')
  const nextMap: Record<string, number> = {}
  for (const portElement of portElements) {
    const direction = portElement.dataset.portDirection
    const portName = portElement.dataset.portName as CanvasConnectionPort | undefined
    if (!portName) continue
    if (direction !== 'input' && direction !== 'output') continue
    const rect = portElement.getBoundingClientRect()
    const centerY = rect.top + rect.height / 2 - rootRect.top
    if (!Number.isFinite(centerY)) continue
    nextMap[getMeasuredPortKey(portName, direction)] = centerY
  }
  measuredPortTopMap.value = nextMap
}

function getMeasuredPortKey(port: CanvasConnectionPort, direction: 'input' | 'output'): string {
  return `${direction}:${port}`
}

function reconnectPortResizeObserver() {
  portResizeObserver?.disconnect()
  const root = flowCanvasNodeRef.value
  if (!root || typeof ResizeObserver === 'undefined') return
  portResizeObserver = new ResizeObserver(() => {
    schedulePortMeasurement()
  })
  const portElements = root.querySelectorAll<HTMLElement>('.node-port[data-port-direction][data-port-name]')
  for (const portElement of portElements) {
    portResizeObserver.observe(portElement)
  }
}

function setupObservers() {
  const root = flowCanvasNodeRef.value
  nodeResizeObserver?.disconnect()
  nodeMutationObserver?.disconnect()
  if (!root) return
  if (typeof ResizeObserver !== 'undefined') {
    nodeResizeObserver = new ResizeObserver(() => {
      schedulePortMeasurement()
    })
    nodeResizeObserver.observe(root)
  }
  if (typeof MutationObserver !== 'undefined') {
    nodeMutationObserver = new MutationObserver(() => {
      schedulePortMeasurement()
    })
    nodeMutationObserver.observe(root, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
      attributeFilter: ['style', 'class', 'data-port-direction', 'data-port-name'],
    })
  }
}

watch(
  flowCanvasNodeRef,
  () => {
    setupObservers()
    schedulePortMeasurement()
  },
  { flush: 'post' },
)

watch(
  () => [
    props.data.kind,
    props.data.snapshotNode,
    handleInputPorts.value.join('|'),
    handleOutputPorts.value.join('|'),
  ],
  () => {
    schedulePortMeasurement()
  },
  { deep: true, flush: 'post' },
)

onBeforeUnmount(() => {
  nodeResizeObserver?.disconnect()
  nodeResizeObserver = null
  portResizeObserver?.disconnect()
  portResizeObserver = null
  nodeMutationObserver?.disconnect()
  nodeMutationObserver = null
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

function setMessageImageRef(element: Element | ComponentPublicInstance | null) {
  messageImageElement.value = element instanceof HTMLImageElement ? element : null
  messageImageLayoutVersion.value += 1
}

function onMessageImageLoad() {
  messageImageLayoutVersion.value += 1
  emit('request-internals-update')
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

function onImageDisplayDragStart(event: DragEvent) {
  emit('image-display-dragstart', event)
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

function onMessageMouseDown(event: MouseEvent) {
  emit('node-mousedown', event)
}

function onMessageContextMenu(event: MouseEvent) {
  emit('message-contextmenu', event)
}

function onMessageImageDragStart(event: DragEvent) {
  if (!event.altKey) {
    event.preventDefault()
    return
  }
  emit('message-image-dragstart', event)
}

function onMessageResizeMouseDown(event: MouseEvent, direction: ResizeDirection) {
  emit('message-resize-mousedown', { event, direction })
}

function onMessageAction(action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design') {
  emit('message-action', action)
}
function onStyleInnovateMouseDown(event: MouseEvent) {
  emit('style-innovate-mousedown', event)
}

function onStyleStrengthTrackMouseDown(event: MouseEvent) {
  emit('style-strength-track-mousedown', event)
}

function getCanvasNodeType(): CanvasConnectionNodeType | null {
  const kind = props.data.kind
  return kind === 'message' ? null : kind
}

function getNodePortsByKind(kind: CanvasConnectionNodeType, direction: 'input' | 'output'): CanvasConnectionPort[] {
  const node = props.data.snapshotNode && typeof props.data.snapshotNode === 'object'
    ? props.data.snapshotNode as Record<string, unknown>
    : { id: 0 }
  const list = direction === 'input'
    ? getCanvasNodeVisibleInputPorts(kind, node as never)
    : getCanvasNodeVisibleOutputPorts(kind, node as never)
  return list.map(port => port.port)
}

function getNodeIoTopMap(kind: CanvasConnectionNodeType): Partial<Record<CanvasConnectionPort, number>> {
  if (!props.data.snapshotNode || typeof props.data.snapshotNode !== 'object') return {}
  const snapshotNode = props.data.snapshotNode as Record<string, unknown>
  const state = {}
  if (kind === 'prompt') return portTopMapFromSlots(PROMPT_NODE_IO_SLOTS, null, state)
  if (kind === 'image') return portTopMapFromSlots(IMAGE_NODE_IO_SLOTS, null, state)
  if (kind === 'mask') return portTopMapFromSlots(MASK_NODE_IO_SLOTS, null, state)
  if (kind === 'color') return portTopMapFromSlots(COLOR_NODE_IO_SLOTS, null, state)
  if (kind === 'model') return portTopMapFromSlots(MODEL_NODE_IO_SLOTS, null, state)
  if (kind === 'viewpoint') return portTopMapFromSlots(VIEWPOINT_NODE_IO_SLOTS, null, state)
  if (kind === 'fusion') return portTopMapFromSlots(FUSION_NODE_IO_SLOTS, null, state)
  if (kind === 'wear') return portTopMapFromSlots(WEAR_NODE_IO_SLOTS, null, state)
  if (kind === 'edit') return portTopMapFromSlots(EDIT_NODE_IO_SLOTS, buildEditNodeIoContext(snapshotNode as unknown as EditNodeItem), state)
  if (kind === 'enhance') return portTopMapFromSlots(ENHANCE_NODE_IO_SLOTS, {
    supportsPrompt: (snapshotNode as unknown as EnhanceNodeItem).mode === 'canvas-extend',
    supportsMask: (snapshotNode as unknown as EnhanceNodeItem).mode === 'hd-upscale',
  }, state)
  return {}
}

function portTopMapFromSlots<T>(
  slots: readonly unknown[],
  context: T,
  state: { connected?: Record<string, boolean>; pending?: Record<string, boolean> },
): Partial<Record<CanvasConnectionPort, number>> {
  const built = buildNodeIoFromSlots(slots as never, context, state)
  const map: Partial<Record<CanvasConnectionPort, number>> = {}
  for (const port of built.ports) {
    if (port.visible === false) continue
    if (port.top == null) continue
    map[port.name as CanvasConnectionPort] = port.top
  }
  return map
}

function getCanvasNodeVisiblePortsForKind(direction: 'input' | 'output'): CanvasConnectionPort[] {
  const kind = getCanvasNodeType()
  if (!kind) return []
  return getNodePortsByKind(kind, direction)
}

function resolveHandleStyle(port: CanvasConnectionPort, direction: 'input' | 'output') {
  const kind = getCanvasNodeType()
  if (!kind) return undefined
  const topMap = getNodeIoTopMap(kind)
  const measuredTop = measuredPortTopMap.value[getMeasuredPortKey(port, direction)]
  const top = typeof measuredTop === 'number' && Number.isFinite(measuredTop) ? measuredTop : topMap[port]
  // Keep handle center aligned to legacy .node-port center after expanding hit area to 18px.
  const side = direction === 'input' ? { left: '-10px' } : { right: '-10px' }
  if (typeof top === 'number' && Number.isFinite(top)) {
    return {
      top: `${top}px`,
      transform: 'translateY(-50%)',
      ...side,
    }
  }
  return {
    ...side,
  }
}

function noop() {}
function noopMouse() {}
function noopPortMouse() {}
</script>

<style scoped>
.flow-canvas-node {
  position: relative;
  isolation: isolate;
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

.flow-canvas-node__drag-zone--message {
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 1;
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

.flow-canvas-node__message {
  position: relative;
  background: #fff;
  border: 1.5px solid #e2e4e8;
  border-radius: 12px;
  padding: 4px;
  box-sizing: border-box;
  overflow: visible;
}

.flow-canvas-node.is-selected .flow-canvas-node__message {
  border-color: #9278e6;
}

.flow-canvas-node__message-image {
  display: block;
  width: 100%;
  height: auto;
  object-fit: contain;
  border-radius: 8px;
  aspect-ratio: auto;
  user-select: none;
  -webkit-user-select: none;
  -webkit-user-drag: none;
}

.flow-canvas-node__message-pending {
  position: absolute;
  inset: 4px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.12);
  color: #334155;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  pointer-events: none;
  user-select: none;
  z-index: 3;
}

.flow-canvas-node__message-resize-handle {
  position: absolute;
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 999px;
  background: transparent;
  z-index: 14;
}

.flow-canvas-node__message-resize-handle.is-n {
  top: -7px;
  left: 50%;
  transform: translateX(-50%);
  cursor: ns-resize;
}

.flow-canvas-node__message-resize-handle.is-s {
  bottom: -7px;
  left: 50%;
  transform: translateX(-50%);
  cursor: ns-resize;
}

.flow-canvas-node__message-resize-handle.is-e {
  right: -7px;
  top: 50%;
  transform: translateY(-50%);
  cursor: ew-resize;
}

.flow-canvas-node__message-resize-handle.is-w {
  left: -7px;
  top: 50%;
  transform: translateY(-50%);
  cursor: ew-resize;
}

.flow-canvas-node__message-resize-handle.is-nw {
  left: -7px;
  top: -7px;
  cursor: nwse-resize;
}

.flow-canvas-node__message-resize-handle.is-ne {
  right: -7px;
  top: -7px;
  cursor: nesw-resize;
}

.flow-canvas-node__message-resize-handle.is-sw {
  left: -7px;
  bottom: -7px;
  cursor: nesw-resize;
}

.flow-canvas-node__message-resize-handle.is-se {
  right: -7px;
  bottom: -7px;
  cursor: nwse-resize;
}

.flow-canvas-node__unsupported {
  padding: 8px 10px;
  border: 1px dashed #d0d7de;
  border-radius: 8px;
  background: #fff;
  color: #6b7280;
  font-size: 12px;
}

.flow-canvas-node :deep(.workflow-node) {
  position: relative;
  left: 0 !important;
  top: 0 !important;
  transform: none !important;
  z-index: 1;
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  background: #ffffff;
  overflow: visible;
  transition: border-color 0.2s ease;
  pointer-events: auto;
}

.flow-canvas-node :deep(.workflow-node .workflow-node-header) {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}

.flow-canvas-node :deep(.workflow-node.active) {
  border-color: #9278e6;
}

.flow-canvas-node :deep(.workflow-node.dragging) {
  transition: none;
}

.flow-canvas-node :deep(.image-action-menu) {
  left: calc(100% + 12px);
  z-index: 30;
}

.flow-canvas-node__handle {
  width: 18px;
  height: 18px;
  border: 0;
  opacity: 0;
  z-index: 24;
  transform: translateY(-50%);
  box-shadow: none;
  background: transparent !important;
  cursor: crosshair;
  pointer-events: all;
}

.flow-canvas-node :deep(.flow-canvas-node__handle.vue-flow__handle-left),
.flow-canvas-node :deep(.flow-canvas-node__handle.vue-flow__handle-right) {
  transform: translateY(-50%) !important;
}

.flow-canvas-node :deep(.node-port) {
  pointer-events: none !important;
  z-index: 21;
}

.flow-canvas-node.is-message :deep(.vue-flow__handle) {
  display: none;
}
</style>
