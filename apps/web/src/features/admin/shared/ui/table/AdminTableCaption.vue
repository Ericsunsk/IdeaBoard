<template>
  <TableCaption :class="cn('text-xs', props.class)">
    <slot />
  </TableCaption>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableCaption from '@/components/ui/table/TableCaption.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
