<template>
  <td
    :class="
      cn(
        'border-b border-admin-divider px-3 py-2.5 align-middle text-admin-body text-admin-text-1 [&:has([role=checkbox])]:pr-0',
        props.class,
      )
    "
  >
    <slot />
  </td>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
