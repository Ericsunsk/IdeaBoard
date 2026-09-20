<template>
  <div :class="computedClasses">
    <slot>{{ value }}</slot>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { cn } from '../lib/classNames'

export type AdminBadgeVariant =
  | 'default'
  | 'secondary'
  | 'destructive'
  | 'outline'
  | 'success'
  | 'warning'
  | 'info'
  | 'danger'

const props = withDefaults(
  defineProps<{
    variant?: AdminBadgeVariant | undefined
    severity?: 'success' | 'danger' | 'warning' | 'info' | 'secondary' | 'warn' | 'contrast' | undefined
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

const computedClasses = computed(() => {
  const sev = normalizeSeverity(props.severity)

  let style = 'bg-admin-accent text-white border-transparent'
  if (sev === 'success' || props.variant === 'success') {
    style = 'bg-admin-success-soft text-admin-success border-admin-success/20 font-bold'
  } else if (sev === 'danger' || props.variant === 'destructive' || props.variant === 'danger') {
    style = 'bg-admin-danger-soft text-admin-danger border-admin-danger/20 font-bold'
  } else if (sev === 'warning' || sev === 'warn' || props.variant === 'warning') {
    style = 'bg-admin-warning-soft text-admin-warning border-admin-warning/20 font-bold'
  } else if (sev === 'info' || props.variant === 'info') {
    style = 'bg-admin-info-soft text-admin-info border-admin-info/20 font-bold'
  } else if (sev === 'secondary' || props.variant === 'secondary') {
    style = 'bg-admin-surface-muted text-admin-text-2 border-admin-border-soft font-semibold'
  } else if (props.variant === 'outline') {
    style = 'border-admin-border text-admin-text-1 bg-white font-medium'
  }

  const radius = props.rounded ? 'rounded-full' : 'rounded-md'

  return cn(
    'inline-flex items-center justify-center border px-2.5 py-0.5 text-admin-caption font-semibold transition-colors select-none whitespace-nowrap',
    style,
    radius,
    props.class,
  )
})

// contrast（旧 PrimeVue Tag 中性深色）归一为中性，避免回退到强调蓝。
function normalizeSeverity(
  severity: 'success' | 'danger' | 'warning' | 'info' | 'secondary' | 'warn' | 'contrast' | undefined,
): 'success' | 'danger' | 'warning' | 'info' | 'secondary' | 'warn' | undefined {
  if (severity === 'contrast') return 'secondary'
  return severity
}
</script>
