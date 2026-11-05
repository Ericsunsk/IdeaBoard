<template>
  <image
    v-for="image in images"
    :key="image.version"
    :href="image.url"
    :opacity="image.isVisible ? 1 : 0"
    :x="x"
    :y="y"
    :width="width"
    :height="height"
    :clip-path="`url(#${clipId})`"
    preserveAspectRatio="xMidYMid slice"
    class="scene-raster-image"
    @load="image.onLoad"
    @error="image.onError"
  />
</template>

<script setup lang="ts">
import { toRef, watch } from 'vue'
import { useSceneThumbnailMedia } from '../../../session/preview/thumbnailMedia'

const props = defineProps<{
  workspaceId: string
  boardId: string
  contentVersion: string
  assetId: string
  x: number
  y: number
  width: number
  height: number
  clipId: string
  isEnabled: boolean
}>()

const emit = defineEmits<{
  (e: 'resolved', isResolved: boolean): void
}>()

// Scene generation never waits on this: the layout (frames, glyphs) renders
// first and this raster commits only when the authorized delivery reports a
// ready preview derivative. Missing media resolves to no element at all.
const { images } = useSceneThumbnailMedia({
  assetId: toRef(props, 'assetId'),
  workspaceId: toRef(props, 'workspaceId'),
  boardId: toRef(props, 'boardId'),
  contentVersion: toRef(props, 'contentVersion'),
  isEnabled: toRef(props, 'isEnabled'),
})

// Readiness report per identity so the parent can keep the solid frame only
// while an eligible raster is still pending/loading. Once any raster mounts
// (candidate or displayed, any mime including unknown) the frame goes
// transparent: opaque pixels cover it fully via `slice`, so their paint is
// unchanged, while transparent sources show the card surface instead of a
// gray matte. Empty identity clears back to pending (solid placeholder).
watch(
  () => images.value.length,
  (length) => {
    emit('resolved', length > 0)
  },
  { immediate: true },
)
</script>

<style scoped>
.scene-raster-image {
  transition: opacity var(--motion-image-duration) var(--motion-image-easing);
}

@media (prefers-reduced-motion: reduce) {
  .scene-raster-image {
    transition: none;
  }
}
</style>
