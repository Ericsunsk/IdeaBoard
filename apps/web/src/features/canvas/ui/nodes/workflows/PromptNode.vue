<template>
  <NodeShell
    kind="prompt"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || t('canvas.node.prompt')"
    :is-active="isActive"
    :is-dragging="isDragging"
    :is-copy-mode="isCopyMode"
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <ChatLines />
    </template>
    <NodeStageFrame
      variant="prompt"
      :is-active="isActive"
      :is-connection-drop-target="isConnectionDropTarget"
    >
      <div class="box-border flex min-h-[280px] flex-col gap-3 p-5">
        <div
          ref="thumbsRef"
          class="node-tool-surface node-input-thumbnail-contract variant-standard relative z-[3] grid justify-between gap-[var(--prompt-thumb-gap)] overflow-visible [--node-tool-button-radius:var(--canvas-radius-md)] [--node-tool-button-size:var(--node-input-thumbnail-size)] [--prompt-thumb-gap:6px] [grid-template-columns:repeat(4,var(--node-input-thumbnail-size))]"
        >
          <NodeInputThumbnail
            v-for="item in sortableInputImages"
            :key="item.id"
            class="prompt-node-thumb-item aspect-square h-[var(--node-input-thumbnail-size)] w-[var(--node-input-thumbnail-size)] flex-none cursor-grab"
            :src="item.url"
            :asset-id="item.mediaRef?.assetId"
            is-removable
            :data-image-id="item.id"
            @mousedown.stop
            @contextmenu.stop.prevent="emit('input-image-preview', item)"
            @remove="removeInputImage(item.id)"
          />
          <button
            v-if="canAcceptMoreImages"
            class="prompt-node-thumb-add node-input-thumbnail-add node-tool-output-count node-tool-button canvas-tool-tooltip-trigger"
            :class="{
              'is-drag-over': isAddSlotDragOver,
            }"
            type="button"
            :aria-label="t('canvas.controls.addImageInput')"
            @mousedown.stop
            @pointerdown.stop
            @click.stop="emit('input-image-open')"
            @dragenter.stop.prevent="onAddSlotDragOver"
            @dragover.stop.prevent="onAddSlotDragOver"
            @dragleave.stop="onAddSlotDragLeave"
            @drop.stop.prevent="onAddSlotDrop"
          >
            <Plus class="size-4" />
            <span class="canvas-tool-tooltip" aria-hidden="true">{{
              t('canvas.controls.addImageInput')
            }}</span>
          </button>
        </div>
        <PromptTextComposer
          :model-value="node.text"
          :output-count="outputCount"
          :output-resolution="node.outputResolution"
          :aspect-ratio="aspectRatio"
          :should-allow-empty="true"
          :is-submit-disabled="props.isGenerationAvailable === false"
          :billing-state="billingQuote"
          class="flex-auto"
          @update:model-value="emit('update:text', $event)"
          @update:output-count="emit('update:output-count', $event)"
          @update:output-resolution="emit('update:output-resolution', $event)"
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
import { useI18n } from 'vue-i18n'
import { ChatLines, Plus } from '@iconoir/vue'
import {
  requireImageNodeOutputCount,
  type PromptNodeItem,
  normalizePromptAspectRatio,
  type PromptAspectRatio,
} from '../../../../board/ports/canvas'
import type { PromptNodeUiState } from '../../../core/nodes/canvasNodeUi.ts'
import {
  NodeInputThumbnail,
  NodeShell,
  NodeStageFrame,
  PromptTextComposer,
  useImageInputSlots,
} from '../primitives'
import { SERVICE } from '../../../../../core/contracts/imageServices'
import { useCanvasBillingQuote } from '../../../session/execution'
import { buildGenerationTaskRequest } from '../../../session/execution'

const props = defineProps<{
  node: PromptNodeItem
  isActive: boolean
  isConnectionDropTarget?: boolean
  isDragging: boolean
  isCopyMode: boolean
  isGenerationAvailable?: boolean
  uiState?: PromptNodeUiState | undefined
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'focus'): void
  (e: 'update:text', value: string): void
  (e: 'update:output-count', value: number): void
  (
    e: 'update:output-resolution',
    value: import('../../../../../core/contracts/generationContract').OutputResolution,
  ): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'update:input-images', value: PromptNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-open'): void
  (e: 'input-image-preview', value: PromptNodeItem['inputImages'][number]): void
  (e: 'input-image-remove', value: PromptNodeItem['inputImages'][number]): void
  (e: 'generate'): void
}>()

const { t } = useI18n()

const thumbsRef = ref<HTMLElement | null>(null)
const maxInputImages = 4
const outputCount = computed(() => requireImageNodeOutputCount(props.node.outputCount))
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const billingQuote = useCanvasBillingQuote(
  () => SERVICE.generation,
  () => props.isActive,
  () =>
    buildGenerationTaskRequest({
      prompt: '',
      aspectRatio: 'auto',
      outputCount: outputCount.value,
      outputResolution: props.node.outputResolution,
    }),
)
const {
  isAddSlotDragOver,
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
