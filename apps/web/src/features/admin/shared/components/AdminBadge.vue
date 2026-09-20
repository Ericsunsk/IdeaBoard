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
  default: 'bg-admin-accent text-white border-transparent',
  success: 'bg-admin-success-soft text-admin-success-strong border-admin-success/20 font-bold',
  destructive: 'bg-admin-danger-soft text-admin-danger border-admin-danger/20 font-bold',
  danger: 'bg-admin-danger-soft text-admin-danger border-admin-danger/20 font-bold',
  warning: 'bg-admin-warning-soft text-admin-warning border-admin-warning/20 font-bold',
  warn: 'bg-admin-warning-soft text-admin-warning border-admin-warning/20 font-bold',
  info: 'bg-admin-info-soft text-admin-info border-admin-info/20 font-bold',
  secondary:
    'bg-admin-surface-muted text-admin-text-2-strong border-admin-border-soft font-semibold',
  outline: 'border-admin-border text-admin-text-1 bg-white font-medium',
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
