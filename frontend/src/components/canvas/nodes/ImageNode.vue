<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodePorts
      node-kind="image"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
    <NodeHeader
      title="Image"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <circle cx="8.5" cy="8.5" r="1.5" />
          <path d="M3 14l4-4 5 5 5-5 4 4" />
        </svg>
      </template>
    </NodeHeader>
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
          <div
            class="image-node-stage-badge"
            :class="displayModeClass"
          >
            {{ displayModeLabel }}
          </div>
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
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, IMAGE_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: ImageNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
  displayMode?: 'source' | 'output' | 'mask'
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
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const previewPopupOpen = ref(false)
const displayModeLabel = computed(() => {
  if (props.displayMode === 'mask') return 'Mask'
  if (props.displayMode === 'output') return 'Output'
  return 'Source'
})
const displayModeClass = computed(() => {
  if (props.displayMode === 'mask') return 'is-mask'
  if (props.displayMode === 'output') return 'is-output'
  return 'is-source'
})
const ports = computed(() =>
  buildNodeIoFromSlots(IMAGE_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
    },
  }).ports,
)

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

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
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

.image-node-body {
  position: relative;
  padding: 10px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
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

.image-node-stage-badge {
  position: absolute;
  left: 8px;
  top: 8px;
  z-index: 2;
  padding: 4px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1;
  font-weight: 700;
  letter-spacing: 0.02em;
  backdrop-filter: blur(8px);
}

.image-node-stage-badge.is-source {
  border: 1px solid rgba(146, 120, 230, 0.2);
  background: rgba(255, 255, 255, 0.9);
  color: #7b67d1;
}

.image-node-stage-badge.is-output {
  border: 1px solid rgba(53, 171, 123, 0.18);
  background: rgba(240, 255, 248, 0.92);
  color: #1d8b61;
}

.image-node-stage-badge.is-mask {
  border: 1px solid rgba(146, 120, 230, 0.2);
  background: rgba(247, 243, 255, 0.94);
  color: #7b67d1;
}

.image-preview-popup {
  z-index: 28;
}

</style>
