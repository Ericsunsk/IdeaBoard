<template>
  <TableBody :class="props.class">
    <slot />
  </TableBody>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import TableBody from '@/components/ui/table/TableBody.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
