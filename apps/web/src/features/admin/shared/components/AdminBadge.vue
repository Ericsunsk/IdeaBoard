<template>
  <Badge :variant="resolvedVariant" :rounded="rounded" :class="computedClasses">
    <slot>{{ value }}</slot>
  </Badge>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { Badge, type BadgeVariants } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

export type AdminBadgeVariant =
  | 'default'
  | 'success'
  | 'destructive'
  | 'danger'
  | 'warning'
  | 'warn'
  | 'info'
  | 'secondary'
  | 'outline'

const props = withDefaults(
  defineProps<{
    variant?: AdminBadgeVariant | undefined
    severity?:
      'success' | 'danger' | 'warning' | 'info' | 'secondary' | 'warn' | 'contrast' | undefined
    value?: string | number | null | undefined
    rounded?: boolean
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    variant: 'default',
    severity: undefined,
    value: null,
    rounded: true,
    class: undefined,
  },
)

const adminBadgeColorOverrides: Record<string, string> = {
  default: 'bg-primary text-primary-foreground border-transparent',
  success: 'bg-emerald-500/15 text-admin-success-strong border-emerald-500/20 font-bold',
  destructive: 'bg-destructive/15 text-destructive border-destructive/20 font-bold',
  danger: 'bg-destructive/15 text-destructive border-destructive/20 font-bold',
  warning: 'bg-amber-500/15 text-admin-warning border-amber-500/20 font-bold',
  warn: 'bg-amber-500/15 text-admin-warning border-amber-500/20 font-bold',
  info: 'bg-sky-500/15 text-admin-info border-sky-500/20 font-bold',
  secondary:
    'bg-secondary text-secondary-foreground border-border font-semibold',
  outline: 'border-border text-foreground bg-card font-medium',
}

const keyVariant = computed(() => {
  return (
    (props.severity === 'contrast' ? 'secondary' : (props.severity as AdminBadgeVariant)) ??
    props.variant ??
    'default'
  )
})

const resolvedVariant = computed<BadgeVariants['variant']>(() => {
  const k = keyVariant.value
  if (k === 'warn' || k === 'warning') return 'warning'
  if (k === 'danger' || k === 'destructive') return 'danger'
  if (k === 'info') return 'info'
  if (k === 'success') return 'success'
  if (k === 'secondary') return 'secondary'
  if (k === 'outline') return 'outline'
  return 'default'
})

const computedClasses = computed(() => {
  return cn(
    'text-admin-caption select-none whitespace-nowrap',
    adminBadgeColorOverrides[keyVariant.value],
    props.class,
  )
})
</script>
