<template>
  <div
    class="workflow-node fusion-node"
    data-node-kind="fusion"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'FUSION'"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M7 7h4l2 2h4a2 2 0 0 1 2 2v6H7a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z" />
          <path d="M9 15c1.4-1.4 4.6-1.4 6 0" />
        </svg>
      </template>
    </NodeHeader>

    <div class="fusion-node-body" @contextmenu.stop>
      <div
        class="fusion-node-stage node-tool-surface"
        data-flow-handle-frame
        :class="{ 'is-drop-target': addSlotDragOver }"
        @dragenter.stop.prevent="onAddSlotDragOver"
        @dragover.stop.prevent="onAddSlotDragOver"
        @dragleave.stop="onAddSlotDragLeave"
        @drop.stop.prevent="onAddSlotDrop"
        @contextmenu.prevent.stop="onStageContextMenu"
      >
        <svg
          v-if="connectionPaths.length"
          class="fusion-node-connection-layer"
          viewBox="0 0 100 100"
          aria-hidden="true"
        >
          <path
            v-for="(d, idx) in connectionPaths"
            :key="idx"
            :d="d"
          />
        </svg>
        <div ref="thumbsRef" class="fusion-node-slots">
          <template v-for="(slot, index) in fusionSlotItems" :key="slot.key">
            <div
              v-if="slot.image"
              class="fusion-node-slot fusion-node-slot-item"
              :class="`fusion-node-slot--${getSlotPositionClass(index)}`"
              role="button"
              tabindex="0"
              :data-image-id="slot.image.id"
              @mousedown.stop
            >
              <img :src="slot.image.url" alt="" draggable="false" />
              <button
                class="fusion-node-slot-remove"
                type="button"
                title="移除图片输入"
                aria-label="移除图片输入"
                @mousedown.stop
                @pointerdown.stop
                @click.stop="removeInputImage(slot.image.id)"
              >
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  stroke-linecap="round"
                >
                  <path d="M6 6l12 12" />
                  <path d="M18 6L6 18" />
                </svg>
              </button>
            </div>
            <div
              v-else
              class="fusion-node-slot fusion-node-slot-placeholder"
              :class="`fusion-node-slot--${getSlotPositionClass(index)}`"
              aria-hidden="true"
            />
          </template>
        </div>
      </div>
      <form
        v-if="showPromptComposer"
        class="image-node-prompt-composer node-tool-surface"
        :class="{ 'count-stacked': shouldStackOutputCount, 'has-text': promptDraft.trim() }"
        @submit.prevent="submitPrompt"
        @mousedown.stop
        @click.stop
        @contextmenu.stop.prevent
      >
        <textarea
          ref="promptInputRef"
          v-model="promptDraft"
          class="image-node-prompt-input"
          rows="1"
          placeholder="输入融合提示词"
          :disabled="isGenerating"
          @keydown.enter.exact="onPromptEnter"
        />
        <div ref="actionsRef" class="image-node-prompt-actions">
          <NodeOutputCountPicker
            :model-value="outputCountValue"
            :visible="outputCountPickerVisible"
            :disabled="isGenerating"
            title="选择张数"
            aria-label="输出张数"
            @toggle="toggleOutputCountPicker"
            @select="selectOutputCount"
          />
          <button
            class="image-node-prompt-send node-tool-send node-tool-button"
            type="submit"
            :disabled="isGenerating || !canGenerate || !promptDraft.trim()"
            title="发送"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.1"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M5 12h13" />
              <path d="m13 6 6 6-6 6" />
            </svg>
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useSortable } from '@vueuse/integrations/useSortable'
import {
  clampImageNodeOutputCount,
  type FusionNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'
import { useAutoResizeTextarea } from './composables/useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from './composables/useNodeToolPopoverGroup'

const props = defineProps<{
  node: FusionNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  promptComposerVisible?: boolean
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:output-count', value: number): void
  (e: 'update:prompt', value: string): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-preview', value: FusionNodeItem['inputImages'][number]): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number): void
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const thumbsRef = ref<HTMLElement | null>(null)
const promptInputRef = ref<HTMLTextAreaElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const sortableInputImages = ref<FusionNodeItem['inputImages']>([])
const addSlotDragOver = ref(false)
const promptDraft = ref('')
const shouldStackOutputCount = ref(false)
const maxInputImages = 4
const visibleInputImages = computed(() => props.node.inputImages.slice(0, maxInputImages))
const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxInputImages)
const canGenerate = computed(() => visibleInputImages.value.length >= 2)
const showPromptComposer = computed(() => !!props.promptComposerVisible)
const isGenerating = computed(() => !!props.uiState?.isRunning)
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))
const toolPopovers = useNodeToolPopoverGroup({
  keys: ['output-count'] as const,
  rootRefs: [actionsRef],
})
const outputCountPickerVisible = toolPopovers.isOpen('output-count')
const promptTextarea = useAutoResizeTextarea(promptInputRef, {
  watchSource: promptDraft,
  onResize: ({ naturalHeight, singleLineHeight }) => {
    shouldStackOutputCount.value = naturalHeight > singleLineHeight + 2
  },
})
const fusionSlotItems = computed(() =>
  Array.from({ length: maxInputImages }, (_, index) => {
    const image = sortableInputImages.value[index] ?? null
    return {
      key: image?.id ?? `empty-${index}`,
      image,
      position: index + 1,
    }
  }),
)

