<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <div class="workflow-node-header" @mousedown.stop="emit('node-mousedown', $event)">
      <div class="node-header-title">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" style="margin-right: 6px; opacity: 0.8">
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <circle cx="8.5" cy="8.5" r="1.5" />
          <path d="M3 14l4-4 5 5 5-5 4 4" />
        </svg>
        Image
      </div>
      <button class="node-delete-btn" title="删除" @mousedown.stop @click.stop="emit('delete')">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>
    <div class="image-node-body" @mousedown.stop @contextmenu.stop>
      <div
        class="image-node-stage"
        :data-image-node-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
        @mousedown.stop="onStageMouseDown"
        @contextmenu.prevent.stop
      >
        <template v-if="displayImage">
          <img
            :src="displayImage.url"
            class="image-node-stage-image"
            draggable="true"
            @dragstart="emit('display-dragstart', $event)"
            @dblclick="emit('append-to-canvas')"
          />
          <div class="image-node-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('reset')">清空</NodeActionButton>
          </div>
        </template>
        <div v-else class="image-node-stage-empty" @click="emit('open-file')">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>上传或拖拽图片作为输入</span>
        </div>
      </div>

      <div
        v-if="previewPopupOpen && displayImage"
        class="image-preview-popup"
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
            class="image-node-stage-image preview"
            draggable="false"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import type { ImageNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'

const props = defineProps<{
  node: ImageNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
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

const nodeStyle = computed(() => ({
  left: '50%',
  top: '50%',
  transform: `translate(${props.node.x}px, ${props.node.y}px)`,
  width: `${props.node.width}px`,
}))

const previewPopupOpen = ref(false)

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
  previewPopupOpen.value = true
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
<style scoped>
.workflow-node.image-node {
  overflow: visible;
}

.workflow-node.image-node .workflow-node-header {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}

.image-node-body {
  position: relative;
  padding: 10px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.image-node-stage {
  position: relative;
  min-height: 120px;
  border-radius: 12px;
  border: 1px solid #e4e7ec;
  background: linear-gradient(160deg, #fafbfd 0%, #f1f4f8 100%);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.image-node-stage-empty {
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  border: 1px dashed #c7cfdd;
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #6d7685;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.image-node-stage-empty:hover {
  border-color: #9278e6;
  color: #6757c8;
  background: rgba(146, 120, 230, 0.05);
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

.image-node-stage-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
}

.image-preview-popup {
  position: absolute;
  left: 50%;
  top: 12px;
  transform: translateX(-50%);
  z-index: 28;
  padding: 8px;
  border-radius: 12px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  box-shadow: 0 10px 30px rgba(25, 30, 45, 0.12);
}

.image-preview-popup-stage {
  overflow: hidden;
  border-radius: 12px;
  background: linear-gradient(160deg, #fafbfd 0%, #f1f4f8 100%);
}

</style>
