<template>
  <TableRow :class="cn('border-border', !props.hoverable && 'hover:bg-transparent', props.class)">
    <slot />
  </TableRow>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableRow from '@/components/ui/table/TableRow.vue'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    hoverable?: boolean | undefined
  }>(),
  {
    class: undefined,
    hoverable: true,
  },
)
</script>
