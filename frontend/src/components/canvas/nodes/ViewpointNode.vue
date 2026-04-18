<template>
  <div
    class="workflow-node viewpoint-node"
    data-node-kind="viewpoint"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Viewpoint"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M4.5 7.5h7l2 2.4h6V17a2 2 0 0 1-2 2h-11a2 2 0 0 1-2-2z" />
          <path d="M10 12.2h4" />
          <path d="M12 10.2v4" />
        </svg>
      </template>
    </NodeHeader>

    <div class="viewpoint-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div
        class="viewpoint-stage"
        :data-viewpoint-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
      >
        <template v-if="primaryImage">
          <img :src="primaryImage.url" class="viewpoint-stage-image" />
          <div class="viewpoint-stage-topbar">
            <span class="viewpoint-count-badge">{{ sourceImages.length }}/3</span>
          </div>
          <div v-if="sourceImages.length > 1" class="viewpoint-thumb-row">
            <button
              v-for="(image, index) in sourceImages.slice(0, 3)"
              :key="`${image.url}-${index}`"
              type="button"
              class="viewpoint-thumb"
              :class="{ active: index === activePreviewIndex }"
              @click.stop="activePreviewIndex = index"
            >
              <img :src="image.url" class="viewpoint-thumb-image" />
              <span
                v-if="!hasConnectedImageInput"
                class="viewpoint-thumb-remove"
                @click.stop="emit('remove-image', index)"
              >
                ×
              </span>
            </button>
          </div>
          <div v-if="!hasConnectedImageInput" class="model-stage-actions viewpoint-stage-actions">
            <NodeActionButton variant="stage" @click="emit('open-file-picker')">Add</NodeActionButton>
            <NodeActionButton variant="stage-ghost" @click="emit('clear-images')">Clear</NodeActionButton>
          </div>
        </template>
        <div v-else class="model-stage-empty viewpoint-stage-empty" @click="emit('open-file-picker')">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>上传服装图（最多 3 张）</span>
        </div>
      </div>

      <div class="viewpoint-mode-card node-mode-card">
        <div class="viewpoint-control-row node-control-head">
          <span class="node-control-label">View</span>
          <span class="viewpoint-selection-count">{{ node.selectedViewpoints.length }}/4</span>
        </div>
        <div class="viewpoint-category-row node-option-row">
          <NodeActionButton
            v-for="option in categoryOptions"
            :key="option.value"
            variant="chip"
            :active="node.activeCategory === option.value"
            @click="emit('update:category', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
        <div class="viewpoint-option-row node-option-row">
          <NodeActionButton
            v-for="option in currentViewOptions"
            :key="option.key"
            variant="chip"
            :active="node.selectedViewpoints.includes(option.key)"
            @click="emit('toggle:viewpoint', option.key)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
        <div class="viewpoint-control-row node-control-head viewpoint-lighting-head">
          <span class="node-control-label">Light</span>
        </div>
        <div class="viewpoint-lighting-row node-option-row">
          <NodeActionButton
            v-for="option in lightingOptions"
            :key="option.value"
            variant="chip"
            :active="node.lighting === option.value"
            @click="emit('update:lighting', option.value)"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '生成' }}
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="viewpoint"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ImageNodeOutputItem, ViewpointNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, VIEWPOINT_NODE_IO_SLOTS } from './nodeIoConfig'
import {
  VIEWPOINT_CATEGORY_OPTIONS,
  VIEWPOINT_LIGHTING_OPTIONS,
  VIEWPOINT_OPTIONS_BY_CATEGORY,
} from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: ViewpointNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  stageHeight: number
  sourceImages: ImageNodeOutputItem[]
  hasConnectedImageInput?: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'open-file-picker'): void
  (e: 'clear-images'): void
  (e: 'remove-image', index: number): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'update:category', value: ViewpointNodeItem['activeCategory']): void
  (e: 'toggle:viewpoint', value: string): void
  (e: 'update:lighting', value: ViewpointNodeItem['lighting']): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image' | 'prompt'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const categoryOptions = VIEWPOINT_CATEGORY_OPTIONS
const lightingOptions = VIEWPOINT_LIGHTING_OPTIONS
const activePreviewIndex = ref(0)
const currentViewOptions = computed(() => VIEWPOINT_OPTIONS_BY_CATEGORY[props.node.activeCategory] ?? [])
const primaryImage = computed(() => props.sourceImages[activePreviewIndex.value] ?? props.sourceImages[0] ?? null)
const io = computed(() =>
  buildNodeIoFromSlots(VIEWPOINT_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

watch(
  () => props.sourceImages.length,
  (length) => {
    if (length <= 0) {
      activePreviewIndex.value = 0
      return
    }
    if (activePreviewIndex.value >= length) {
      activePreviewIndex.value = 0
    }
  },
  { immediate: true },
)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image' | 'prompt')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.viewpoint-node-body {
  gap: 8px;
}

.viewpoint-stage {
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

.viewpoint-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.viewpoint-stage-topbar {
  position: absolute;
  top: 8px;
  left: 8px;
  right: 8px;
  display: flex;
  justify-content: flex-start;
  pointer-events: none;
}

.viewpoint-count-badge {
  min-height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(216, 222, 234, 0.9);
  color: #667085;
  font-size: 10px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.viewpoint-thumb-row {
  position: absolute;
  left: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
  max-width: calc(100% - 90px);
}

.viewpoint-thumb {
  position: relative;
  width: 40px;
  height: 40px;
  border: 1px solid rgba(216, 222, 234, 0.96);
  background: rgba(255, 255, 255, 0.9);
  border-radius: 10px;
  overflow: hidden;
  padding: 0;
  cursor: default;
}

.viewpoint-thumb.active {
  border-color: rgba(146, 120, 230, 0.55);
}

.viewpoint-thumb-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.viewpoint-thumb-remove {
  position: absolute;
  top: 3px;
  right: 4px;
  width: 14px;
  height: 14px;
  border-radius: 999px;
  background: rgba(17, 24, 39, 0.72);
  color: #fff;
  font-size: 10px;
  line-height: 14px;
  text-align: center;
  cursor: pointer;
}

.viewpoint-stage-actions {
  right: 8px;
  bottom: 8px;
}

.viewpoint-selection-count {
  font-size: 10px;
  font-weight: 700;
  color: #7c3aed;
}

.viewpoint-lighting-head {
  margin-top: 2px;
}

</style>
