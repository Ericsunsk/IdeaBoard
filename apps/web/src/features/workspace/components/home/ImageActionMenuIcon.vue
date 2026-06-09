<template>
  <span class="flex items-center justify-center">
    <component
      :is="resolvedIcon"
      class="size-[21px] [stroke-width:var(--node-tool-icon-stroke,1.5)]"
    />
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../config/imageActionMenu'
import type { Component } from 'vue'
import { BorderInner, DesignPencil, EditPencil, Expand, Hd, Spark } from '@iconoir/vue'

const props = defineProps<{
  icon: ImageActionMenuIcon
}>()

const iconMap: Record<ImageActionMenuIcon, Component> = {
  'remove-bg': BorderInner,
  'style-variation': Spark,
  'sketch-to-image': EditPencil,
  hd: Hd,
  'image-extend': Expand,
  'image-inpaint': DesignPencil,
}

const resolvedIcon = computed<Component>(() => iconMap[props.icon])
</script>
