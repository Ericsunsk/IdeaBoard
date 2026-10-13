<template>
  <AvatarFallback
    :delay-ms="props.delayMs"
    :class="
      cn(
        'flex size-full items-center justify-center rounded-pill bg-muted font-medium',
        props.class,
      )
    "
  >
    <slot />
  </AvatarFallback>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AvatarFallback } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    delayMs?: number
    class?: HTMLAttributes['class']
  }>(),
  {
    delayMs: 0,
    class: undefined,
  },
)
</script>
