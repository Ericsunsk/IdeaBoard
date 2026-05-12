<template>
  <div
    class="workflow-node image-node overflow-visible"
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
    <div class="relative flex flex-col gap-2 px-3 pt-2.5 pb-3" @contextmenu.stop>
      <div
        class="image-node-stage"
        :data-image-node-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.stop="onStageDragOver"
        @drop.stop="onStageDrop"
        @mousedown="onStageMouseDown"
        @contextmenu.prevent.stop
      >
        <template v-if="displayImage">
          <div
            class="absolute top-2 left-2 z-[2] rounded-full px-2 py-1 text-[11px] leading-none font-bold tracking-[0.02em] backdrop-blur"
            :class="displayModeClass"
          >
            {{ displayModeLabel }}
          </div>
          <img
            :src="stageDisplayUrl"
            class="nodrag h-full w-full bg-white object-contain"
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
            'text-[#8f789b]': isPendingDisplayMode,
            'cursor-default': props.inputLocked,
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
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>{{ emptyStateLabel }}</span>
        </div>
      </div>

      <div
        v-if="previewPopupOpen && displayImage"
        class="image-preview-popup z-28"
        @mousedown.stop
        @click.stop
        @contextmenu.prevent.stop
      >
        <div
          class="image-preview-popup-stage"
          :style="{ width: `${popupStageWidth}px`, height: `${popupStageHeight}px` }"
        >
          <img
            :src="displayImage.url"
            class="pointer-events-none h-full w-full bg-white object-contain"
            draggable="false"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ImageNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

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
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'display-dragstart', event: DragEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const previewPopupOpen = ref(false)
const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, { maxSide: 1280 })
const displayModeLabel = computed(() => {
  if (props.displayMode === 'pending') return 'Pending'
  if (props.displayMode === 'linked-output') return 'Linked'
  if (props.displayMode === 'node-output') return 'Output'
  if (props.displayMode === 'mask') return 'Mask'
  return 'Source'
})
const displayModeClass = computed(() => {
  if (props.displayMode === 'pending') {
    return 'border border-[rgba(243,187,64,0.18)] bg-[rgba(255,250,239,0.94)] text-[#bb7b00]'
  }
  if (props.displayMode === 'linked-output') {
    return 'border border-[rgba(76,118,229,0.18)] bg-[rgba(244,247,255,0.92)] text-[#496ad5]'
  }
  if (props.displayMode === 'node-output') {
    return 'border border-[rgba(53,171,123,0.18)] bg-[rgba(240,255,248,0.92)] text-[#1d8b61]'
  }
  if (props.displayMode === 'mask') {
    return 'border border-[rgba(146,120,230,0.2)] bg-[rgba(247,243,255,0.94)] text-[#7b67d1]'
  }
  return 'border border-[rgba(146,120,230,0.2)] bg-[rgba(255,255,255,0.9)] text-[#7b67d1]'
})
const isPendingDisplayMode = computed(() => props.displayMode === 'pending')
const emptyStateLabel = computed(() => {
  if (props.displayMode === 'pending' || props.inputLocked) return 'Waiting...'
  return '上传或拖拽图片作为输入'
})

const popupStageHeight = computed(() =>
  Math.max(280, Math.min(560, Math.round(props.stageHeight * 2))),
)

const popupStageWidth = computed(() => {
  const stageWidth = Math.max(120, Math.round(props.node.width - 24))
  const scale = props.stageHeight > 0 ? popupStageHeight.value / props.stageHeight : 1
  return Math.round(stageWidth * scale)
})

function closePreviewPopup() {
  previewPopupOpen.value = false
}

function onStageMouseDown(event: MouseEvent) {
  if (event.button !== 2 || !props.displayImage) return
  event.preventDefault()
  event.stopPropagation()
  previewPopupOpen.value = true
}

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

function onGlobalMouseUp(event: MouseEvent) {
  if (event.button !== 2 && event.button !== 0) return
  closePreviewPopup()
}

function onWindowBlur() {
  closePreviewPopup()
}

window.addEventListener('mouseup', onGlobalMouseUp, true)
window.addEventListener('blur', onWindowBlur, true)

onBeforeUnmount(() => {
  window.removeEventListener('mouseup', onGlobalMouseUp, true)
  window.removeEventListener('blur', onWindowBlur, true)
})
</script>

<style scoped src="./node-shared.css"></style>
