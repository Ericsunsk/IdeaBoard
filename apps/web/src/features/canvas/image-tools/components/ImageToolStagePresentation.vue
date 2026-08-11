<template>
  <div
    class="image-tool-stage-presentation"
    :class="{ 'has-rendered-preview': !!binding.renderedPreview }"
  >
    <slot />
  </div>
  <RenderedImagePreviewCanvas v-if="binding.renderedPreview" :preview="binding.renderedPreview" />
  <ImageEditorRasterOverlay
    v-if="isRasterOverlayVisible && rasterOverlayStyle"
    :key="`raster:${binding.nodeId}`"
    :style="rasterOverlayStyle"
    :canvas-width="binding.session.imageEditorRasterOverlay.rasterCanvasWidth.value"
    :canvas-height="binding.session.imageEditorRasterOverlay.rasterCanvasHeight.value"
    :brush-size="binding.session.imageEditorRasterOverlay.rasterBrushSize.value"
    :raster-tool-enabled="binding.session.imageEditorRasterOverlay.rasterToolEnabled.value"
    :set-canvas-ref="rasterCanvasRefSetter"
    @canvas-pointerdown="binding.session.imageEditorRasterOverlay.onRasterCanvasPointerDown"
  />
  <ImageMaskEditorOverlay
    v-if="binding.mode === 'inpaint' && maskOverlayStyle"
    :key="`mask:${binding.nodeId}`"
    :style="maskOverlayStyle"
    :canvas-width="maskCanvasWidth"
    :canvas-height="maskCanvasHeight"
    :mask-layer-opacity="binding.session.maskChromeState.maskEditorLayerOpacity.value"
    :image-layer-visible="true"
    :brush-size="binding.session.maskEditor.brushSize.value"
    :brush-shape="binding.session.maskEditor.brushShape.value"
    :set-canvas-ref="maskCanvasRefSetter"
    @canvas-pointerdown="binding.session.maskEditor.onCanvasPointerDown"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import type { ImageToolStageBinding } from '../imageToolBindings'
import { ImageEditorRasterOverlay } from '../../image-editor'
import { ImageMaskEditorOverlay } from '../../mask-editor'
import RenderedImagePreviewCanvas from '../../components/nodes/RenderedImagePreviewCanvas.vue'

const props = defineProps<{ binding: ImageToolStageBinding }>()
const maskOverlayStyle = computed(() =>
  props.binding.session.maskEditor.maskEditorNodeId.value === props.binding.nodeId
    ? props.binding.session.maskEditor.maskEditorOverlayStyle.value
    : null,
)
const rasterOverlayStyle = computed(() =>
  props.binding.mode === 'adjust'
    ? props.binding.session.imageEditorRasterOverlay.rasterOverlayStyle.value
    : null,
)
const isRasterOverlayVisible = computed(
  () =>
    props.binding.session.imageEditorRasterOverlay.hasRasterEdits.value ||
    props.binding.session.imageEditorRasterOverlay.rasterTool.value != null,
)
const maskCanvasWidth = computed(() => readMaskCanvasSize(maskOverlayStyle.value, 'width'))
const maskCanvasHeight = computed(() => readMaskCanvasSize(maskOverlayStyle.value, 'height'))
const maskCanvasRefSetter = computed(() => {
  const { nodeId, session } = props.binding
  return (element: Element | null) => session.maskEditor.setMaskCanvasRef(nodeId, element)
})
const rasterCanvasRefSetter = computed(() => {
  const { nodeId, session } = props.binding
  return (element: Element | null) =>
    session.imageEditorRasterOverlay.setRasterCanvasRef(nodeId, element)
})

watch(
  () => props.binding,
  (binding, previousBinding) => {
    if (
      binding.nodeId === previousBinding.nodeId &&
      binding.mode === previousBinding.mode &&
      binding.session === previousBinding.session
    ) {
      return
    }

    releaseCanvasRefs(previousBinding)
  },
  { flush: 'sync' },
)

onBeforeUnmount(() => {
  releaseCanvasRefs(props.binding)
})

function releaseCanvasRefs(binding: ImageToolStageBinding) {
  binding.session.maskEditor.setMaskCanvasRef(binding.nodeId, null)
  binding.session.imageEditorRasterOverlay.setRasterCanvasRef(binding.nodeId, null)
}
function readMaskCanvasSize(style: Record<string, string> | null, key: 'width' | 'height') {
  return Math.max(1, Math.round(parseFloat(style?.[key] ?? '') || 0))
}
</script>

<style scoped>
.image-tool-stage-presentation {
  position: absolute;
  inset: 0;
}
.image-tool-stage-presentation :slotted(.image-tool-stage-media) {
  width: 100%;
  height: 100%;
}
.image-tool-stage-presentation.has-rendered-preview :slotted(.image-tool-stage-media) {
  visibility: hidden;
}
</style>
