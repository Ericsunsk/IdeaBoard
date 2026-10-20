<template>
  <NodeShell
    kind="image"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || t('canvas.node.image')"
    :is-active="isActive"
    :is-dragging="isDragging"
    :is-copy-mode="isCopyMode"
    :class="{
      'is-outpaint-mode': !!isOutpaintModeActive,
      'is-retouch-active': isRetouchActive,
    }"
    is-rounded-header
    :is-header-hidden="!!isOutpaintModeActive || isRetouchActive"
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <MediaImage />
    </template>
    <div
      class="relative flex flex-col gap-2 overflow-visible rounded-panel p-0"
      :class="
        isOutpaintModeActive || isRetouchActive ? 'bg-transparent' : 'bg-[var(--canvas-node-bg)]'
      "
      @contextmenu.stop
    >
      <ImageNodeStage
        ref="imageStageRef"
        :node-id="node.id"
        :flow-node-id="flowNodeId"
        :stage-width="node.width"
        :stage-height="stageHeight"
        :display-image="displayImage"
        :library-import="libraryImport"
        :local-upload="localUpload"
        :is-input-locked="isInputLocked"
        :empty-icon-variant="emptyIconVariant"
        :is-direct-generating="actionBinding.isDirectGenerating"
        :is-outpaint-mode-active="isOutpaintModeActive"
        :outpaint-overlay-style="outpaintOverlayStyle"
        :outpaint-handle-radius="outpaintHandleRadius"
        :image-tool-stage-binding="imageToolStageBinding"
        :is-active="isActive"
        :is-connection-drop-target="isConnectionDropTarget"
        :is-dragging="isDragging"
        @stage-drop="emit('stage-drop', $event)"
        @open-file="emit('open-file')"
        @open-preview="emit('open-preview')"
        @append-to-canvas="emit('append-to-canvas')"
        @reset="emit('reset')"
        @retry-image-import="emit('retry-image-import')"
        @contextmenu="onImageStageContextMenu"
        @outpaint-submit="submitOutpaint"
        @outpaint-handle-pointerdown="emit('outpaint-handle-pointerdown', $event)"
        @outpaint-handle-pointermove="emit('outpaint-handle-pointermove', $event)"
        @outpaint-handle-pointerup="emit('outpaint-handle-pointerup', $event)"
        @outpaint-frame-pointerdown="emit('outpaint-frame-pointerdown', $event)"
      />

      <NodeFloatingControls v-if="shouldShowImageActionMenu || shouldShowPromptComposer">
        <HomeCanvasImageActionMenu
          v-if="shouldShowImageActionMenu"
          :output-count="outputCountValue"
          :output-resolution="node.outputResolution"
          :strength-control="actionBinding.strengthControl"
          @action="submitAction"
        />
        <NodePromptComposer
          v-if="shouldShowPromptComposer"
          v-model="promptDraft"
          offset="52px"
          :placeholder="actionBinding.promptPlaceholder"
          should-allow-empty
          :should-show-submit="false"
          :output-count="outputCountValue"
          :output-resolution="node.outputResolution"
          @update:output-count="selectOutputCount"
          @update:output-resolution="emit('update:output-resolution', $event)"
        />
      </NodeFloatingControls>
    </div>
  </NodeShell>
</template>

<script setup lang="ts">
import { computed, ref, watchPostEffect } from 'vue'
import { useI18n } from 'vue-i18n'
import { MediaImage } from '@iconoir/vue'
import {
  requireImageNodeOutputCount,
  type ImageNodeItem,
  type LibraryImportFact,
} from '../../../../board/ports/canvas'
import type { ImageNodeAction } from '../../../core/nodes/imageActionRegistry.ts'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import type { OutpaintDragAnchor } from '../../../tools/outpaint/session/useImageOutpaintOverlay.ts'
import type { ImageActionBinding, ImageToolStageBinding } from '../../../tools/image-tools'
import type { FlowLocalUploadState } from '../../../core/nodes/flowTypes.ts'
import HomeCanvasImageActionMenu from '../home/ImageActionMenu.vue'
import { NodePromptComposer, NodeFloatingControls, NodeShell } from '../primitives'
import ImageNodeStage from './ImageNodeStage.vue'

