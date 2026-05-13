<template>
  <div
    class="workflow-node prompt-node"
    data-node-kind="prompt"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'PROMPT'"
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
          stroke-width="2.5"
        >
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
      </template>
    </NodeHeader>
    <div class="prompt-node-shell" data-flow-handle-frame>
      <textarea
        :value="node.text"
        class="workflow-node-input"
        placeholder="type your idea..."
        @input="onInput"
        @focus="emit('focus')"
        @mousedown.stop
        @contextmenu.stop
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { PromptNodeItem } from '../../types/canvasNodes'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: PromptNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: PromptNodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'focus'): void
  (e: 'update:text', value: string): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.workflow-node.prompt-node {
  border: 0;
  background: transparent;
  box-shadow: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.prompt-node-shell {
  aspect-ratio: 1 / 1;
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  background: #ffffff;
  overflow: hidden;
  transition: border-color 0.2s ease;
}

.workflow-node.prompt-node.active .prompt-node-shell {
  border-color: #9278e6;
}

.workflow-node-input {
  width: 100%;
  height: 100%;
  min-height: 0;
  cursor: text;
  border: none;
  resize: none;
  outline: none;
  padding: 12px 13px;
  box-sizing: border-box;
  font-size: 14px;
  line-height: 1.6;
  color: #334155;
  background: transparent;
  font-family: inherit;
}

.workflow-node-input::placeholder {
  color: #94a3b8;
}
</style>
