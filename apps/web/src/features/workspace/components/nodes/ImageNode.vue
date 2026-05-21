<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, 'is-expand-mode': !!expandModeActive }"
    :style="nodeStyle"
  >
    <NodeHeader
      class="image-node-header-shell"
      :class="{ 'is-hidden': !!expandModeActive }"
      :title="node.title || 'IMAGE'"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.2"
        >
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <circle cx="8.5" cy="8.5" r="1.5" />
          <path d="M3 14l4-4 5 5 5-5 4 4" />
        </svg>
      </template>
    </NodeHeader>
    <div class="image-node-body" @contextmenu.stop>
      <div
        ref="imageStageRef"
        class="image-node-stage"
        data-flow-handle-frame
        :data-image-node-id="node.id"
        :data-flow-node-id="flowNodeId || undefined"
        :class="{
          'is-loading': props.directGenerating && !displayImage,
          'is-expand-mode': !!expandModeActive,
        }"
        :style="{ height: `${stageHeight}px` }"
        @dragover.stop="onStageDragOver"
        @drop.stop="onStageDrop"
        @contextmenu.prevent.stop="onStageContextMenu"
      >
        <template v-if="displayImage">
          <img
            :src="stageDisplayUrl"
            class="image-node-stage-image"
            draggable="false"
            @dblclick="emit('append-to-canvas')"
          />
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
          <div class="image-node-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('open-preview')">预览</NodeActionButton>
            <NodeActionButton v-if="!props.inputLocked" variant="stage-ghost" @click="emit('reset')">
              清空
            </NodeActionButton>
          </div>
        </template>
        <div
          v-if="!displayImage"
          class="image-node-stage-empty"
          :class="{ 'is-readonly': props.inputLocked, 'is-loading': props.directGenerating }"
          @click="onEmptyStateClick"
        >
          <NodeEmptyStateIcon
            :variant="emptyIconVariant"
            :class="{ 'image-node-stage-loader-logo': props.directGenerating }"
          />
          <span v-if="!props.directGenerating">{{ emptyStateLabel }}</span>
        </div>
      </div>

      <HomeCanvasImageActionMenu
        v-if="showImageActionMenu"
        :show-style-strength-control="!!showStyleStrengthControl"
        :direct-generating="!!directGenerating"
        :display-height="stageHeight"
        :style-innovate-strength-pct="styleInnovateStrengthPct ?? 50"
        :set-style-strength-track-ref="setStyleStrengthTrackRef"
        @action-remove-bg="emit('action', 'remove-bg')"
        @style-innovate-mousedown="emit('style-innovate-mousedown', $event)"
        @style-innovate-click="emit('action', 'style-innovate')"
        @action-sketch-to-design="emit('action', 'sketch-to-design')"
        @action-canvas-extend="emit('action', 'canvas-extend')"
        @style-strength-track-mousedown="emit('style-strength-track-mousedown', $event)"
      />
      <NodePromptComposer
        v-if="showPromptComposer"
        v-model="promptDraft"
        offset="52px"
        placeholder="输入提示词改造这张图"
        :disabled="!!directGenerating"
        :output-count="outputCountValue"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, type ComponentPublicInstance } from 'vue'
import {
  clampImageNodeOutputCount,
  type ImageNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import NodeEmptyStateIcon from './NodeEmptyStateIcon.vue'
import NodePromptComposer from './NodePromptComposer.vue'
import ImageCanvasExtendOverlay from './ImageCanvasExtendOverlay.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: ImageNodeItem
  flowNodeId?: string
  setImageStageRef?: (nodeId: string, element: Element | null) => void
  clearImageStageRef?: (nodeId: string, element?: Element | null) => void
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
  displayMode?: 'source' | 'node-output' | 'linked-output' | 'mask' | 'pending'
  emptyIconVariant?: 'image' | 'fusion'
  inputLocked?: boolean
  uiState?: NodeUiState
  actionMenuVisible?: boolean
  promptComposerVisible?: boolean
  directGenerating?: boolean
  showStyleStrengthControl?: boolean
  styleInnovateStrengthPct?: number
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'open-preview'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number): void
  (e: 'expand-submit', prompt: string, outputCount: number): void
  (e: 'action', action: 'remove-bg' | 'style-innovate' | 'sketch-to-design' | 'canvas-extend'): void
  (e: 'style-innovate-mousedown', event: MouseEvent): void
  (e: 'style-strength-track-mousedown', event: MouseEvent): void
  (e: 'expand-handle-pointerdown', payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent }): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, { maxSide: 1280 })
