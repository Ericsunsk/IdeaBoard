<template>
  <div
    class="workflow-node prompt-node"
    data-node-kind="prompt"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'Prompt'"
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <ChatLines />
      </template>
    </NodeHeader>
    <div class="prompt-node-shell" data-flow-handle-frame>
      <div class="prompt-node-content">
        <div
          ref="thumbsRef"
          class="prompt-node-thumbs node-tool-surface node-input-thumbnail-contract variant-standard"
        >
          <NodeInputThumbnail
            v-for="item in sortableInputImages"
            :key="item.id"
            class="prompt-node-thumb prompt-node-thumb-item"
            :src="item.url"
            removable
            :data-image-id="item.id"
            @mousedown.stop
            @contextmenu.stop.prevent="emit('input-image-preview', item)"
            @remove="removeInputImage(item.id)"
          />
          <button
            v-if="canAcceptMoreImages"
            class="prompt-node-thumb prompt-node-thumb-add node-tool-output-count node-tool-button"
            :class="{ 'is-drop-target': addSlotDragOver }"
            type="button"
            title="添加图片输入"
            aria-label="添加图片输入"
            @mousedown.stop
            @pointerdown.stop
            @click.stop="emit('input-image-open')"
            @dragenter.stop.prevent="onAddSlotDragOver"
            @dragover.stop.prevent="onAddSlotDragOver"
            @dragleave.stop="onAddSlotDragLeave"
            @drop.stop.prevent="onAddSlotDrop"
          >
            <Plus class="node-action-icon" />
          </button>
        </div>
        <PromptTextComposer
          :model-value="node.text"
          :output-count="outputCount"
          :aspect-ratio="aspectRatio"
          :submit-disabled="!canGenerate"
          class="prompt-node-composer"
          @update:model-value="emit('update:text', $event)"
          @update:output-count="emit('update:output-count', $event)"
          @update:aspect-ratio="emit('update:aspect-ratio', $event)"
          @focus="emit('focus')"
          @submit="emit('generate')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChatLines, Plus } from '@iconoir/vue'
import { clampImageNodeOutputCount, type PromptNodeItem } from '../../types/canvasNodes'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import PromptTextComposer from './PromptTextComposer.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'
import { useSortableMediaInputs } from './composables/useSortableMediaInputs'
import {
  normalizePromptAspectRatio,
  type PromptAspectRatio,
} from '../../config/promptAspectRatio'

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
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'update:input-images', value: PromptNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-open'): void
  (e: 'input-image-preview', value: PromptNodeItem['inputImages'][number]): void
  (e: 'input-image-remove', value: PromptNodeItem['inputImages'][number]): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const thumbsRef = ref<HTMLElement | null>(null)
const addSlotDragOver = ref(false)
const maxInputImages = 4
const outputCount = computed(() => clampImageNodeOutputCount(props.node.outputCount))
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const visibleInputImages = computed(() => props.node.inputImages.slice(0, maxInputImages))
const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxInputImages)
const canGenerate = computed(() => props.node.text.trim().length > 0)
const { sortableItems: sortableInputImages } = useSortableMediaInputs({
  containerRef: thumbsRef,
  sourceItems: visibleInputImages,
  draggableSelector: '.node-input-thumbnail-item',
  filterSelector: '.prompt-node-thumb-add, .node-input-thumbnail-remove',
  fallbackClass: 'prompt-node-sortable-fallback',
  onUpdate: (next) => emit('update:input-images', next),
})
function removeInputImage(imageId: string) {
  const item = visibleInputImages.value.find((inputImage) => inputImage.id === imageId)
  if (!item) return
  emit('input-image-remove', item)
}
function onAddSlotDragOver(event: DragEvent) {
  if (!canAcceptMoreImages.value) return
  addSlotDragOver.value = true
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
}
function onAddSlotDragLeave() {
  addSlotDragOver.value = false
}
function onAddSlotDrop(event: DragEvent) {
  addSlotDragOver.value = false
  if (!canAcceptMoreImages.value) return
  emit('input-image-drop', event)
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped src="./styles/node-tool-buttons.css"></style>
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
  position: relative;
  min-height: 280px;
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  background: #ffffff;
  overflow: visible;
  transition: border-color 0.2s ease;
}
.prompt-node-content {
  display: flex;
  min-height: 280px;
  flex-direction: column;
  gap: 12px;
  padding: 20px;
  box-sizing: border-box;
}

.workflow-node.prompt-node.active .prompt-node-shell {
  border-color: #9278e6;
}
.node-action-icon {
  width: 16px;
  height: 16px;
  stroke-width: var(--node-tool-icon-stroke);
}
.prompt-node-thumbs {
  --node-tool-button-size: var(--node-input-thumbnail-size);
  --node-tool-button-radius: 8px;
  --prompt-thumb-gap: 6px;
  position: relative;
  display: grid;
  grid-template-columns: repeat(4, var(--node-input-thumbnail-size));
  gap: var(--prompt-thumb-gap);
  justify-content: space-between;
  overflow: visible;
  z-index: 3;
}
.prompt-node-thumb {
  width: var(--node-input-thumbnail-size);
  height: var(--node-input-thumbnail-size);
  flex: 0 0 var(--node-input-thumbnail-size);
  cursor: grab;
}
.prompt-node-thumbs .prompt-node-thumb {
  aspect-ratio: 1 / 1;
}
:global(.prompt-node-sortable-fallback) {
  opacity: 0 !important;
}
.prompt-node-thumb-add {
  color: #64748b;
  border-style: dashed;
  cursor: copy;
}
.prompt-node-thumb-add:hover,
.prompt-node-thumb-add:focus,
.prompt-node-thumb-add.is-drop-target {
  color: var(--node-tool-button-accent);
  border-color: var(--node-tool-button-accent);
  background: #f8f7ff;
}
.prompt-node-composer {
  flex: 1 1 auto;
}
</style>
