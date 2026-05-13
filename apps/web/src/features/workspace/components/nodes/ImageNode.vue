<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      title="Image"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
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
        class="image-node-stage"
        data-flow-handle-frame
        :data-image-node-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.stop="onStageDragOver"
        @drop.stop="onStageDrop"
        @contextmenu.prevent.stop="onStageContextMenu"
      >
        <template v-if="displayImage">
          <img
            :src="stageDisplayUrl"
            class="image-node-stage-image nodrag"
            draggable="true"
            @dragstart="emit('display-dragstart', $event)"
            @dblclick="emit('append-to-canvas')"
          />
          <div v-if="!props.inputLocked" class="image-node-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('reset')">清空</NodeActionButton>
          </div>
        </template>
        <div
          v-else
          class="image-node-stage-empty"
          :class="{
            'is-pending': isPendingDisplayMode,
            'is-readonly': props.inputLocked,
          }"
          @click="onEmptyStateClick"
        >
          <svg
            v-if="isPendingDisplayMode"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
          >
            <circle cx="12" cy="12" r="8" />
            <path d="M12 8v4l2.5 2.5" />
          </svg>
          <svg
            v-else
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
          >
            <rect x="4" y="4" width="16" height="16" rx="3" />
            <circle cx="15.5" cy="8.5" r="1.5" />
            <path d="M5 16l4.5-4.5 4 4 2-2L19 17" />
          </svg>
          <span>{{ emptyStateLabel }}</span>
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
        @action-to-vector="emit('action', 'to-vector')"
        @action-sketch-to-design="emit('action', 'sketch-to-design')"
        @style-strength-track-mousedown="emit('style-strength-track-mousedown', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import type { ImageNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: ImageNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
  displayMode?: 'source' | 'node-output' | 'linked-output' | 'mask' | 'pending'
  inputLocked?: boolean
  uiState?: NodeUiState
  actionMenuVisible?: boolean
  directGenerating?: boolean
  showStyleStrengthControl?: boolean
  styleInnovateStrengthPct?: number
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'display-dragstart', event: DragEvent): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'action', action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'): void
  (e: 'style-innovate-mousedown', event: MouseEvent): void
  (e: 'style-strength-track-mousedown', event: MouseEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, { maxSide: 1280 })
const isPendingDisplayMode = computed(() => props.displayMode === 'pending')
const emptyStateLabel = computed(() => {
  if (props.displayMode === 'pending' || props.inputLocked) return 'Waiting...'
  return '上传或拖拽图片作为输入'
})

const showImageActionMenu = computed(() => !!props.actionMenuVisible && !!props.displayImage)

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
  border-radius: 16px;
  background: #ffffff;
  overflow: visible;
}

.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #fff;
}

.image-node-stage-image.preview {
  pointer-events: none;
}

.image-node-stage {
  position: relative;
  width: 100%;
  min-height: 120px;
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  background: #ffffff;
  box-sizing: border-box;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.workflow-node.image-node.active .image-node-stage {
  border-color: #9278e6;
}

.image-node-stage-empty {
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 16px;
  background: #ffffff;
  color: #8a95a5;
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
  color: #64748b;
}

.image-node-stage-empty svg {
  width: 84px;
  height: 84px;
  stroke-width: 1.7;
}

.image-node-stage-empty span {
  display: none;
}

.image-node-stage-empty.is-readonly {
  cursor: default;
}

.image-node-stage-empty.is-pending {
  color: #8f789b;
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
</style>