const emptyStateLabel = computed(() => {
  if (props.inputLocked) return 'Waiting...'
  return '上传或拖拽图片作为输入'
})
const emptyIconVariant = computed(() => props.emptyIconVariant ?? 'image')

const showImageActionMenu = computed(() => !!props.actionMenuVisible && !!props.displayImage)
const showPromptComposer = computed(
  () => !!props.promptComposerVisible && !!props.displayImage && !props.expandModeActive,
)
const imageStageRef = ref<HTMLElement | null>(null)
const promptDraft = ref('')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watch(
  [() => props.flowNodeId, imageStageRef, () => props.setImageStageRef, () => props.clearImageStageRef],
  ([flowNodeId, stageElement, setImageStageRef, clearImageStageRef], oldValues, onCleanup) => {
    const [prevFlowNodeId, prevStageElement] = oldValues ?? []
    if (prevFlowNodeId && prevFlowNodeId !== flowNodeId) {
      clearImageStageRef?.(prevFlowNodeId, prevStageElement)
    }
    if (flowNodeId) {
      setImageStageRef?.(flowNodeId, stageElement)
    }
    onCleanup(() => {
      if (flowNodeId) clearImageStageRef?.(flowNodeId, stageElement)
    })
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (!props.flowNodeId) return
  props.clearImageStageRef?.(props.flowNodeId, imageStageRef.value)
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
  if (props.inputLocked) return
  emit('open-file')
}

function onStageContextMenu(event: MouseEvent) {
  if (!props.displayImage) return
  emit('contextmenu', event)
}

function submitPrompt(prompt: string, outputCount: number) {
  if (!prompt || props.directGenerating) return
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
}

function submitExpand(prompt: string, outputCount: number) {
  if (props.directGenerating) return
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
}

</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.workflow-node.image-node {
  overflow: visible;
}

.image-node-body {
  position: relative;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-radius: 11px;
  background: #ffffff;
  overflow: visible;
}

.image-node-header-shell.is-hidden {
  visibility: hidden;
  pointer-events: none;
}

.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #fff;
  cursor: grab;
  user-select: none;
}

.workflow-node.image-node.dragging .image-node-stage-image {
  cursor: grabbing;
}

.image-node-stage.is-expand-mode .image-node-stage-image {
  pointer-events: none;
}

.image-node-stage-image.preview {
  pointer-events: none;
}

.image-node-stage {
  position: relative;
  width: 100%;
  border: 1.5px solid #e2e8f0;
  border-radius: 11px;
  background: #ffffff;
  box-sizing: border-box;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.image-node-stage.is-expand-mode {
  overflow: visible;
  border-color: transparent;
}

.workflow-node.image-node.is-expand-mode .image-node-body {
  background: transparent;
}

.workflow-node.image-node.active .image-node-stage {
  border-color: #9278e6;
}

.workflow-node.image-node.is-expand-mode .image-node-stage,
.workflow-node.image-node.is-expand-mode.active .image-node-stage {
  border-color: transparent;
}

.image-node-stage.is-loading {
  border-color: #e2e8f0;
  box-shadow:
    0 0 30px rgba(239, 68, 68, 0.2),
    0 0 48px rgba(146, 120, 230, 0.18);
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
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}

.image-node-stage-empty:hover {
  background: #ffffff;
  color: #8894a5;
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
  cursor: wait;
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
  transition: opacity 0.16s ease;
}

.image-node-stage:hover .image-node-stage-actions {
  opacity: 1;
  pointer-events: auto;
}

.image-node-stage.is-expand-mode .image-node-stage-actions {
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
