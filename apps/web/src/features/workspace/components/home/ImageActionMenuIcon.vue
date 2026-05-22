<template>
  <span class="action-logo">
    <span v-if="icon === 'hd-text'" class="action-logo--text">HD</span>
    <component :is="resolvedIcon" v-else class="action-logo-icon" />
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../config/imageActionMenu'
import type { Component } from 'vue'
import {
  FrameTool,
  MagicWand,
  Scissor,
  Sparks,
  XmarkCircle,
} from '@iconoir/vue'

const props = defineProps<{
  icon: ImageActionMenuIcon
}>()

const iconMap: Record<Exclude<ImageActionMenuIcon, 'hd-text'>, Component> = {
  'remove-bg': Scissor,
  'style-variation': Sparks,
  'sketch-to-image': MagicWand,
  'image-extend': FrameTool,
  'image-inpaint': XmarkCircle,
}

const resolvedIcon = computed<Component>(() => (props.icon === 'hd-text' ? MagicWand : iconMap[props.icon]))
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
  stroke-width: 1.8;
}

.action-logo--text {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1;
}
</style>
