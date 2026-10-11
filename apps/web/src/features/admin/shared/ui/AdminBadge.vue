<template>
  <Badge :variant="resolvedVariant" :rounded="rounded" :class="computedClasses">
    <slot>{{ value }}</slot>
  </Badge>
</template>

<script setup lang="ts">
/**
 * AdminBadge：shadcn Badge 承载的通用徽标（计数/标签等非状态用途）。
 *
 * 状态表达已收口到 AdminStatusPill（同样 shadcn Badge 载体，带语义圆点/脉冲）：
 * 新的状态胶囊请用 AdminStatusPill。
 */
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
    value?: string | number | null | undefined
    rounded?: boolean
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    variant: 'default',
    value: null,
    rounded: true,
    class: undefined,
  },
)

const adminBadgeColorOverrides: Record<string, string> = {
  default: 'bg-primary text-primary-foreground border-transparent',
  success: 'bg-success-soft text-success border-success/20 font-bold',
  destructive: 'bg-destructive-soft text-destructive border-destructive/20 font-bold',
  danger: 'bg-destructive-soft text-destructive border-destructive/20 font-bold',
  warning: 'bg-warning-soft text-warning border-warning/20 font-bold',
  warn: 'bg-warning-soft text-warning border-warning/20 font-bold',
  info: 'bg-info-soft text-info border-info/20 font-bold',
  secondary: 'bg-secondary text-secondary-foreground border-border font-semibold',
  outline: 'border-border text-foreground bg-card font-medium',
}

const keyVariant = computed<AdminBadgeVariant>(() => props.variant ?? 'default')

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
