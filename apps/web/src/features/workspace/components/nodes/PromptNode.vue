<template>
  <div
    class="workflow-node prompt-node"
    data-node-kind="prompt"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      title="Prompt"
      @node-mousedown="emit('node-mousedown', $event)"
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
    <textarea
      :value="node.text"
      class="min-h-[100px] w-full resize-none border-none bg-transparent p-3 font-[inherit] text-sm leading-[1.6] text-[#334155] outline-none placeholder:text-[#94a3b8]"
      placeholder="type your idea..."
      @input="onInput"
      @focus="emit('focus')"
      @mousedown.stop
      @contextmenu.stop
    />
  </div>
</template>

<script setup lang="ts">
import type { PromptNodeItem } from '../../types/canvasNodes'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: PromptNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  uiState?: PromptNodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'focus'): void
  (e: 'update:text', value: string): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
}
</script>

<style scoped src="./node-shared.css"></style>
