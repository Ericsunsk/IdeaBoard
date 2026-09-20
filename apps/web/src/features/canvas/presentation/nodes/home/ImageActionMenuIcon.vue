<template>
  <span class="flex items-center justify-center">
    <component :is="resolvedIcon" class="size-[21px]" />
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../../domain/nodes/imageActionRegistry.ts'
import { computed, type Component } from 'vue'
import {
  AdobePhotoshop,
  BorderInner,
  EditPencil,
  Expand,
  Palette,
  Spark,
  ZoomIn,
} from '@iconoir/vue'

const props = defineProps<{
  icon: ImageActionMenuIcon
}>()

const iconMap: Record<ImageActionMenuIcon, Component> = {
  cutout: BorderInner,
  variations: Spark,
  render: Palette,
  upscale: ZoomIn,
  outpaint: Expand,
  inpaint: EditPencil,
  retouch: AdobePhotoshop,
}

const resolvedIcon = computed<Component>(() => iconMap[props.icon])
</script>
