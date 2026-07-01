<template>
  <NodeShell
    kind="image"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || 'Image'"
    :active="active"
    :dragging="dragging"
    :copy-mode="copyMode"
    :class="{
      'is-expand-mode': !!expandModeActive,
      'is-image-editor-active': isImageEditorActive,
    }"
    rounded-header
    :header-hidden="!!expandModeActive || isImageEditorActive"
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <MediaImage />
    </template>
    <div
      class="relative flex flex-col gap-2 overflow-visible rounded-[11px] p-0"
      :class="expandModeActive || isImageEditorActive ? 'bg-transparent' : 'bg-white'"
      @contextmenu.stop
    >
      <ImageNodeStage
        ref="imageStageRef"
        :node-id="node.id"
        :flow-node-id="flowNodeId"
        :stage-width="node.width"
        :stage-height="stageHeight"
        :display-image="displayImage"
        :input-locked="inputLocked"
        :empty-icon-variant="emptyIconVariant"
        :direct-generating="actionBinding.directGenerating"
        :expand-mode-active="expandModeActive"
        :expand-overlay-style="expandOverlayStyle"
        :expand-handle-radius="expandHandleRadius"
        :image-tool-stage-binding="imageToolStageBinding"
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
      >
        <template #handle-rail>
          <slot name="handle-rail" />
        </template>
      </ImageNodeStage>

      <HomeCanvasImageActionMenu
        v-if="showImageActionMenu"
        :show-strength-control="actionBinding.showStrengthControl"
        :direct-generating="actionBinding.directGenerating"
        :active-strength-action="actionBinding.activeStrengthAction"
        :get-strength-value="actionBinding.getStrengthValue"
        :set-strength-range-ref="actionBinding.setStrengthRangeRef"
        :on-strength-range-pointer-down="actionBinding.onStrengthRangePointerDown"
        :on-strength-input="actionBinding.onStrengthInput"
        :on-strength-change="actionBinding.onStrengthChange"
        :on-strength-pointer-up="actionBinding.onStrengthPointerUp"
        :on-strength-click="actionBinding.onStrengthClick"
        @action="emit('action', $event)"
      />
      <NodePromptComposer
        v-if="showPromptComposer"
        v-model="promptDraft"
        offset="52px"
        :placeholder="actionBinding.promptPlaceholder"
        :disabled="actionBinding.directGenerating"
        :output-count="outputCountValue"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </NodeShell>
</template>

<script setup lang="ts">
import { computed, ref, watchPostEffect } from 'vue'
import { MediaImage } from '@iconoir/vue'
import { clampImageNodeOutputCount, type ImageNodeItem } from '../../../canvas-board/domain'
import type { ImageNodeAction } from '../../config/imageActionRegistry'
import type { CanvasExtendDragAnchor } from '../../composables/useImageCanvasExtendOverlay'
import type { ImageActionBinding, ImageToolStageBinding } from '../../image-tools'
import HomeCanvasImageActionMenu from '../home/HomeCanvasImageActionMenu.vue'
import NodePromptComposer from './NodePromptComposer.vue'
import ImageNodeStage from './ImageNodeStage.vue'
import NodeShell from './NodeShell.vue'

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
  actionBinding: ImageActionBinding
  expandModeActive?: boolean
  expandOverlayStyle?: Record<string, string> | null
  expandHandleRadius?: number
  imageToolStageBinding?: ImageToolStageBinding | null
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
  (
    e: 'expand-handle-pointerdown',
    payload: { anchor: CanvasExtendDragAnchor; event: PointerEvent },
  ): void
  (e: 'expand-handle-pointermove', event: PointerEvent): void
  (e: 'expand-handle-pointerup', event: PointerEvent): void
  (e: 'expand-frame-pointerdown', event: PointerEvent): void
}>()

const emptyIconVariant = computed(() => props.emptyIconVariant ?? 'image')
const isImageEditorActive = computed(() => props.imageToolStageBinding?.mode != null)

const showImageActionMenu = computed(
  () => props.actionBinding.actionMenuVisible && !!props.displayImage && !isImageEditorActive.value,
)
const showPromptComposer = computed(
  () =>
    props.actionBinding.promptComposerVisible && !!props.displayImage && !props.expandModeActive,
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
  if (!prompt || props.actionBinding.directGenerating) return
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
}

function submitExpand(prompt: string, outputCount: number) {
  if (props.actionBinding.directGenerating) return
  emit('expand-submit', prompt, outputCount)
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
}

function onImageStageContextMenu(event: MouseEvent) {
  if (isImageEditorActive.value) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  emit('contextmenu', event)
}
</script>
