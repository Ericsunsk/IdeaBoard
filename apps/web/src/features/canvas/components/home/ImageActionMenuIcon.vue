<template>
  <span class="flex items-center justify-center">
    <component :is="resolvedIcon" class="size-[21px]" />
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../config/imageActionRegistry'
import { computed, type Component } from 'vue'
import {
  AdobePhotoshop,
  BorderInner,
  DesignPencil,
  EditPencil,
  Expand,
  Hd,
  Spark,
} from '@iconoir/vue'

const props = defineProps<{
  icon: ImageActionMenuIcon
}>()

const iconMap: Record<ImageActionMenuIcon, Component> = {
  'cutout': BorderInner,
  'variations': Spark,
  'render': EditPencil,
  hd: Hd,
  'outpaint': Expand,
  'image-inpaint': DesignPencil,
  'retouch': AdobePhotoshop,
}

const resolvedIcon = computed<Component>(() => iconMap[props.icon])
</script>
