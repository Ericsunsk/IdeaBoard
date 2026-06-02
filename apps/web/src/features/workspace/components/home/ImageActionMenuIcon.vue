<template>
  <span class="action-logo">
    <component :is="resolvedIcon" class="action-logo-icon" />
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

<style scoped>
.action-logo {
  display: flex;
  align-items: center;
  justify-content: center;
}

.action-logo-icon {
  width: 21px;
  height: 21px;
  stroke-width: var(--node-tool-icon-stroke, 1.5);
}
</style>
