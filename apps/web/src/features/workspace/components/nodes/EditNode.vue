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
        <EditPencil />
      </template>
    </NodeHeader>

    <div class="edit-node-body node-execution-body" @contextmenu.stop>
      <div class="edit-mode-card node-mode-card">
        <div class="edit-mode-row node-option-row">
          <NodeActionButton
            v-for="option in EDIT_NODE_MODE_OPTIONS"
            :key="option.value"
            variant="chip"
            :active="node.mode === option.value"
            @click="emit('update:mode', option.value)"
          >
            {{ option.label }}
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
import { EditPencil } from '@iconoir/vue'
import type { EditNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { EDIT_NODE_MODE_OPTIONS, type EditNodeMode } from '../../types/editNodeModes'

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
  (e: 'update:mode', value: EditNodeMode): void
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
