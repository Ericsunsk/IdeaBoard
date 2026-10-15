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
    @load="image.onLoad"
    @error="image.onError"
  />
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { useSceneThumbnailMedia } from '../../session/preview/thumbnailMedia'

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
</script>
