<template>
  <div
    class="workflow-node prompt-node flex flex-col gap-0.5 border-0 bg-transparent shadow-none"
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
    <div
      class="relative min-h-[280px] overflow-visible rounded-2xl border-[1.5px] bg-white transition-colors"
      :class="active ? 'border-[#9278e6]' : 'border-[#e2e8f0]'"
      data-flow-handle-frame
    >
      <div class="box-border flex min-h-[280px] flex-col gap-3 p-5">
        <div
          ref="thumbsRef"
          class="node-tool-surface node-input-thumbnail-contract variant-standard relative z-[3] grid justify-between gap-[var(--prompt-thumb-gap)] overflow-visible [--node-tool-button-radius:8px] [--node-tool-button-size:var(--node-input-thumbnail-size)] [--prompt-thumb-gap:6px] [grid-template-columns:repeat(4,var(--node-input-thumbnail-size))]"
        >
          <NodeInputThumbnail
            v-for="item in sortableInputImages"
            :key="item.id"
            class="prompt-node-thumb-item aspect-square h-[var(--node-input-thumbnail-size)] w-[var(--node-input-thumbnail-size)] flex-none cursor-grab"
            :src="item.url"
            removable
            :data-image-id="item.id"
            @mousedown.stop
            @contextmenu.stop.prevent="emit('input-image-preview', item)"
            @remove="removeInputImage(item.id)"
          />
          <button
            v-if="canAcceptMoreImages"
            class="node-tool-output-count node-tool-button node-tool-tooltip-trigger aspect-square h-[var(--node-input-thumbnail-size)] w-[var(--node-input-thumbnail-size)] cursor-copy border-dashed text-[#64748b] hover:border-[var(--node-tool-button-accent)] hover:bg-[#f8f7ff] hover:text-[var(--node-tool-button-accent)] focus:border-[var(--node-tool-button-accent)] focus:bg-[#f8f7ff] focus:text-[var(--node-tool-button-accent)]"
            :class="{
              '!border-[var(--node-tool-button-accent)] !bg-[#f8f7ff] !text-[var(--node-tool-button-accent)]':
                addSlotDragOver,
            }"
            type="button"
            aria-label="添加图片输入"
            @mousedown.stop
            @pointerdown.stop
            @click.stop="emit('input-image-open')"
            @dragenter.stop.prevent="onAddSlotDragOver"
            @dragover.stop.prevent="onAddSlotDragOver"
            @dragleave.stop="onAddSlotDragLeave"
            @drop.stop.prevent="onAddSlotDrop"
          >
            <Plus class="size-4 stroke-[var(--node-tool-icon-stroke)]" />
            <span class="node-tool-tooltip" aria-hidden="true">添加图片输入</span>
          </button>
        </div>
        <PromptTextComposer
          :model-value="node.text"
          :output-count="outputCount"
          :aspect-ratio="aspectRatio"
          :submit-disabled="!canGenerate"
          class="flex-auto"
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
import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../config/promptAspectRatio'

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
:global(.prompt-node-sortable-fallback) {
  opacity: 0 !important;
}
</style>
