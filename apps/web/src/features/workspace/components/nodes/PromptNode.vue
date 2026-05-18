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
      <div ref="thumbsRef" class="prompt-node-thumbs node-tool-surface">
        <button
          v-for="item in sortableInputImages"
          :key="item.sourceNodeId"
          class="prompt-node-thumb prompt-node-thumb-item node-tool-output-count node-tool-button"
          type="button"
          :data-source-node-id="item.sourceNodeId"
          @mousedown.stop
          @contextmenu.stop.prevent="emit('input-image-preview', item)"
        >
          <img :src="item.url" alt="" draggable="false" />
        </button>
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
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
          >
            <path d="M12 5v14" />
            <path d="M5 12h14" />
          </svg>
        </button>
      </div>
      <textarea
        :value="node.text"
        class="workflow-node-input"
        placeholder="type your idea..."
        @input="onInput"
        @focus="emit('focus')"
        @contextmenu.stop
      />
      <div ref="actionsRef" class="prompt-node-actions node-tool-surface">
        <div class="image-node-output-count-wrap node-tool-output-count-wrap">
          <button
            class="image-node-output-count node-tool-output-count node-tool-button node-tool-tooltip-trigger prompt-node-aspect-trigger"
            type="button"
            :class="{ active: aspectRatioPickerVisible }"
            title="选择尺寸比例"
            aria-label="尺寸比例"
            @click.stop="toggleAspectRatioPicker"
          >
            <span class="prompt-node-aspect-icon" :data-ratio="aspectRatio">
              <span aria-hidden="true"></span>
            </span>
            <span class="node-tool-tooltip">
              {{ aspectRatio === 'auto' ? 'Auto' : aspectRatio }}
            </span>
          </button>
          <NodeToolPopover
            :visible="aspectRatioPickerVisible"
            layout="row"
            class="prompt-node-aspect-popover"
          >
            <button
              v-for="option in PROMPT_ASPECT_RATIO_OPTIONS"
              :key="option"
              type="button"
              class="prompt-node-aspect-option node-tool-tooltip-trigger"
              :class="{ active: option === aspectRatio }"
              @click.stop="selectAspectRatio(option)"
            >
              <span class="prompt-node-aspect-icon" :data-ratio="option">
                <span aria-hidden="true"></span>
              </span>
              <span class="node-tool-tooltip">
                {{ option === 'auto' ? 'Auto' : option }}
              </span>
            </button>
          </NodeToolPopover>
        </div>
        <div class="image-node-output-count-wrap node-tool-output-count-wrap">
          <button
            class="image-node-output-count node-tool-output-count node-tool-button"
            type="button"
            :class="{ active: outputCountPickerVisible }"
            title="选择张数"
            aria-label="输出张数"
            @click.stop="toggleOutputCountPicker"
          >
            {{ outputCount }}
          </button>
          <NodeToolPopover :visible="outputCountPickerVisible">
            <button
              v-for="option in outputCountPresetOptions"
              :key="option"
              type="button"
              class="image-node-output-count-option node-tool-output-count-option"
              :class="{ active: option === outputCount }"
              @click.stop="selectOutputCount(option)"
            >
              {{ option }}
            </button>
          </NodeToolPopover>
        </div>
        <button
          class="image-node-prompt-send node-tool-send node-tool-button"
          type="button"
          :disabled="!canGenerate"
          title="发送"
          @click="emit('generate')"
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
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useSortable } from '@vueuse/integrations/useSortable'
import {
  clampImageNodeOutputCount,
  IMAGE_OUTPUT_COUNT_PRESET_OPTIONS,
  type PromptNodeItem,
} from '../../types/canvasNodes'
import type { PromptNodeUiState } from '../../types/canvasNodeUi'
import NodeToolPopover from './NodeToolPopover.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'
import {
  PROMPT_ASPECT_RATIO_OPTIONS,
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
  (e: 'generate'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)

function onInput(event: Event) {
  emit('update:text', (event.target as HTMLTextAreaElement).value)
}
const thumbsRef = ref<HTMLElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const sortableInputImages = ref<PromptNodeItem['inputImages']>([])
const addSlotDragOver = ref(false)
const outputCountPickerVisible = ref(false)
const aspectRatioPickerVisible = ref(false)
const maxInputImages = 4
const outputCount = computed(() => clampImageNodeOutputCount(props.node.outputCount))
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const visibleInputImages = computed(() => props.node.inputImages.slice(0, maxInputImages))
const canAcceptMoreImages = computed(() => visibleInputImages.value.length < maxInputImages)
const canGenerate = computed(() => props.node.text.trim().length > 0)
const outputCountPresetOptions = IMAGE_OUTPUT_COUNT_PRESET_OPTIONS
watch(
  visibleInputImages,
  (items) => {
    sortableInputImages.value = [...items]
  },
  { immediate: true },
)
useSortable(() => thumbsRef.value, sortableInputImages, {
  animation: 150,
  draggable: '.prompt-node-thumb-item',
  filter: '.prompt-node-thumb-add',
  ghostClass: 'is-sortable-ghost',
  chosenClass: 'is-sortable-chosen',
  dragClass: 'is-sortable-drag',
  fallbackClass: 'prompt-node-sortable-fallback',
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  preventOnFilter: false,
  onEnd() {
    const sourceNodeIds = Array.from(
      thumbsRef.value?.querySelectorAll<HTMLElement>('.prompt-node-thumb-item') ?? [],
    )
      .map((item) => item.dataset.sourceNodeId ?? '')
      .filter(Boolean)
    if (sourceNodeIds.length !== visibleInputImages.value.length) return
    const byId = new Map(visibleInputImages.value.map((item) => [item.sourceNodeId, item]))
    const next = sourceNodeIds
      .map((sourceNodeId) => byId.get(sourceNodeId))
      .filter((item): item is PromptNodeItem['inputImages'][number] => !!item)
    if (next.length !== visibleInputImages.value.length) return
    const same = next.every(
      (item, index) => item.sourceNodeId === visibleInputImages.value[index]?.sourceNodeId,
    )
    if (same) return
    sortableInputImages.value = next
    emit('update:input-images', next)
  },
} as Parameters<typeof useSortable<PromptNodeItem['inputImages'][number]>>[2])
function toggleOutputCountPicker() {
  const nextVisible = !outputCountPickerVisible.value
  outputCountPickerVisible.value = nextVisible
  if (nextVisible) aspectRatioPickerVisible.value = false
}
function selectOutputCount(value: number) {
  outputCountPickerVisible.value = false
  emit('update:output-count', clampImageNodeOutputCount(value))
}
function toggleAspectRatioPicker() {
  const nextVisible = !aspectRatioPickerVisible.value
  aspectRatioPickerVisible.value = nextVisible
  if (nextVisible) outputCountPickerVisible.value = false
}
function selectAspectRatio(value: PromptAspectRatio) {
  aspectRatioPickerVisible.value = false
  emit('update:aspect-ratio', value)
}
function closeToolPopovers() {
  outputCountPickerVisible.value = false
  aspectRatioPickerVisible.value = false
}
function onGlobalPointerDown(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (actionsRef.value?.contains(target)) return
  closeToolPopovers()
}
onMounted(() => {
  window.addEventListener('pointerdown', onGlobalPointerDown, true)
})
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onGlobalPointerDown, true)
})
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
  aspect-ratio: 1 / 1;
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  background: #ffffff;
  overflow: visible;
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
  padding: 58px 13px 48px;
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
.prompt-node-thumbs {
  --node-tool-button-size: 46px;
  --node-tool-button-radius: 8px;
  position: absolute;
  left: 10px;
  right: 10px;
  top: 10px;
  display: flex;
  gap: 6px;
  overflow: visible;
  z-index: 3;
}
.prompt-node-thumb {
  position: relative;
  flex: 0 0 var(--node-tool-button-size);
  cursor: grab;
  overflow: hidden;
  padding: 0;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease,
    opacity 0.16s ease;
}
.prompt-node-thumb:hover,
.prompt-node-thumb:focus,
.prompt-node-thumb.is-drop-target,
.prompt-node-thumb.is-sortable-chosen {
  border-color: var(--node-tool-button-accent);
  background: #f8f7ff;
  color: #272932;
}
.prompt-node-thumb.is-sortable-chosen,
.prompt-node-thumb.is-sortable-drag {
  cursor: grabbing;
}
.prompt-node-thumb.is-sortable-ghost {
  opacity: 0.45;
}
.prompt-node-thumb img {
  width: 100%;
  height: 100%;
  display: block;
  border-radius: calc(var(--node-tool-button-radius) - 1px);
  object-fit: cover;
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
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: var(--node-tool-button-gap);
}
.prompt-node-actions.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}
.prompt-node-aspect-trigger {
  flex: 0 0 var(--node-tool-button-size);
  position: relative;
}
.prompt-node-aspect-icon {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
}
.prompt-node-aspect-icon > span {
  display: grid;
  place-items: center;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  color: #94a3b8;
  width: 20px;
  height: 20px;
  transition:
    border-color 0.16s ease,
    color 0.16s ease;
}
.prompt-node-aspect-icon[data-ratio='3:4'] > span {
  width: 17px;
  height: 22px;
}
.prompt-node-aspect-icon[data-ratio='9:16'] > span {
  width: 13px;
  height: 22px;
}
.prompt-node-aspect-icon[data-ratio='4:3'] > span {
  width: 22px;
  height: 17px;
}
.prompt-node-aspect-icon[data-ratio='16:9'] > span {
  width: 22px;
  height: 13px;
}
.prompt-node-aspect-popover {
  right: 0;
}
.prompt-node-aspect-option {
  position: relative;
  width: var(--node-tool-button-size);
  height: var(--node-tool-popover-option-height);
  border: 0;
  border-radius: 0;
  background: transparent;
  color: #5f6878;
  cursor: pointer;
  padding: 0;
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    color 0.16s ease;
}
.prompt-node-aspect-option:hover,
.prompt-node-aspect-option.active {
  color: var(--node-tool-button-accent);
}
.prompt-node-aspect-option:hover .prompt-node-aspect-icon > span,
.prompt-node-aspect-option.active .prompt-node-aspect-icon > span,
.prompt-node-aspect-trigger:hover .prompt-node-aspect-icon > span,
.prompt-node-aspect-trigger.active .prompt-node-aspect-icon > span {
  border-color: var(--node-tool-button-accent);
  color: var(--node-tool-button-accent);
}
</style>
