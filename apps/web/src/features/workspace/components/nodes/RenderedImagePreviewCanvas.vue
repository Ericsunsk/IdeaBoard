<template>
  <canvas
    ref="canvasRef"
    class="image-node-stage-image image-node-stage-image--preview"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { ImageEditorRenderedPreview } from '../../image-editor/types'

const props = defineProps<{
  preview: ImageEditorRenderedPreview
}>()

const canvasRef = ref<HTMLCanvasElement | null>(null)

watch(
  () => props.preview,
  async (preview) => {
    await nextTick()
    const canvas = canvasRef.value
    if (!canvas) return
    canvas.width = preview.width
    canvas.height = preview.height
    const context = canvas.getContext('2d')
    context?.putImageData(preview.imageData, 0, 0)
  },
  { immediate: true },
)
</script>
