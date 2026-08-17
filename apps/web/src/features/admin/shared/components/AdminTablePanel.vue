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
import AdminSurfaceFrame from './AdminSurfaceFrame.vue'

const props = defineProps<{
  contentClass?: string
  viewportHeight?: boolean
}>()
const tableFrameClass = computed(() => [
  'min-w-0 overflow-hidden rounded-lg border border-admin-border-soft bg-white',
  props.viewportHeight && 'h-full',
])
</script>
