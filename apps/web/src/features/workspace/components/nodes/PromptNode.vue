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
        <MessageText />
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
        <textarea
          ref="textInputRef"
          :value="node.text"
          class="workflow-node-input"
          placeholder="type your idea..."
          @input="onInput"
          @focus="emit('focus')"
          @contextmenu.stop
        />
        <div ref="actionsRef" class="prompt-node-actions node-tool-surface">
          <NodeAspectRatioPicker
            :model-value="aspectRatio"
            :visible="aspectRatioPickerVisible"
            @toggle="toggleAspectRatioPicker"
            @select="selectAspectRatio"
          />
          <NodeOutputCountPicker
            :model-value="outputCount"
            :visible="outputCountPickerVisible"
            title="选择张数"
            aria-label="输出张数"
            @toggle="toggleOutputCountPicker"
            @select="selectOutputCount"
          />
          <button
            class="image-node-prompt-send node-tool-send node-tool-button"
            type="button"
            :disabled="!canGenerate"
            title="发送"
            @click="emit('generate')"
          >
            <Send class="node-action-icon node-send-icon" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessageText, Plus, Send } from '@iconoir/vue'
import { clampImageNodeOutputCount, type PromptNodeItem } from '../../types/canvasNodes'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'
import { useAutoResizeTextarea } from './composables/useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from './composables/useNodeToolPopoverGroup'
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

function onInput(event: Event) {
  const input = event.target as HTMLTextAreaElement
  emit('update:text', input.value)
}
const thumbsRef = ref<HTMLElement | null>(null)
const textInputRef = ref<HTMLTextAreaElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const addSlotDragOver = ref(false)
const maxInputImages = 4
const outputCount = computed(() => clampImageNodeOutputCount(props.node.outputCount))
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const visibleInputImages = computed(() => props.node.inputImages.slice(0, maxInputImages))
const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxInputImages)
const canGenerate = computed(() => props.node.text.trim().length > 0)
const toolPopovers = useNodeToolPopoverGroup({
  keys: ['aspect-ratio', 'output-count'] as const,
  rootRefs: [actionsRef],
})
const outputCountPickerVisible = toolPopovers.isOpen('output-count')
const aspectRatioPickerVisible = toolPopovers.isOpen('aspect-ratio')
useAutoResizeTextarea(textInputRef, {
  watchSource: [() => props.node.text, () => props.node.width],
})
const { sortableItems: sortableInputImages } = useSortableMediaInputs({
  containerRef: thumbsRef,
  sourceItems: visibleInputImages,
  draggableSelector: '.node-input-thumbnail-item',
  filterSelector: '.prompt-node-thumb-add, .node-input-thumbnail-remove',
  fallbackClass: 'prompt-node-sortable-fallback',
  onUpdate: (next) => emit('update:input-images', next),
})
function toggleOutputCountPicker() {
  toolPopovers.toggle('output-count')
}
function selectOutputCount(value: number) {
  toolPopovers.close('output-count')
  emit('update:output-count', clampImageNodeOutputCount(value))
}
function toggleAspectRatioPicker() {
  toolPopovers.toggle('aspect-ratio')
}
function selectAspectRatio(value: PromptAspectRatio) {
  toolPopovers.close('aspect-ratio')
  emit('update:aspect-ratio', value)
}
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
  stroke-width: 2.1;
}
.node-send-icon {
  transform: translateX(0.5px);
}

.workflow-node-input {
  flex: 1 1 auto;
  width: 100%;
  min-height: 0;
  cursor: text;
  border: none;
  resize: none;
  outline: none;
  padding: 0;
  box-sizing: border-box;
  font-size: 14px;
  line-height: 1.6;
  color: #334155;
  background: transparent;
  font-family: inherit;
  overflow: hidden;
}

.workflow-node-input::placeholder {
  color: #94a3b8;
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
.prompt-node-actions {
  align-self: flex-end;
  display: flex;
  gap: var(--node-tool-button-gap);
}
.prompt-node-actions.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}
</style>
