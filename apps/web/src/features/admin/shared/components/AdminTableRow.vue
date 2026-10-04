<template>
  <tr
    :class="
      cn(
        'border-b border-border transition-colors data-[state=selected]:bg-muted',
        props.hoverable && 'hover:bg-muted/50',
        props.class,
      )
    "
  >
    <slot />
  </tr>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

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
