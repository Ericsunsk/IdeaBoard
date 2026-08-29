<template>
  <div class="image-tool-stage-presentation">
    <RenderedImagePreviewCanvas v-if="binding.renderedPreview" :preview="binding.renderedPreview" />
    <slot v-else />
  </div>
  <ImageInpaintOverlay
    v-if="binding.mode === 'inpaint' && maskOverlayStyle"
    :key="`mask:${binding.nodeId}`"
    :style="maskOverlayStyle"
    :canvas-width="maskCanvasWidth"
    :canvas-height="maskCanvasHeight"
    :mask-layer-opacity="binding.session.maskChromeState.inpaintLayerOpacity.value"
    :image-layer-visible="true"
    :brush-size="binding.session.inpaint.brushSize.value"
    :set-canvas-ref="maskCanvasRefSetter"
    @canvas-pointerdown="binding.session.inpaint.onCanvasPointerDown"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import type { ImageToolStageBinding } from '../imageToolBindings'
import { ImageInpaintOverlay } from '../../inpaint'
import RenderedImagePreviewCanvas from './RenderedImagePreviewCanvas.vue'

const props = defineProps<{ binding: ImageToolStageBinding }>()
const maskOverlayStyle = computed(() =>
  props.binding.session.inpaint.inpaintNodeId.value === props.binding.nodeId
    ? props.binding.session.inpaint.inpaintOverlayStyle.value
    : null,
)
const maskCanvasWidth = computed(() => readMaskCanvasSize(maskOverlayStyle.value, 'width'))
const maskCanvasHeight = computed(() => readMaskCanvasSize(maskOverlayStyle.value, 'height'))
const maskCanvasRefSetter = computed(() => {
  const { nodeId, session } = props.binding
  return (element: Element | null) => session.inpaint.setMaskCanvasRef(nodeId, element)
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
  binding.session.inpaint.setMaskCanvasRef(binding.nodeId, null)
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
</style>
