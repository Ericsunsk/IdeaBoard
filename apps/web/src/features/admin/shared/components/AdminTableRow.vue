<template>
  <tr
    :class="
      cn(
        'border-b border-admin-divider transition-colors data-[state=selected]:bg-admin-surface-muted',
        props.hoverable && 'hover:bg-admin-surface-hover',
        props.class,
      )
    "
  >
    <slot />
  </tr>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '../lib/classNames'

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
