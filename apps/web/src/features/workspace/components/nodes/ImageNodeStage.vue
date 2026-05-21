<template>
  <div
    ref="stageElement"
    class="image-node-stage"
    data-flow-handle-frame
    :data-image-node-id="nodeId"
    :data-flow-node-id="flowNodeId || undefined"
    :class="{
      'is-loading': directGenerating && !displayImage,
      'is-expand-mode': !!expandModeActive,
      'is-active': active && !expandModeActive,
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
        :class="{ 'is-dragging': dragging }"
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
        <NodeActionButton v-if="!inputLocked" variant="stage-ghost" @click="emit('reset')">
          清空
        </NodeActionButton>
      </div>
    </template>
    <div
      v-if="!displayImage"
      class="image-node-stage-empty"
      :class="{ 'is-readonly': inputLocked, 'is-loading': directGenerating }"
      @click="onEmptyStateClick"
    >
      <NodeEmptyStateIcon
        :variant="emptyIconVariantComputed"
        :class="{ 'image-node-stage-loader-logo': directGenerating }"
      />
      <span v-if="!directGenerating">{{ emptyStateLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { clampImageNodeOutputCount } from '../../types/canvasNodes'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import ImageCanvasExtendOverlay from './ImageCanvasExtendOverlay.vue'
import NodeActionButton from './NodeActionButton.vue'
import NodeEmptyStateIcon from './NodeEmptyStateIcon.vue'

const props = defineProps<{
  nodeId: string | number
  flowNodeId?: string
  stageHeight: number
  displayImage: { url: string } | null
  inputLocked?: boolean
  emptyIconVariant?: 'image' | 'fusion'
  directGenerating?: boolean
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  outputCount?: number
  active: boolean
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
  (e: 'expand-handle-pointerdown', payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent }): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, { maxSide: 1280 })
const emptyStateLabel = computed(() => (props.inputLocked ? 'Waiting...' : '上传或拖拽图片作为输入'))
const emptyIconVariantComputed = computed(() => props.emptyIconVariant ?? 'image')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.outputCount))

const stageElement = ref<HTMLElement | null>(null)
defineExpose({ stageElement })

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

function submitExpand(prompt: string, outputCount: number) {
  if (props.directGenerating) return
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
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

.image-node-stage-image.is-dragging {
  cursor: grabbing;
}

.image-node-stage.is-expand-mode .image-node-stage-image {
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

.image-node-stage.is-active {
  border-color: #9278e6;
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
