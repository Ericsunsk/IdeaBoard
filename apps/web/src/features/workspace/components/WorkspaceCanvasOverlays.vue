<template>
  <ImageMaskEditorChrome
    v-if="
      imageEditorNodeId &&
      imageEditorMode === 'inpaint' &&
      imageEditorChromeAnchorRect &&
      isImageEditorChromeReady
    "
    :anchor-rect="imageEditorChromeAnchorRect"
    :draw-mode="flowMaskEditorDrawMode"
    :brush-shape="flowMaskEditorBrushShape"
    :brush-size="flowMaskEditorBrushSize"
    :brush-hardness="flowMaskEditorBrushHardness"
    :mask-layer-opacity="maskEditorLayerOpacity"
    :output-count="activeMaskEditorOutputCount"
    :reference-images="maskEditorReferenceImages"
    :direct-generating="isNodeDirectActionRunning(imageEditorNodeId)"
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
  <ImageEditorChrome
    v-if="
      imageEditorNodeId &&
      imageEditorMode === 'adjust' &&
      imageEditorChromeAnchorRect &&
      isImageEditorChromeReady
    "
    :anchor-rect="imageEditorChromeAnchorRect"
    :active-tool="activeImageEditorTool"
    :layers="imageEditorLayers"
    :selected-layer-id="selectedImageEditorLayerId"
    :selected-adjustment-layer="selectedImageEditorAdjustmentLayer"
    :applying-adjustments="applyingImageEditorAdjustments"
    @select-tool="emit('select-image-editor-tool', $event)"
    @select-adjustment="emit('select-image-adjustment-layer', $event)"
    @select-layer="emit('select-image-editor-layer', $event)"
    @set-layer-visible="(layerId, visible) => emit('set-image-editor-layer-visible', layerId, visible)"
    @add-layer="emit('add-image-editor-layer')"
    @copy-layer="emit('copy-image-editor-layer')"
    @merge-layer="emit('merge-image-editor-layer')"
    @remove-layer="emit('remove-image-editor-layer', $event)"
    @update-param="(key, value) => emit('update-image-editor-param', key, value)"
    @apply-adjustments="emit('apply-image-editor-adjustments')"
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
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { ImageEditorChrome } from '../image-editor'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorMode,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorTool,
} from '../image-editor'
import ImageMaskEditorChrome from './nodes/ImageMaskEditorChrome.vue'
import WorkspaceNodeContextMenu from './WorkspaceNodeContextMenu.vue'

type MaskEditorReferenceImage = {
  id: string
  url: string
  ratio?: number
}

defineProps<{
  imageEditorNodeId: string | null
  imageEditorMode: ImageEditorMode | null
  imageEditorChromeAnchorRect: { top: number; left: number; width: number; height: number } | null
  isImageEditorChromeReady: boolean
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
  activeImageEditorTool: ImageEditorTool
  imageEditorLayers: ImageEditorLayer[]
  selectedImageEditorLayerId: string
  selectedImageEditorAdjustmentLayer: ImageEditorAdjustmentLayer | null
  applyingImageEditorAdjustments: boolean
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
  'select-image-editor-tool': [tool: ImageEditorTool]
  'select-image-adjustment-layer': [kind: ImageAdjustmentKind]
  'select-image-editor-layer': [layerId: string]
  'set-image-editor-layer-visible': [layerId: string, visible: boolean]
  'add-image-editor-layer': []
  'copy-image-editor-layer': []
  'merge-image-editor-layer': []
  'remove-image-editor-layer': [layerId: string]
  'update-image-editor-param': [key: ImageEditorParamKey, value: ImageEditorParamValue]
  'apply-image-editor-adjustments': []
  submit: [prompt: string, outputCount: number]
  'create-node': [kind: CanvasCreatableNodeKind]
}>()
</script>
