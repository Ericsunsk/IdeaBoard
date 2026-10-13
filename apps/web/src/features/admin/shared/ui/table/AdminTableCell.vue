<template>
  <TableCell
    :class="cn('border-b border-border px-3 py-2.5 text-admin-body text-foreground', props.class)"
  >
    <slot />
  </TableCell>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableCell from '@/components/ui/table/TableCell.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
