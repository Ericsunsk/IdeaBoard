<template>
  <ImageInpaintChrome
    v-if="
      chromeModel.nodeId &&
      chromeModel.mode === 'inpaint' &&
      chromeModel.anchorRect &&
      chromeModel.isReady
    "
    :anchor-rect="chromeModel.anchorRect"
    :draw-mode="chromeModel.mask.drawMode"
    :brush-size="chromeModel.mask.brushSize"
    :mask-layer-opacity="chromeModel.mask.layerOpacity"
    :is-direct-generating="chromeModel.mask.directGenerating"
    :can-undo="chromeModel.mask.canUndo"
    :can-redo="chromeModel.mask.canRedo"
    @set-draw-mode="chromeCommands.setMaskDrawMode"
    @set-brush-size="chromeCommands.setMaskBrushSize"
    @set-mask-layer-opacity="chromeCommands.setMaskLayerOpacity"
    @clear-mask="chromeCommands.clearMask"
    @undo-mask="chromeCommands.undoMask"
    @redo-mask="chromeCommands.redoMask"
    @submit="chromeCommands.submitMaskPrompt"
  />
  <RetouchChrome
    v-if="
      chromeModel.nodeId &&
      chromeModel.mode === 'adjust' &&
      chromeModel.anchorRect &&
      chromeModel.isReady
    "
    :anchor-rect="chromeModel.anchorRect"
    :layers="chromeModel.editor.layers"
    :selected-layer-id="chromeModel.editor.selectedLayerId"
    :selected-effect-layer="chromeModel.editor.selectedEffectLayer"
    :histogram-source="chromeModel.editor.histogramSource"
    :applying-retouch="chromeModel.editor.applyingRetouch"
    :can-apply="chromeModel.editor.canApply"
    @select-retouch="chromeCommands.selectEffectLayer"
    @select-layer="chromeCommands.selectEditorLayer"
    @set-layer-visible="chromeCommands.setEditorLayerVisible"
    @add-layer="chromeCommands.addEditorLayer"
    @copy-layer="chromeCommands.copyEditorLayer"
    @merge-layer="chromeCommands.mergeEditorLayer"
    @remove-layer="chromeCommands.removeEditorLayer"
    @update-param="chromeCommands.updateEditorParam"
    @update:params="chromeCommands.updateEditorRetouchParams"
    @apply-retouch="chromeCommands.applyRetouch"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RetouchChrome } from '../../retouch'
import { ImageInpaintChrome } from '../../inpaint'
import type { ImageToolPresentationBinding } from '../session/bindings'

const props = defineProps<{ imageTools: ImageToolPresentationBinding }>()

const chromeModel = computed(() => props.imageTools.viewModel.chrome.value)
const chromeCommands = computed(() => props.imageTools.commands.chrome)
</script>
