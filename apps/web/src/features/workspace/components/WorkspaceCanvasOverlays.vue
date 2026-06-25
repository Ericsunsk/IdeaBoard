<template>
  <ImageMaskEditorChrome
    v-if="flowMaskEditorNodeId && maskEditorChromeAnchorRect && isMaskEditorChromeReady"
    :anchor-rect="maskEditorChromeAnchorRect"
    :draw-mode="flowMaskEditorDrawMode"
    :brush-shape="flowMaskEditorBrushShape"
    :brush-size="flowMaskEditorBrushSize"
    :brush-hardness="flowMaskEditorBrushHardness"
    :mask-layer-opacity="maskEditorLayerOpacity"
    :output-count="activeMaskEditorOutputCount"
    :reference-images="maskEditorReferenceImages"
    :direct-generating="isNodeDirectActionRunning(flowMaskEditorNodeId)"
    :can-undo="canUndoFlowMaskEditor"
    :can-redo="canRedoFlowMaskEditor"
    @set-draw-mode="emit('set-draw-mode', $event)"
    @set-brush-shape="emit('set-brush-shape', $event)"
    @set-brush-size="emit('set-brush-size', $event)"
    @set-brush-hardness="emit('set-brush-hardness', $event)"
    @set-mask-layer-opacity="emit('set-mask-layer-opacity', $event)"
    @clear-mask="emit('clear-mask')"
    @invert-mask="emit('invert-mask')"
    @feather-mask="emit('feather-mask')"
    @undo-mask="emit('undo-mask')"
    @redo-mask="emit('redo-mask')"
    @update:output-count="emit('update:output-count', $event)"
    @image-input-open="emit('image-input-open')"
    @reference-image-remove="emit('reference-image-remove', $event)"
    @reference-image-preview="emit('reference-image-preview', $event)"
    @submit="(prompt, outputCount) => emit('submit', prompt, outputCount)"
  />
  <WorkspaceNodeContextMenu
    :visible="nodeContextMenu.visible"
    :x="nodeContextMenu.x"
    :y="nodeContextMenu.y"
    :items="canvasNodeMenuItems"
    @create="emit('create-node', $event)"
  />
</template>

<script setup lang="ts">
import type {
  ImageMaskBrushShape,
  ImageMaskDrawMode,
} from '../composables/useImageMaskEditorOverlay'
import type { CanvasCreatableNodeKind } from '../config/canvasNodeRegistry'
import ImageMaskEditorChrome from './nodes/ImageMaskEditorChrome.vue'
import WorkspaceNodeContextMenu from './WorkspaceNodeContextMenu.vue'

type MaskEditorReferenceImage = {
  id: string
  url: string
  ratio?: number
}

defineProps<{
  flowMaskEditorNodeId: string | null
  maskEditorChromeAnchorRect: { top: number; left: number; width: number; height: number } | null
  isMaskEditorChromeReady: boolean
  flowMaskEditorDrawMode: ImageMaskDrawMode
  flowMaskEditorBrushShape: ImageMaskBrushShape
  flowMaskEditorBrushSize: number
  flowMaskEditorBrushHardness: number
  maskEditorLayerOpacity: number
  activeMaskEditorOutputCount: number
  maskEditorReferenceImages: MaskEditorReferenceImage[]
  isNodeDirectActionRunning: (nodeId: string) => boolean
  canUndoFlowMaskEditor: boolean
  canRedoFlowMaskEditor: boolean
  nodeContextMenu: { visible: boolean; x: number; y: number }
  canvasNodeMenuItems: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  'set-draw-mode': [mode: ImageMaskDrawMode]
  'set-brush-shape': [shape: ImageMaskBrushShape]
  'set-brush-size': [size: number]
  'set-brush-hardness': [hardness: number]
  'set-mask-layer-opacity': [opacity: number]
  'clear-mask': []
  'invert-mask': []
  'feather-mask': []
  'undo-mask': []
  'redo-mask': []
  'update:output-count': [count: number]
  'image-input-open': []
  'reference-image-remove': [imageId: string]
  'reference-image-preview': [image: MaskEditorReferenceImage]
  submit: [prompt: string, outputCount: number]
  'create-node': [kind: CanvasCreatableNodeKind]
}>()
</script>
