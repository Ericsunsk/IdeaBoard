<template>
  <ImageMaskEditorChrome
    v-if="
      chromeModel.nodeId &&
      chromeModel.mode === 'inpaint' &&
      chromeModel.anchorRect &&
      chromeModel.isReady
    "
    :anchor-rect="chromeModel.anchorRect"
    :draw-mode="chromeModel.mask.drawMode"
    :brush-shape="chromeModel.mask.brushShape"
    :brush-size="chromeModel.mask.brushSize"
    :brush-hardness="chromeModel.mask.brushHardness"
    :mask-layer-opacity="chromeModel.mask.layerOpacity"
    :output-count="chromeModel.mask.outputCount"
    :reference-images="chromeModel.mask.referenceImages"
    :direct-generating="chromeModel.mask.directGenerating"
    :can-undo="chromeModel.mask.canUndo"
    :can-redo="chromeModel.mask.canRedo"
    @set-draw-mode="chromeCommands.setMaskDrawMode"
    @set-brush-shape="chromeCommands.setMaskBrushShape"
    @set-brush-size="chromeCommands.setMaskBrushSize"
    @set-brush-hardness="chromeCommands.setMaskBrushHardness"
    @set-mask-layer-opacity="chromeCommands.setMaskLayerOpacity"
    @clear-mask="chromeCommands.clearMask"
    @invert-mask="chromeCommands.invertMask"
    @feather-mask="chromeCommands.featherMask"
    @undo-mask="chromeCommands.undoMask"
    @redo-mask="chromeCommands.redoMask"
    @update:output-count="chromeCommands.updateMaskOutputCount"
    @image-input-open="chromeCommands.openMaskReferenceImageInput"
    @reference-image-remove="chromeCommands.removeMaskReferenceImage"
    @reference-image-preview="chromeCommands.previewMaskReferenceImage"
    @submit="chromeCommands.submitMaskPrompt"
  />
  <ImageEditorChrome
    v-if="
      chromeModel.nodeId &&
      chromeModel.mode === 'adjust' &&
      chromeModel.anchorRect &&
      chromeModel.isReady
    "
    :anchor-rect="chromeModel.anchorRect"
    :active-tool="chromeModel.editor.activeTool"
    :layers="chromeModel.editor.layers"
    :selected-layer-id="chromeModel.editor.selectedLayerId"
    :selected-adjustment-layer="chromeModel.editor.selectedAdjustmentLayer"
    :raster-tool-settings="chromeModel.editor.rasterToolSettings"
    :can-use-raster-tool="chromeModel.editor.canUseRasterTool"
    :can-undo-raster-stroke="chromeModel.editor.canUndoRasterStroke"
    :can-redo-raster-stroke="chromeModel.editor.canRedoRasterStroke"
    :has-raster-edits="chromeModel.editor.hasRasterEdits"
    :histogram-source="chromeModel.editor.histogramSource"
    :applying-adjustments="chromeModel.editor.applyingAdjustments"
    @select-tool="chromeCommands.selectEditorTool"
    @select-adjustment="chromeCommands.selectAdjustmentLayer"
    @select-layer="chromeCommands.selectEditorLayer"
    @set-layer-visible="chromeCommands.setEditorLayerVisible"
    @add-layer="chromeCommands.addEditorLayer"
    @copy-layer="chromeCommands.copyEditorLayer"
    @merge-layer="chromeCommands.mergeEditorLayer"
    @remove-layer="chromeCommands.removeEditorLayer"
    @update-param="chromeCommands.updateEditorParam"
    @update-params="chromeCommands.updateEditorAdjustmentParams"
    @update-raster-settings="chromeCommands.updateEditorRasterToolSettings"
    @undo-raster-stroke="chromeCommands.undoEditorRasterStroke"
    @redo-raster-stroke="chromeCommands.redoEditorRasterStroke"
    @apply-adjustments="chromeCommands.applyAdjustments"
  />
  <CanvasNodeContextMenu
    :visible="nodeContextMenu.visible"
    :x="nodeContextMenu.x"
    :y="nodeContextMenu.y"
    :items="canvasNodeMenuItems"
    @create="emit('create-node', $event)"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { ImageEditorChrome } from '../image-editor'
import { ImageMaskEditorChrome } from '../mask-editor'
import type { CanvasImageTools } from '../image-tools'
import CanvasNodeContextMenu from './CanvasNodeContextMenu.vue'

type ImageToolChromeSession = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}

const props = defineProps<{
  imageToolSession: ImageToolChromeSession
  nodeContextMenu: { visible: boolean; x: number; y: number }
  canvasNodeMenuItems: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  'create-node': [kind: CanvasCreatableNodeKind]
}>()

const chromeModel = computed(() => props.imageToolSession.viewModel.chrome.value)
const chromeCommands = computed(() => props.imageToolSession.commands.chrome)
</script>
