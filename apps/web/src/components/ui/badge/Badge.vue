<template>
  <div :class="cn(badgeVariants({ variant }), props.class)">
    <slot />
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { type BadgeVariants, badgeVariants } from './badgeVariants'
import { cn } from '@/lib/utils'

interface Props {
  variant?: BadgeVariants['variant'] | undefined
  class?: HTMLAttributes['class'] | undefined
}

const props = withDefaults(defineProps<Props>(), {
  variant: undefined,
  class: undefined,
})
</script>
