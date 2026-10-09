<template>
  <div
    :class="
      cn(
        badgeVariants({
          variant: props.variant ?? 'default',
          rounded: props.rounded ? 'full' : 'default',
        }),
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { type BadgeVariants, badgeVariants } from './badgeVariants'
import { cn } from '@/lib/utils'

interface Props {
  variant?: BadgeVariants['variant'] | undefined
  rounded?: boolean | undefined
  class?: HTMLAttributes['class'] | undefined
}

const props = withDefaults(defineProps<Props>(), {
  variant: undefined,
  rounded: false,
  class: undefined,
})
</script>
