<template>
  <NodeShell
    kind="prompt"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || 'Prompt'"
    :active="active"
    :dragging="dragging"
    :copy-mode="copyMode"
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <ChatLines />
    </template>
    <NodeStageFrame variant="prompt" :active="active">
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
            class="prompt-node-thumb-add node-input-thumbnail-add node-tool-output-count node-tool-button node-tool-tooltip-trigger"
            :class="{
              'is-drag-over': addSlotDragOver,
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
            <Plus class="size-4" />
            <span class="node-tool-tooltip" aria-hidden="true">添加图片输入</span>
          </button>
        </div>
        <PromptTextComposer
          :model-value="node.text"
          :output-count="outputCount"
          :aspect-ratio="aspectRatio"
          :submit-disabled="!canGenerate || props.generationAvailable === false"
          :billing-state="billingQuote"
          class="flex-auto"
          @update:model-value="emit('update:text', $event)"
          @update:output-count="emit('update:output-count', $event)"
          @update:aspect-ratio="emit('update:aspect-ratio', $event)"
          @focus="emit('focus')"
          @submit="emit('generate')"
        />
      </div>
    </NodeStageFrame>
  </NodeShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChatLines, Plus } from '@iconoir/vue'
import { clampImageNodeOutputCount, type PromptNodeItem } from '../../../canvas-board/domain'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import NodeShell from './NodeShell.vue'
import NodeStageFrame from './NodeStageFrame.vue'
import PromptTextComposer from './PromptTextComposer.vue'
import { useImageInputSlots } from './composables/useImageInputSlots'
import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../../canvas-board/domain'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'
import { useCanvasBillingQuote } from '../../execution'

const props = defineProps<{
  node: PromptNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generationAvailable?: boolean
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

const thumbsRef = ref<HTMLElement | null>(null)
const maxInputImages = 4
const outputCount = computed(() => clampImageNodeOutputCount(props.node.outputCount))
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const canGenerate = computed(() => props.node.text.trim().length > 0)
const billingQuote = useCanvasBillingQuote(() => WORKFLOW_FEATURE.patternGen)
const {
  addSlotDragOver,
  canAcceptMoreImages,
  onAddSlotDragLeave,
  onAddSlotDragOver,
  onAddSlotDrop,
  removeInputImage,
  sortableInputImages,
} = useImageInputSlots({
  containerRef: thumbsRef,
  inputImages: () => props.node.inputImages,
  maxImages: maxInputImages,
  draggableSelector: '.node-input-thumbnail-item',
  filterSelector: '.prompt-node-thumb-add, .node-input-thumbnail-remove',
  onUpdate: (next) => emit('update:input-images', next),
  onDrop: (event) => emit('input-image-drop', event),
  onRemove: (item) => emit('input-image-remove', item),
})
</script>
