<template>
  <div
    class="workflow-node viewpoint-node"
    data-node-kind="viewpoint"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      :title="node.title || 'DISPLAY'"
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
          stroke-width="2.1"
        >
          <path d="M4.5 7.5h7l2 2.4h6V17a2 2 0 0 1-2 2h-11a2 2 0 0 1-2-2z" />
          <path d="M10 12.2h4" />
          <path d="M12 10.2v4" />
        </svg>
      </template>
    </NodeHeader>

    <div class="viewpoint-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="viewpoint-mode-card node-mode-card">
        <div class="viewpoint-control-row node-control-head">
          <span class="node-control-label">Garment</span>
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
        <div class="viewpoint-control-row node-control-head">
          <span class="node-control-label">Angle</span>
        </div>
        <div class="viewpoint-option-row node-option-row">
          <NodeActionButton
            v-for="option in currentViewOptions"
            :key="option.key"
            variant="chip"
            :active="node.selectedViewpoint === option.key"
            @click="emit('select:viewpoint', option.key)"
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

      <NodeExecutionFooter
        :items="connectionChips"
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '生成'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ViewpointNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

import { buildNodeIoFromSlots, VIEWPOINT_NODE_IO_SLOTS } from './config/nodeIoConfig'
import {
  VIEWPOINT_CATEGORY_OPTIONS,
  VIEWPOINT_LIGHTING_OPTIONS,
  VIEWPOINT_OPTIONS_BY_CATEGORY,
} from './config/nodeOptionConfig'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: ViewpointNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:category', value: ViewpointNodeItem['activeCategory']): void
  (e: 'select:viewpoint', value: string): void
  (e: 'update:lighting', value: ViewpointNodeItem['lighting']): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const categoryOptions = VIEWPOINT_CATEGORY_OPTIONS
const lightingOptions = VIEWPOINT_LIGHTING_OPTIONS
const currentViewOptions = computed(
  () => VIEWPOINT_OPTIONS_BY_CATEGORY[props.node.activeCategory] ?? [],
)
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
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.viewpoint-node-body {
  gap: 8px;
}

.viewpoint-lighting-head {
  margin-top: 2px;
}
</style>
