<template>
  <TableHeader :class="cn('[&_tr]:border-border', props.class)">
    <slot />
  </TableHeader>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import TableHeader from '@/components/ui/table/TableHeader.vue'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
