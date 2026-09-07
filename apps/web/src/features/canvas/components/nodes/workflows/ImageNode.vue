<template>
  <NodeShell
    kind="image"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || t('canvas.node.image')"
    :active="active"
    :dragging="dragging"
    :copy-mode="copyMode"
    :class="{
      'is-outpaint-mode': !!outpaintModeActive,
      'is-retouch-active': isRetouchActive,
    }"
    rounded-header
    :header-hidden="!!outpaintModeActive || isRetouchActive"
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <MediaImage />
    </template>
    <div
      class="relative flex flex-col gap-2 overflow-visible rounded-[11px] p-0"
      :class="
        outpaintModeActive || isRetouchActive ? 'bg-transparent' : 'bg-[var(--canvas-node-bg)]'
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
        :input-locked="inputLocked"
        :empty-icon-variant="emptyIconVariant"
        :direct-generating="actionBinding.directGenerating"
        :outpaint-mode-active="outpaintModeActive"
        :outpaint-overlay-style="outpaintOverlayStyle"
        :outpaint-handle-radius="outpaintHandleRadius"
        :image-tool-stage-binding="imageToolStageBinding"
        :active="active"
        :connection-drop-target="connectionDropTarget"
        :dragging="dragging"
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

      <NodeFloatingControls v-if="showImageActionMenu || showPromptComposer">
        <HomeCanvasImageActionMenu
          v-if="showImageActionMenu"
          :output-count="outputCountValue"
          :output-resolution="node.outputResolution"
          :strength-control="actionBinding.strengthControl"
          @action="submitAction"
        />
        <NodePromptComposer
          v-if="showPromptComposer"
          v-model="promptDraft"
          offset="52px"
          :placeholder="actionBinding.promptPlaceholder"
          allow-empty
          :show-submit="false"
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
} from '../../../../canvas-board/domain'
import type { ImageNodeAction } from '../../../config/imageActionRegistry'
import type { OutputResolution } from '../../../../../shared/domain/generationContract'
import type { OutpaintDragAnchor } from '../../../image-workflow/useImageOutpaintOverlay'
import type { ImageActionBinding, ImageToolStageBinding } from '../../../image-tools'
import type { FlowLocalUploadState } from '../../../types/flowTypes'
import HomeCanvasImageActionMenu from '../../home/HomeCanvasImageActionMenu.vue'
import NodePromptComposer from '../primitives/NodePromptComposer.vue'
import NodeFloatingControls from '../primitives/NodeFloatingControls.vue'
import ImageNodeStage from './ImageNodeStage.vue'
import NodeShell from '../primitives/NodeShell.vue'

const props = defineProps<{
  node: ImageNodeItem
  flowNodeId?: string | undefined
  setImageStageRef?: (nodeId: string, element: Element | null) => void
  clearImageStageRef?: (nodeId: string, element?: Element | null) => void
  active: boolean
  connectionDropTarget?: boolean
  dragging: boolean
  copyMode: boolean
  stageHeight: number
  displayImage: { url: string; mediaAssetId?: number | undefined } | null
  libraryImport?: LibraryImportFact | undefined
  localUpload?: FlowLocalUploadState | undefined
  emptyIconVariant?: 'image' | 'fusion'
  inputLocked?: boolean
  actionBinding: ImageActionBinding
  outpaintModeActive?: boolean
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

const showImageActionMenu = computed(
  () =>
    !props.libraryImport &&
    !props.localUpload &&
    props.actionBinding.actionMenuVisible &&
    !!props.displayImage &&
    !isRetouchActive.value,
)
const showPromptComposer = computed(
  () =>
    !props.libraryImport &&
    !props.localUpload &&
    props.actionBinding.promptComposerVisible &&
    !!props.displayImage &&
    !props.outpaintModeActive,
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