const imageCount = computed(() => Math.min(visibleInputImages.value.length || 0, maxInputImages))

function getSlotPositionClass(index: number): string {
  const count = imageCount.value || 4
  if (count === 1) return index === 0 ? 'center' : ['top-left', 'top-right', 'bottom-right'][index - 1]
  if (count === 2) return ['top-left', 'bottom-right', 'top-right', 'bottom-left'][index]
  if (count === 3) return ['top-center', 'bottom-left', 'bottom-right', 'top-left'][index]
  return ['top-left', 'top-right', 'bottom-left', 'bottom-right'][index]
}

const connectionPaths = computed(() => {
  const count = imageCount.value || 4
  if (count <= 1) return []
  if (count === 2) return ['M23 23 L77 77']
  if (count === 3) return ['M50 23 L23 77 L77 77 Z']
  return ['M23 23 L77 23 L77 77 L23 77 Z', 'M23 23 L77 77', 'M77 23 L23 77']
})

watch(
  () => props.node.prompt,
  (value) => {
    promptDraft.value = typeof value === 'string' ? value : ''
    promptTextarea.resizeAfterRender()
  },
  { immediate: true },
)

watch(showPromptComposer, async (visible) => {
  if (!visible) {
    closeOutputCountPicker()
    return
  }
  await nextTick()
  promptTextarea.resize()
  promptInputRef.value?.focus()
})

function finishThumbReorder() {
  void nextTick(() => {
    sortableInputImages.value = [...visibleInputImages.value]
  })
}

watch(
  visibleInputImages,
  (items) => {
    const same =
      sortableInputImages.value.length === items.length &&
      sortableInputImages.value.every((item, index) => item.id === items[index]?.id)
    if (same) return
    sortableInputImages.value = [...items]
  },
  { immediate: true },
)

useSortable(() => thumbsRef.value, sortableInputImages, {
  animation: 150,
  draggable: '.fusion-node-slot-item',
  filter: '.fusion-node-slot-remove',
  ghostClass: 'is-sortable-ghost',
  chosenClass: 'is-sortable-chosen',
  dragClass: 'is-sortable-drag',
  fallbackClass: 'fusion-node-sortable-fallback',
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  preventOnFilter: false,
  onEnd() {
    const imageIds = Array.from(
      thumbsRef.value?.querySelectorAll<HTMLElement>('.fusion-node-slot-item') ?? [],
    )
      .map((item) => item.dataset.imageId ?? '')
      .filter(Boolean)
    if (imageIds.length !== visibleInputImages.value.length) {
      finishThumbReorder()
      return
    }
    const byId = new Map(visibleInputImages.value.map((item) => [item.id, item]))
    const next = imageIds
      .map((imageId) => byId.get(imageId))
      .filter((item): item is FusionNodeItem['inputImages'][number] => !!item)
    if (next.length !== visibleInputImages.value.length) {
      finishThumbReorder()
      return
    }
    const same = next.every((item, index) => item.id === visibleInputImages.value[index]?.id)
    if (same) {
      finishThumbReorder()
      return
    }
    sortableInputImages.value = next
    emit('update:input-images', next)
    finishThumbReorder()
  },
} as Parameters<typeof useSortable<FusionNodeItem['inputImages'][number]>>[2])

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

function onStageContextMenu(event: MouseEvent) {
  emit('contextmenu', event)
}

function submitPrompt() {
  const prompt = promptDraft.value.trim()
  if (!prompt || isGenerating.value || !canGenerate.value) return
  const outputCount = outputCountValue.value
  emit('update:prompt', prompt)
  closeOutputCountPicker()
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
  promptTextarea.resizeAfterRender()
}

function onPromptEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  submitPrompt()
}

function toggleOutputCountPicker() {
  if (isGenerating.value) return
  toolPopovers.toggle('output-count')
}

