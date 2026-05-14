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
          <div v-if="!props.inputLocked" class="image-node-stage-actions">
            <NodeActionButton variant="stage-ghost" @click="emit('reset')">清空</NodeActionButton>
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
        class="image-node-prompt-composer"
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
          @input="resizePromptInput"
          @keydown.enter.exact="onPromptEnter"
        />
        <button
          class="image-node-prompt-send"
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
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import type { ImageNodeItem } from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

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
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string): void
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

watch(showPromptComposer, async (visible) => {
  if (!visible) return
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
  emit('prompt-generate', prompt)
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
  const maxHeight = 108
  input.style.height = 'auto'
  input.style.height = `${Math.min(input.scrollHeight, maxHeight)}px`
  input.style.overflowY = input.scrollHeight > maxHeight ? 'auto' : 'hidden'
}
</script>

<style scoped src="./styles/node-shared.css"></style>
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
  isolation: isolate;
}

.image-node-stage.is-loading::after {
  content: '';
  position: absolute;
  inset: -48px;
  z-index: 0;
  border-radius: 42% 58% 48% 52% / 54% 42% 58% 46%;
  background:
    radial-gradient(circle at 50% 50%, rgba(255, 255, 255, 0.08) 0 24%, transparent 38%),
    repeating-conic-gradient(
      from 12deg at 50% 50%,
      transparent 0deg 18deg,
      rgba(239, 68, 68, 0.38) 22deg 44deg,
      rgba(244, 114, 182, 0.3) 50deg 64deg,
      transparent 70deg 91deg,
      rgba(146, 120, 230, 0.36) 96deg 122deg,
      rgba(239, 68, 68, 0.22) 128deg 142deg,
      transparent 148deg 180deg
    );
  filter: blur(18px);
  opacity: 0.92;
  animation:
    image-node-loading-aura-flow 3.8s linear infinite,
    image-node-loading-aura-wave 5.4s ease-in-out infinite;
  pointer-events: none;
  -webkit-mask: radial-gradient(ellipse at center, transparent 0 24%, #000 38% 78%, transparent 86%);
  mask: radial-gradient(
    ellipse at center,
    transparent 0 24%,
    #000 38% 78%,
    transparent 86%
  );
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
  animation: image-node-logo-spin-cycle 4s linear infinite;
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
  z-index: 7;
  display: grid;
  grid-template-columns: 1fr 32px;
  align-items: end;
  gap: 7px;
  padding: 5px 5px 5px 13px;
  border: 1px solid rgba(122, 107, 213, 0.28);
  border-radius: 13px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
  animation: image-node-prompt-rise 0.16s ease-out;
}

.image-node-prompt-input {
  min-width: 0;
  max-height: 108px;
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

.image-node-prompt-input::placeholder {
  color: #9aa4b2;
}

.image-node-prompt-input:disabled {
  cursor: wait;
}

.image-node-prompt-send {
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #ffffff;
  background: #7a6bd5;
  cursor: pointer;
  transition:
    transform 0.16s ease,
    opacity 0.16s ease,
    background-color 0.16s ease;
}

.image-node-prompt-send:hover:not(:disabled) {
  transform: translateX(1px);
  background: #6f61c6;
}

.image-node-prompt-send:disabled {
  cursor: not-allowed;
  opacity: 0.45;
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
    transform: rotate(3turn);
  }
  100% {
    transform: rotate(3turn);
  }
}

@keyframes image-node-loading-aura-flow {
  to {
    transform: rotate(360deg);
  }
}

@keyframes image-node-loading-aura-wave {
  0%,
  100% {
    border-radius: 42% 58% 48% 52% / 54% 42% 58% 46%;
  }
  50% {
    border-radius: 56% 44% 60% 40% / 42% 58% 44% 56%;
  }
}
</style>
