<template>
  <TableFooter :class="cn('border-border bg-background', props.class)">
    <slot />
  </TableFooter>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableFooter from '@/components/ui/table/TableFooter.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
