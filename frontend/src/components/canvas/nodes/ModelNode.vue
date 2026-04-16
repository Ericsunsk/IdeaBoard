<template>
  <div
    class="workflow-node model-node"
    data-node-kind="model"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Model"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
          <circle cx="12" cy="6" r="2.5" />
          <path d="M12 9.5v5M8.5 13.5h7M10 20l2-5 2 5" />
        </svg>
      </template>
    </NodeHeader>

    <div class="model-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div
        class="model-stage"
        :data-model-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
      >
        <template v-if="effectiveSourceImageUrl">
          <img :src="effectiveSourceImageUrl" class="model-stage-image" />
          <div v-if="node.sourceImageUrl && !hasConnectedImageInput" class="model-stage-actions">
            <NodeActionButton variant="stage" @click="emit('open-file-picker')">更换图片</NodeActionButton>
            <NodeActionButton variant="stage-ghost" @click="emit('clear-image')">清空</NodeActionButton>
          </div>
        </template>
        <div v-else class="model-stage-empty" @click="emit('open-file-picker')">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
          <span>上传模特图（点击或拖拽）</span>
        </div>
      </div>

      <div class="model-control-group node-control-group">
        <div class="model-angle-head node-control-head">
          <span class="model-control-label node-control-label">角度（最多 4 个）</span>
          <button
            v-if="node.selectedAngles.length > 0"
            type="button"
            class="model-angle-clear-btn"
            @click="emit('clear-angles')"
          >
            清空
          </button>
        </div>
        <div class="model-control-options node-option-row">
          <NodeActionButton
            v-for="opt in angleOptions"
            :key="`${node.id}-angle-${opt.key}`"
            variant="chip"
            :active="node.selectedAngles.includes(opt.key)"
            @click="emit('toggle-angle', opt.key)"
          >
            {{ opt.label }}
          </NodeActionButton>
        </div>
        <span v-if="node.selectedAngles.length === 0" class="model-angle-empty">请至少选择一个角度</span>
      </div>

      <div class="model-control-group node-control-group">
        <span class="model-control-label node-control-label">背景</span>
        <div class="model-control-options node-option-row">
          <NodeActionButton
            v-for="opt in backgroundOptions"
            :key="`${node.id}-bg-${opt.key}`"
            variant="chip"
            :active="node.background === opt.key"
            @click="emit('select-background', opt.key)"
          >
            {{ opt.label }}
          </NodeActionButton>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
          生成多角度
        </NodeActionButton>
      </div>
    </div>
    <NodePorts
      node-kind="model"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ModelNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, MODEL_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: ModelNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  stageHeight: number
  sourceImageUrl?: string
  hasConnectedImageInput?: boolean
  imageInputConnected?: boolean
  promptInputConnected?: boolean
  outputConnected?: boolean
  errorPortKeys: string[]
  angleOptions: Array<{ key: string; label: string }>
  backgroundOptions: Array<{ key: string; label: string }>
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'open-file-picker'): void
  (e: 'clear-image'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'toggle-angle', key: string): void
  (e: 'clear-angles'): void
  (e: 'select-background', key: string): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(MODEL_NODE_IO_SLOTS, null, {
    connected: {
      image: props.imageInputConnected,
      prompt: props.promptInputConnected,
      output: props.outputConnected,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

const effectiveSourceImageUrl = computed(() => props.sourceImageUrl ?? props.node.sourceImageUrl)

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>

.model-node-body {
  gap: 8px;
}

.model-stage-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.model-angle-empty {
  font-size: 11px;
  color: #94a3b8;
}

.model-angle-clear-btn {
  border: none;
  background: transparent;
  color: #7f8898;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
}

.model-angle-clear-btn:hover {
  color: #9278e6;
}

</style>
