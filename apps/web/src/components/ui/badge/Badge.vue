<template>
  <div :class="cn(badgeVariants({ variant: resolvedVariant, rounded: props.rounded ? 'full' : 'default' }), props.class)">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { type BadgeVariants, badgeVariants } from './badgeVariants'
import { cn } from '@/lib/utils'

export type BadgeSeverity = 'success' | 'warning' | 'danger' | 'info' | 'secondary'

interface Props {
  variant?: BadgeVariants['variant'] | undefined
  severity?: BadgeSeverity | undefined
  rounded?: boolean | undefined
  class?: HTMLAttributes['class'] | undefined
}

const props = withDefaults(defineProps<Props>(), {
  variant: undefined,
  severity: undefined,
  rounded: false,
  class: undefined,
})

const resolvedVariant = computed<BadgeVariants['variant']>(() => {
  if (props.severity === 'danger') return 'danger'
  if (props.severity) return props.severity
  return props.variant ?? 'default'
})
</script>
