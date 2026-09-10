<template>
  <div class="image-tool-stage-presentation">
    <slot />
    <canvas
      v-if="showZeroCopyCanvas"
      :ref="(element) => zeroCopyCanvas?.setCanvasRef(element)"
      class="image-node-stage-image image-node-stage-image--preview image-tool-stage-zero-copy"
      aria-hidden="true"
    />
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
import type { ImageToolStageBinding } from '../bindings'
import { ImageInpaintOverlay } from '../../inpaint'
import { isWebGLRetouchSupported } from '../../retouch/processing/webglEngine.ts'

const props = defineProps<{ binding: ImageToolStageBinding }>()
// Zero-copy GPU stage (issue #672): the mounted canvas IS the render target
// and stays mounted for the whole adjust session, so the GL context survives
// slider drags. Before the first GPU draw (or when the GPU is unavailable)
// it is transparent and the CPU preview / slot beneath shows through.
const zeroCopyCanvas = computed(() =>
  props.binding.mode === 'adjust' ? (props.binding.retouchStage ?? null) : null,
)
const showZeroCopyCanvas = computed(() => zeroCopyCanvas.value !== null && gpuSupported)
const gpuSupported = isWebGLRetouchSupported()
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
  binding.retouchStage?.setCanvasRef(null)
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
.image-node-stage-image--preview {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}
.image-tool-stage-zero-copy {
  position: absolute;
  inset: 0;
}
</style>
