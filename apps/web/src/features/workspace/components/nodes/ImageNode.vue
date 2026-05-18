<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'IMAGE'"
      rounded
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
          stroke-width="2.2"
        >
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <circle cx="8.5" cy="8.5" r="1.5" />
          <path d="M3 14l4-4 5 5 5-5 4 4" />
        </svg>
      </template>
    </NodeHeader>
    <div class="image-node-body" @contextmenu.stop>
      <div
        class="image-node-stage"
        data-flow-handle-frame
        :data-image-node-id="node.id"
        :class="{ 'is-loading': props.directGenerating && !displayImage }"
        :style="{ height: `${stageHeight}px` }"
        @dragover.stop="onStageDragOver"
        @drop.stop="onStageDrop"
        @contextmenu.prevent.stop="onStageContextMenu"
      >
        <template v-if="displayImage">
          <img
            :src="stageDisplayUrl"
            class="image-node-stage-image"
            draggable="false"
            @dblclick="emit('append-to-canvas')"
          />
          <div class="image-node-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('open-preview')">预览</NodeActionButton>
            <NodeActionButton v-if="!props.inputLocked" variant="stage-ghost" @click="emit('reset')">
              清空
            </NodeActionButton>
          </div>
        </template>
        <div
          v-if="!displayImage"
          class="image-node-stage-empty"
          :class="{ 'is-readonly': props.inputLocked, 'is-loading': props.directGenerating }"
          @click="onEmptyStateClick"
        >
          <svg
            v-if="props.directGenerating"
            class="image-node-stage-loader-logo"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
          >
            <rect x="4" y="4" width="16" height="16" rx="3" />
            <circle cx="15.5" cy="8.5" r="1.5" />
            <path d="M5 16l4.5-4.5 4 4 2-2L19 17" />
          </svg>
          <svg
            v-else
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
          >
            <rect x="4" y="4" width="16" height="16" rx="3" />
            <circle cx="15.5" cy="8.5" r="1.5" />
            <path d="M5 16l4.5-4.5 4 4 2-2L19 17" />
          </svg>
          <span v-if="!props.directGenerating">{{ emptyStateLabel }}</span>
        </div>
      </div>

      <HomeCanvasImageActionMenu
        v-if="showImageActionMenu"
        :show-style-strength-control="!!showStyleStrengthControl"
        :direct-generating="!!directGenerating"
        :display-height="stageHeight"
        :style-innovate-strength-pct="styleInnovateStrengthPct ?? 50"
        :set-style-strength-track-ref="setStyleStrengthTrackRef"
        @action-remove-bg="emit('action', 'remove-bg')"
        @style-innovate-mousedown="emit('style-innovate-mousedown', $event)"
        @style-innovate-click="emit('action', 'style-innovate')"
        @action-to-vector="emit('action', 'to-vector')"
        @action-sketch-to-design="emit('action', 'sketch-to-design')"
        @style-strength-track-mousedown="emit('style-strength-track-mousedown', $event)"
      />
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
          placeholder="输入提示词改造这张图"
          :disabled="directGenerating"
          @keydown.enter.exact="onPromptEnter"
        />
        <div ref="actionsRef" class="image-node-prompt-actions">
          <NodeOutputCountPicker
            :model-value="outputCountValue"
            :visible="outputCountPickerVisible"
            :disabled="directGenerating"
            title="选择张数"
            aria-label="输出张数"
            @toggle="toggleOutputCountPicker"
            @select="selectOutputCount"
          />
          <button
            class="image-node-prompt-send node-tool-send node-tool-button"
            type="submit"
            :disabled="directGenerating || !promptDraft.trim()"
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
import { computed, nextTick, ref, watch, type ComponentPublicInstance } from 'vue'
import {
  clampImageNodeOutputCount,
  type ImageNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'
import { useNodeToolPopoverGroup } from './composables/useNodeToolPopoverGroup'

const props = defineProps<{
  node: ImageNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
  displayMode?: 'source' | 'node-output' | 'linked-output' | 'mask' | 'pending'
  inputLocked?: boolean
  uiState?: NodeUiState
  actionMenuVisible?: boolean
  promptComposerVisible?: boolean
  directGenerating?: boolean
  showStyleStrengthControl?: boolean
  styleInnovateStrengthPct?: number
  setStyleStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'open-preview'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number): void
  (e: 'action', action: 'remove-bg' | 'style-innovate' | 'to-vector' | 'sketch-to-design'): void
  (e: 'style-innovate-mousedown', event: MouseEvent): void
  (e: 'style-strength-track-mousedown', event: MouseEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const sourceDisplayUrl = computed(() => props.displayImage?.url ?? null)
const { displayUrl: stageDisplayUrl } = useDisplayImagePreview(sourceDisplayUrl, { maxSide: 1280 })
const emptyStateLabel = computed(() => {
  if (props.inputLocked) return 'Waiting...'
  return '上传或拖拽图片作为输入'
})

const showImageActionMenu = computed(() => !!props.actionMenuVisible && !!props.displayImage)
const showPromptComposer = computed(() => !!props.promptComposerVisible && !!props.displayImage)
const promptDraft = ref('')
const promptInputRef = ref<HTMLTextAreaElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const shouldStackOutputCount = ref(false)
const toolPopovers = useNodeToolPopoverGroup({
  keys: ['output-count'] as const,
  rootRefs: [actionsRef],
})
const outputCountPickerVisible = toolPopovers.isOpen('output-count')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watch(showPromptComposer, async (visible) => {
  if (!visible) {
    closeOutputCountPicker()
    return
  }
  await nextTick()
  resizePromptInput()
  promptInputRef.value?.focus()
})

watch(promptDraft, async () => {
  await nextTick()
  resizePromptInput()
})

function onStageDragOver(event: DragEvent) {
  event.preventDefault()
}

function onStageDrop(event: DragEvent) {
  event.preventDefault()
  if (props.inputLocked) return
  emit('stage-drop', event)
}

function onEmptyStateClick() {
  if (props.inputLocked) return
  emit('open-file')
}

function onStageContextMenu(event: MouseEvent) {
  if (!props.displayImage) return
  emit('contextmenu', event)
}

function submitPrompt() {
  const prompt = promptDraft.value.trim()
  if (!prompt || props.directGenerating) return
  const outputCount = outputCountValue.value
  closeOutputCountPicker()
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
  void nextTick(resizePromptInput)
}

function onPromptEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  submitPrompt()
}

function resizePromptInput() {
  const input = promptInputRef.value
  if (!input) return
  const singleLineHeight = resolvePromptSingleLineHeight(input)
  const previousMinHeight = input.style.minHeight
  input.style.minHeight = '0'
  input.style.height = 'auto'
  const naturalHeight = input.scrollHeight
  input.style.minHeight = previousMinHeight
  input.style.height = `${naturalHeight}px`
  input.style.overflowY = 'hidden'
  shouldStackOutputCount.value = naturalHeight > singleLineHeight + 2
}

function toggleOutputCountPicker() {
  if (props.directGenerating) return
  toolPopovers.toggle('output-count')
}

function closeOutputCountPicker() {
  toolPopovers.close('output-count')
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
  closeOutputCountPicker()
}

function resolvePromptSingleLineHeight(input: HTMLTextAreaElement): number {
  const style = window.getComputedStyle(input)
  const lineHeight = Number.parseFloat(style.lineHeight)
  const paddingTop = Number.parseFloat(style.paddingTop)
  const paddingBottom = Number.parseFloat(style.paddingBottom)
  return (
    (Number.isFinite(lineHeight) ? lineHeight : 18) +
    (Number.isFinite(paddingTop) ? paddingTop : 0) +
    (Number.isFinite(paddingBottom) ? paddingBottom : 0)
  )
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.workflow-node.image-node {
  overflow: visible;
}

.image-node-body {
  position: relative;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-radius: 11px;
  background: #ffffff;
  overflow: visible;
}

.image-node-stage-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: #fff;
  cursor: grab;
  user-select: none;
}

.workflow-node.image-node.dragging .image-node-stage-image {
  cursor: grabbing;
}

.image-node-stage-image.preview {
  pointer-events: none;
}

.image-node-stage {
  position: relative;
  width: 100%;
  border: 1.5px solid #e2e8f0;
  border-radius: 11px;
  background: #ffffff;
  box-sizing: border-box;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.workflow-node.image-node.active .image-node-stage {
  border-color: #9278e6;
}

.image-node-stage.is-loading {
  border-color: #e2e8f0;
  box-shadow:
    0 0 30px rgba(239, 68, 68, 0.2),
    0 0 48px rgba(146, 120, 230, 0.18);
}

.image-node-stage-empty {
  position: relative;
  z-index: 2;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 11px;
  background: #ffffff;
  color: #a8b3be;
  gap: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}

.image-node-stage-empty:hover {
  background: #ffffff;
  color: #8894a5;
}

.image-node-stage-empty svg {
  width: 84px;
  height: 84px;
  stroke-width: 0.85;
}

.image-node-stage-empty span {
  display: none;
}

.image-node-stage-empty.is-readonly {
  cursor: default;
}

.image-node-stage-empty.is-loading {
  background: rgba(255, 255, 255, 0.56);
  color: #a8b3be;
  cursor: wait;
}

.image-node-stage-loader-logo {
  width: 84px;
  height: 84px;
  stroke-width: 0.85;
  animation: image-node-logo-spin-cycle 2s linear infinite;
}

.image-node-stage-actions {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: flex;
  gap: 6px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.16s ease;
}

.image-node-stage:hover .image-node-stage-actions {
  opacity: 1;
  pointer-events: auto;
}

.image-node-prompt-composer {
  position: absolute;
  left: 0;
  right: 0;
  top: calc(100% + 52px);
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

.image-node-output-count-input {
  text-align: center;
  appearance: textfield;
}

.image-node-output-count-input::-webkit-outer-spin-button,
.image-node-output-count-input::-webkit-inner-spin-button {
  margin: 0;
  appearance: none;
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

@keyframes image-node-logo-spin-cycle {
  0% {
    transform: rotate(0deg);
  }
  50% {
    transform: rotate(1turn);
  }
  100% {
    transform: rotate(1turn);
  }
}

</style>
