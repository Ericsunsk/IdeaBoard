<template>
  <div
    role="progressbar"
    :aria-label="ariaLabel"
    :class="
      cn(
        'inline-block size-6 animate-spin rounded-full border-2 border-current border-t-transparent text-admin-accent',
        props.class,
      )
    "
  >
    <span class="sr-only">{{ ariaLabel }}</span>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    ariaLabel?: string | undefined
  }>(),
  {
    class: undefined,
    ariaLabel: '加载中',
  },
)
</script>
