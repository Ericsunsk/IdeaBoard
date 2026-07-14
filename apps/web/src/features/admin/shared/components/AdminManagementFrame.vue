<template>
  <AdminSurfaceFrame viewport-height scroll-content :content-class="props.contentClass">
    <template v-if="$slots.toolbar" #toolbar>
      <slot name="toolbar" />
    </template>

    <template v-if="$slots['toolbar-start']" #toolbar-start>
      <slot name="toolbar-start" />
    </template>

    <template v-if="$slots['toolbar-end']" #toolbar-end>
      <slot name="toolbar-end" />
    </template>

    <slot />
  </AdminSurfaceFrame>
</template>

<script setup lang="ts">
import AdminSurfaceFrame from './AdminSurfaceFrame.vue'

const props = defineProps<{
  contentClass?: string
}>()
</script>
