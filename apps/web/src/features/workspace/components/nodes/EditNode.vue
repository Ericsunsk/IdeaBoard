<template>
  <div
    class="workflow-node edit-node"
    data-node-kind="edit"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'EDIT'"
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
          <path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-4-4L4 16v4z" />
          <path d="M13.5 6.5l4 4" />
        </svg>
      </template>
    </NodeHeader>

    <div class="edit-node-body node-execution-body" @contextmenu.stop>
      <div class="edit-mode-card node-mode-card">
        <div class="edit-mode-row node-option-row">
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'partial-edit'"
            @click="emit('update:mode', 'partial-edit')"
          >
            Detail
          </NodeActionButton>
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'bg-swap'"
            @click="emit('update:mode', 'bg-swap')"
          >
            Background
          </NodeActionButton>
          <NodeActionButton
            variant="chip"
            :active="node.mode === 'face-swap'"
            @click="emit('update:mode', 'face-swap')"
          >
            Face
          </NodeActionButton>
        </div>
      </div>

      <NodeExecutionFooter
        :disabled="uiState?.primaryAction?.disabled"
        :label="uiState?.primaryAction?.label || '编辑'"
        @generate="emit('generate')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { EditNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'

import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: EditNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: NodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:mode', value: 'partial-edit' | 'bg-swap' | 'face-swap'): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.edit-node-body {
  gap: 8px;
}
</style>