const props = defineProps<{
  node: ImageNodeItem
  flowNodeId?: string | undefined
  setImageStageRef?: (nodeId: string, element: Element | null) => void
  clearImageStageRef?: (nodeId: string, element?: Element | null) => void
  isActive: boolean
  isConnectionDropTarget?: boolean
  isDragging: boolean
  isCopyMode: boolean
  stageHeight: number
  displayImage: { url: string; assetId?: string | undefined } | null
  libraryImport?: LibraryImportFact | undefined
  localUpload?: FlowLocalUploadState | undefined
  emptyIconVariant?: 'image' | 'fusion'
  isInputLocked?: boolean
  actionBinding: ImageActionBinding
  isOutpaintModeActive?: boolean
  outpaintOverlayStyle?: Record<string, string> | null
  outpaintHandleRadius?: number
  imageToolStageBinding?: ImageToolStageBinding | null
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:output-resolution', value: OutputResolution): void
  (e: 'delete'): void
  (e: 'stage-drop', event: DragEvent): void
  (e: 'open-file'): void
  (e: 'open-preview'): void
  (e: 'append-to-canvas'): void
  (e: 'reset'): void
  (e: 'retry-image-import'): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'outpaint-submit', prompt: string): void
  (
    e: 'action',
    action: ImageNodeAction,
    prompt: string,
    outputCount: number,
    strength?: number,
  ): void
  (
    e: 'outpaint-handle-pointerdown',
    payload: { anchor: OutpaintDragAnchor; event: PointerEvent },
  ): void
  (e: 'outpaint-handle-pointermove', event: PointerEvent): void
  (e: 'outpaint-handle-pointerup', event: PointerEvent): void
  (e: 'outpaint-frame-pointerdown', event: PointerEvent): void
}>()

const { t } = useI18n()

const emptyIconVariant = computed(() => props.emptyIconVariant ?? 'image')
const isRetouchActive = computed(() => props.imageToolStageBinding?.mode != null)

const shouldShowImageActionMenu = computed(
  () =>
    !props.libraryImport &&
    !props.localUpload &&
    props.actionBinding.isActionMenuVisible &&
    !!props.displayImage &&
    !isRetouchActive.value,
)
const shouldShowPromptComposer = computed(
  () =>
    !props.libraryImport &&
    !props.localUpload &&
    props.actionBinding.isPromptComposerVisible &&
    !!props.displayImage &&
    !props.isOutpaintModeActive,
)
type StageExpose = { stageElement: HTMLElement | null }
const imageStageRef = ref<StageExpose | null>(null)
const promptDraft = ref('')
const outputCountValue = computed(() => requireImageNodeOutputCount(props.node.outputCount))

watchPostEffect((onCleanup) => {
  const flowNodeId = props.flowNodeId
  const stageElement = imageStageRef.value?.stageElement ?? null
  if (!flowNodeId) return

  props.setImageStageRef?.(flowNodeId, stageElement)
  onCleanup(() => {
    props.clearImageStageRef?.(flowNodeId, stageElement)
  })
})

function submitAction(action: ImageNodeAction, strength?: number) {
  emit('action', action, promptDraft.value.trim(), outputCountValue.value, strength)
}

function submitOutpaint(prompt: string) {
  emit('outpaint-submit', prompt)
}

function selectOutputCount(value: number) {
  emit('update:output-count', requireImageNodeOutputCount(value))
}

function onImageStageContextMenu(event: MouseEvent) {
  if (isRetouchActive.value) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  emit('contextmenu', event)
}
</script>
