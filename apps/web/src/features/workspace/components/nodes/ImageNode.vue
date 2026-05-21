<template>
  <div
    class="workflow-node image-node"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, 'is-expand-mode': !!expandModeActive }"
    :style="nodeStyle"
  >
    <NodeHeader
      class="image-node-header-shell"
      :class="{ 'is-hidden': !!expandModeActive }"
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
      <ImageNodeStage
        ref="imageStageRef"
        :node-id="node.id"
        :flow-node-id="flowNodeId"
        :stage-height="stageHeight"
        :display-image="displayImage"
        :input-locked="inputLocked"
        :empty-icon-variant="emptyIconVariant"
        :direct-generating="directGenerating"
        :expand-mode-active="expandModeActive"
        :expand-overlay-style="expandOverlayStyle"
        :expand-handle-radius="expandHandleRadius"
        :output-count="node.outputCount"
        :active="active"
        :dragging="dragging"
        @stage-drop="emit('stage-drop', $event)"
        @open-file="emit('open-file')"
        @open-preview="emit('open-preview')"
        @append-to-canvas="emit('append-to-canvas')"
        @reset="emit('reset')"
        @contextmenu="emit('contextmenu', $event)"
        @expand-submit="submitExpand"
        @update:output-count="selectOutputCount"
        @expand-handle-pointerdown="emit('expand-handle-pointerdown', $event)"
        @expand-handle-pointermove="emit('expand-handle-pointermove', $event)"
        @expand-handle-pointerup="emit('expand-handle-pointerup', $event)"
        @expand-frame-pointerdown="emit('expand-frame-pointerdown', $event)"
      />

      <HomeCanvasImageActionMenu
        v-if="showImageActionMenu"
        :show-strength-control="!!showStrengthControl"
        :direct-generating="!!directGenerating"
        :strength-pct="strengthPct ?? 50"
        :set-strength-track-ref="setStrengthTrackRef"
        @action="emit('action', $event)"
        @strength-action-mousedown="(action, event) => emit('strength-action-mousedown', action, event)"
        @strength-track-mousedown="emit('strength-track-mousedown', $event)"
      />
      <NodePromptComposer
        v-if="showPromptComposer"
        v-model="promptDraft"
        offset="52px"
        placeholder="输入提示词改造这张图"
        :disabled="!!directGenerating"
        :output-count="outputCountValue"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, type ComponentPublicInstance } from 'vue'
import {
  clampImageNodeOutputCount,
  type ImageNodeItem,
} from '../../types/canvasNodes'
import type { ImageNodeAction, ImageStrengthAction } from '../../types/imageActions'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import NodePromptComposer from './NodePromptComposer.vue'
import ImageNodeStage from './ImageNodeStage.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: ImageNodeItem
  flowNodeId?: string
  setImageStageRef?: (nodeId: string, element: Element | null) => void
  clearImageStageRef?: (nodeId: string, element?: Element | null) => void
  active: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string } | null
  emptyIconVariant?: 'image' | 'fusion'
  inputLocked?: boolean
  actionMenuVisible?: boolean
  promptComposerVisible?: boolean
  directGenerating?: boolean
  showStrengthControl?: boolean
  strengthPct?: number
  setStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
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
  (e: 'expand-submit', prompt: string, outputCount: number): void
  (e: 'action', action: ImageNodeAction): void
  (e: 'strength-action-mousedown', action: ImageStrengthAction, event: MouseEvent): void
  (e: 'strength-track-mousedown', event: MouseEvent): void
  (e: 'expand-handle-pointerdown', payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent }): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const emptyIconVariant = computed(() => props.emptyIconVariant ?? 'image')

const showImageActionMenu = computed(() => !!props.actionMenuVisible && !!props.displayImage)
const showPromptComposer = computed(
  () => !!props.promptComposerVisible && !!props.displayImage && !props.expandModeActive,
)
type StageExpose = { stageElement: HTMLElement | null }
const imageStageRef = ref<StageExpose | null>(null)
const promptDraft = ref('')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watch(
  [() => props.flowNodeId, imageStageRef, () => props.setImageStageRef, () => props.clearImageStageRef],
  ([flowNodeId, stageVm, setImageStageRef, clearImageStageRef], oldValues, onCleanup) => {
    const stageElement = stageVm?.stageElement ?? null
    const [prevFlowNodeId, prevStageVm] = oldValues ?? []
    const prevStageElement = prevStageVm?.stageElement ?? null
    if (prevFlowNodeId && prevFlowNodeId !== flowNodeId) {
      clearImageStageRef?.(prevFlowNodeId, prevStageElement)
    }
    if (flowNodeId) {
      setImageStageRef?.(flowNodeId, stageElement)
    }
    onCleanup(() => {
      if (flowNodeId) clearImageStageRef?.(flowNodeId, stageElement)
    })
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (!props.flowNodeId) return
  props.clearImageStageRef?.(props.flowNodeId, imageStageRef.value?.stageElement ?? null)
})

function submitPrompt(prompt: string, outputCount: number) {
  if (!prompt || props.directGenerating) return
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
}

function submitExpand(prompt: string, outputCount: number) {
  if (props.directGenerating) return
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
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

.image-node-header-shell.is-hidden {
  visibility: hidden;
  pointer-events: none;
}

.workflow-node.image-node.is-expand-mode .image-node-body {
  background: transparent;
}

.workflow-node.image-node.is-expand-mode .image-node-stage,
.workflow-node.image-node.is-expand-mode.active .image-node-stage {
  border-color: transparent;
}

</style>
