<template>
  <div
    class="workflow-node prompt-node"
    data-node-kind="prompt"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <div class="workflow-node-header" @mousedown.stop="emit('node-mousedown', $event)">
      <div class="node-header-title">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" style="margin-right: 6px; opacity: 0.8">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
        </svg>
        Prompt
      </div>
      <button class="node-delete-btn" title="删除" @mousedown.stop @click.stop="emit('delete')">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <line x1="18" y1="6" x2="6" y2="18"/>
          <line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>
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
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { PromptNodeItem } from '../../../types/canvasNodes'

const props = defineProps<{
  node: PromptNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'focus'): void
  (e: 'update:text', value: string): void
}>()

const nodeStyle = computed(() => ({
  left: '50%',
  top: '50%',
  transform: `translate(${props.node.x}px, ${props.node.y}px)`,
  width: `${props.node.width}px`,
}))

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.workflow-node-input {
  width: 100%;
  min-height: 100px;
  border: none;
  resize: none;
  outline: none;
  padding: 12px;
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
