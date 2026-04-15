<template>
  <div
    class="workflow-node model-node"
    data-node-kind="model"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <div class="workflow-node-header" @mousedown.stop="emit('node-mousedown', $event)">
      <div class="node-header-title">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" style="margin-right: 6px; opacity: 0.8">
          <circle cx="12" cy="6" r="2.5" />
          <path d="M12 9.5v5M8.5 13.5h7M10 20l2-5 2 5" />
        </svg>
        Model
      </div>
      <button class="node-delete-btn" title="删除" @mousedown.stop @click.stop="emit('delete')">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>

    <div class="model-node-body" @mousedown.stop @contextmenu.stop>
      <div
        class="model-stage"
        :data-model-id="node.id"
        :style="{ height: `${stageHeight}px` }"
        @dragover.prevent
        @drop.prevent.stop="emit('stage-drop', $event)"
      >
        <template v-if="node.sourceImageUrl">
          <img :src="node.sourceImageUrl" class="model-stage-image" />
          <div class="model-stage-actions">
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

      <div class="model-control-group">
        <div class="model-angle-head">
          <span class="model-control-label">角度（最多 4 个）</span>
          <button
            v-if="node.selectedAngles.length > 0"
            type="button"
            class="model-angle-clear-btn"
            @click="emit('clear-angles')"
          >
            清空
          </button>
        </div>
        <div class="model-control-options">
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

      <div class="model-control-group">
        <span class="model-control-label">背景</span>
        <div class="model-control-options">
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

      <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
        生成多角度
      </NodeActionButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ModelNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'

const props = defineProps<{
  node: ModelNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  stageHeight: number
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
}>()

const nodeStyle = computed(() => ({
  left: '50%',
  top: '50%',
  transform: `translate(${props.node.x}px, ${props.node.y}px)`,
  width: `${props.node.width}px`,
}))
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>

.model-node-body {
  padding: 10px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.model-stage {
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

.model-stage-empty {
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

.model-stage-empty:hover {
  border-color: #9278e6;
  color: #6757c8;
  background: rgba(146, 120, 230, 0.05);
}

.model-stage-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.model-stage-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
}

.model-angle-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
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

.model-control-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.model-control-label {
  font-size: 11px;
  font-weight: 700;
  color: #7b8798;
  letter-spacing: 0.03em;
}

.model-control-options {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
</style>
