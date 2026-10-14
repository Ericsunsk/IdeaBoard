<template>
  <span class="flex items-center justify-center">
    <component :is="resolvedIcon" class="size-[21px]" />
  </span>
</template>

<script setup lang="ts">
import type { ImageActionMenuIcon } from '../../../core/nodes/imageActionRegistry.ts'
import { computed, type Component } from 'vue'
import { AdobePhotoshop } from '@iconoir/vue'
import { getServiceIcon } from '../../../../../core/components/serviceIcons'

const props = defineProps<{
  icon: ImageActionMenuIcon
}>()

const resolvedIcon = computed<Component>(() =>
  props.icon === 'retouch' ? AdobePhotoshop : getServiceIcon(props.icon),
)
</script>
