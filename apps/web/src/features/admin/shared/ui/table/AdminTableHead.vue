<template>
  <TableHead
    :class="cn('px-3 py-2 text-xs font-semibold text-foreground whitespace-nowrap', props.class)"
  >
    <slot />
  </TableHead>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableHead from '@/components/ui/table/TableHead.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
