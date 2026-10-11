<template>
  <AdminSurfaceFrame :content-class="props.contentClass" :viewport-height="props.viewportHeight">
    <template v-if="$slots.toolbar" #toolbar>
      <slot name="toolbar" />
    </template>

    <div :class="tableFrameClass">
      <slot />
    </div>
  </AdminSurfaceFrame>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import AdminSurfaceFrame from '../shell/AdminSurfaceFrame.vue'

const props = defineProps<{
  contentClass?: string | undefined
  viewportHeight?: boolean
}>()
const tableFrameClass = computed(() => [
  'min-w-0 overflow-hidden rounded-lg border border-border bg-card',
  props.viewportHeight && 'h-full',
])
</script>
