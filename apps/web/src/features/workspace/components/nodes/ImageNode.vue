<template>
  <div
    class="workflow-node image-node overflow-visible"
    data-node-kind="image"
    :data-node-id="node.id"
    :class="{
      active,
      dragging,
      'copy-mode': copyMode,
      'is-expand-mode': !!expandModeActive,
      'is-mask-editor-mode': !!maskEditorModeActive,
    }"
    :style="nodeStyle"
  >
    <NodeHeader
      :class="{ 'invisible pointer-events-none': !!expandModeActive || !!maskEditorModeActive }"
      :title="node.title || 'Image'"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <MediaImage />
      </template>
    </NodeHeader>
    <div
      class="relative flex flex-col gap-2 overflow-visible rounded-[11px] p-0"
      :class="expandModeActive || maskEditorModeActive ? 'bg-transparent' : 'bg-white'"
      @contextmenu.stop
    >
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
        :mask-editor-mode-active="maskEditorModeActive"
        :mask-editor-overlay-style="maskEditorOverlayStyle"
        :mask-editor-canvas-width="maskEditorCanvasWidth"
        :mask-editor-canvas-height="maskEditorCanvasHeight"
        :mask-layer-opacity="maskLayerOpacity"
        :image-layer-visible="imageLayerVisible"
        :mask-editor-brush-size="maskEditorBrushSize"
        :mask-editor-brush-shape="maskEditorBrushShape"
        :set-mask-canvas-ref="setMaskCanvasRef"
        :output-count="node.outputCount"
        :active="active"
        :dragging="dragging"
        @stage-drop="emit('stage-drop', $event)"
        @open-file="emit('open-file')"
        @open-preview="emit('open-preview')"
        @append-to-canvas="emit('append-to-canvas')"
        @reset="emit('reset')"
        @contextmenu="onImageStageContextMenu"
        @expand-submit="submitExpand"
        @update:output-count="selectOutputCount"
        @expand-handle-pointerdown="emit('expand-handle-pointerdown', $event)"
        @expand-handle-pointermove="emit('expand-handle-pointermove', $event)"
        @expand-handle-pointerup="emit('expand-handle-pointerup', $event)"
        @expand-frame-pointerdown="emit('expand-frame-pointerdown', $event)"
        @mask-canvas-pointerdown="emit('mask-canvas-pointerdown', $event)"
      />

      <HomeCanvasImageActionMenu
        v-if="showImageActionMenu"
        :show-strength-control="!!showStrengthControl"
        :direct-generating="!!directGenerating"
        :strength-pct="strengthPct ?? 50"
        :set-strength-track-ref="setStrengthTrackRef"
        @action="emit('action', $event)"
        @strength-action-mousedown="
          (action, event) => emit('strength-action-mousedown', action, event)
        "
        @strength-track-mousedown="emit('strength-track-mousedown', $event)"
      />
      <NodePromptComposer
        v-if="showPromptComposer"
        v-model="promptDraft"
        offset="52px"
        :placeholder="props.promptPlaceholder ?? ''"
        :disabled="!!directGenerating"
        :output-count="outputCountValue"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watchPostEffect, type ComponentPublicInstance } from 'vue'
import { MediaImage } from '@iconoir/vue'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../../types/canvasNodes'
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
  promptPlaceholder?: string
  directGenerating?: boolean
  showStrengthControl?: boolean
  strengthPct?: number
  setStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  maskEditorModeActive?: boolean
  maskEditorOverlayStyle?: Record<string, string> | null
  maskEditorCanvasWidth?: number
  maskEditorCanvasHeight?: number
  maskLayerOpacity?: number
  imageLayerVisible?: boolean
  maskEditorBrushSize?: number
  maskEditorBrushShape?: 'round' | 'square'
  setMaskCanvasRef?: (element: Element | null) => void
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
  (
    e: 'expand-handle-pointerdown',
    payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent },
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
  (e: 'mask-canvas-pointerdown', event: PointerEvent): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const emptyIconVariant = computed(() => props.emptyIconVariant ?? 'image')

const showImageActionMenu = computed(
  () => !!props.actionMenuVisible && !!props.displayImage && !props.maskEditorModeActive,
)
const showPromptComposer = computed(
  () => !!props.promptComposerVisible && !!props.displayImage && !props.expandModeActive,
)
type StageExpose = { stageElement: HTMLElement | null }
const imageStageRef = ref<StageExpose | null>(null)
const promptDraft = ref('')
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watchPostEffect((onCleanup) => {
  const flowNodeId = props.flowNodeId
  const stageElement = imageStageRef.value?.stageElement ?? null
  if (!flowNodeId) return

  props.setImageStageRef?.(flowNodeId, stageElement)
  onCleanup(() => {
    props.clearImageStageRef?.(flowNodeId, stageElement)
  })
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

function onImageStageContextMenu(event: MouseEvent) {
  if (props.maskEditorModeActive) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  emit('contextmenu', event)
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.workflow-node.image-node.is-expand-mode .image-node-stage,
.workflow-node.image-node.is-expand-mode.active .image-node-stage,
.workflow-node.image-node.is-mask-editor-mode .image-node-stage,
.workflow-node.image-node.is-mask-editor-mode.active .image-node-stage {
  border-color: transparent;
}
</style>
