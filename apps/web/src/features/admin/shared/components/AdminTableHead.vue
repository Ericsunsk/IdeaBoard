<template>
  <th
    :class="
      cn(
        'h-10 border-b border-admin-border px-3 py-2 text-left align-middle text-admin-caption font-bold text-admin-text-3 whitespace-nowrap [&:has([role=checkbox])]:pr-0',
        props.class,
      )
    "
  >
    <slot />
  </th>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