function closeOutputCountPicker() {
  toolPopovers.close('output-count')
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
  closeOutputCountPicker()
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.workflow-node.fusion-node {
  overflow: visible;
}

.fusion-node-body {
  position: relative;
  padding: 0;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-radius: 11px;
  background: #ffffff;
  overflow: visible;
}

.fusion-node-stage {
  --fusion-line-color: rgba(146, 120, 230, 0.44);
  --fusion-slot-size: clamp(58px, 29%, 88px);
  --fusion-slot-inset: clamp(18px, 10%, 34px);
  position: relative;
  width: 100%;
  aspect-ratio: 1;
  min-height: 280px;
  border: 1.5px solid #e2e8f0;
  border-radius: 11px;
  background: #ffffff;
  box-sizing: border-box;
  transition: border-color 0.2s ease;
  overflow: hidden;
}

.fusion-node-connection-layer {
  position: absolute;
  inset: 0;
  z-index: 1;
  fill: none;
  stroke: var(--fusion-line-color);
  stroke-width: 1.05;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
  transition: stroke 0.2s ease;
}

.fusion-node-slots {
  position: absolute;
  inset: 0;
  z-index: 2;
}

.workflow-node.fusion-node.active .fusion-node-stage {
  border-color: #9278e6;
}

.workflow-node.fusion-node.active .fusion-node-connection-layer {
  stroke: rgba(146, 120, 230, 0.62);
}

.fusion-node-stage.is-drop-target {
  border-color: #9278e6;
}

.fusion-node-stage.is-drop-target .fusion-node-connection-layer {
  stroke: rgba(146, 120, 230, 0.62);
}

.fusion-node-slot {
  position: absolute;
  width: var(--fusion-slot-size);
  height: var(--fusion-slot-size);
  border: 1.5px solid #e2e8f0;
  border-radius: 10px;
  background: #ffffff;
  overflow: hidden;
  padding: 0;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease,
    opacity 0.16s ease;
}

.fusion-node-slot--top-left {
  top: var(--fusion-slot-inset);
  left: var(--fusion-slot-inset);
}

.fusion-node-slot--top-right {
  top: var(--fusion-slot-inset);
  right: var(--fusion-slot-inset);
}

.fusion-node-slot--top-center {
  top: var(--fusion-slot-inset);
  left: calc(50% - var(--fusion-slot-size) / 2);
}

.fusion-node-slot--bottom-left {
  bottom: var(--fusion-slot-inset);
  left: var(--fusion-slot-inset);
}

.fusion-node-slot--bottom-right {
  bottom: var(--fusion-slot-inset);
  right: var(--fusion-slot-inset);
}

.fusion-node-slot--center {
  top: calc(50% - var(--fusion-slot-size) / 2);
  left: calc(50% - var(--fusion-slot-size) / 2);
}

.fusion-node-slot-item {
  cursor: grab;
}

.fusion-node-slot:hover,
.fusion-node-slot:focus-visible,
.fusion-node-slot.is-sortable-chosen {
  border-color: #9278e6;
  background: #f8f7ff;
  color: #272932;
}

.fusion-node-slot.is-sortable-chosen,
.fusion-node-slot.is-sortable-drag {
  cursor: grabbing;
}

.fusion-node-slot.is-sortable-ghost {
  opacity: 0.45;
}

.fusion-node-slot img {
  width: 100%;
  height: 100%;
  display: block;
  object-fit: cover;
  pointer-events: none;
}

.fusion-node-slot-remove {
  position: absolute;
  top: 5px;
  right: 5px;
  width: 18px;
  height: 18px;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.96);
  color: #64748b;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  opacity: 0;
  transform: scale(0.92);
  transition:
    opacity 0.14s ease,
    transform 0.14s ease,
    color 0.14s ease,
    border-color 0.14s ease;
}

.fusion-node-slot:hover .fusion-node-slot-remove,
.fusion-node-slot:focus-within .fusion-node-slot-remove {
  opacity: 1;
  transform: scale(1);
}

:global(.fusion-node-sortable-fallback) {
  opacity: 0 !important;
}

.fusion-node-slot-placeholder {
  opacity: 0;
  pointer-events: none;
}

.image-node-prompt-composer {
  position: absolute;
  left: 0;
  right: 0;
  top: calc(100% + 8px);
  z-index: 42;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: var(--node-tool-button-gap);
  padding: 5px 5px 5px 13px;
  border: 1px solid rgba(122, 107, 213, 0.28);
  border-radius: 13px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  animation: image-node-prompt-rise 0.16s ease-out;
}

.image-node-prompt-composer.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}

.image-node-prompt-composer.has-text {
  border-color: #9278e6;
}

.image-node-prompt-composer.count-stacked {
  min-height: 73px;
}

.image-node-prompt-input {
  align-self: end;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: #272932;
  cursor: text;
  font-size: 12px;
  line-height: 18px;
  resize: none;
  overflow: hidden;
  padding: 6px 0;
}

.image-node-prompt-composer.count-stacked .image-node-prompt-input {
  min-height: 73px;
}

.image-node-prompt-input::placeholder {
  color: #9aa4b2;
}

.image-node-prompt-input:disabled {
  cursor: wait;
}

.image-node-prompt-actions {
  align-self: end;
  display: flex;
  align-items: center;
  gap: var(--node-tool-button-gap);
}

.image-node-prompt-composer.count-stacked .image-node-prompt-actions {
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

@keyframes image-node-prompt-rise {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
