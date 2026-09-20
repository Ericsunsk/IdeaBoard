<template>
  <div
    role="alert"
    :class="
      cn(
        'relative w-full rounded-lg border px-4 py-3 text-admin-body transition-colors flex items-start gap-3',
        variantClasses,
        props.class,
      )
    "
  >
    <div class="flex-1 min-w-0">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { cn } from '../lib/classNames'

export type AdminAlertSeverity =
  | 'info'
  | 'success'
  | 'warn'
  | 'warning'
  | 'error'
  | 'danger'
  | 'secondary'

const props = withDefaults(
  defineProps<{
    severity?: AdminAlertSeverity | undefined
    variant?: 'default' | 'destructive' | 'success' | 'warning' | 'info' | undefined
    class?: HTMLAttributes['class']
  }>(),
  {
    severity: 'info',
    variant: 'default',
    class: undefined,
  },
)

const variantClasses = computed(() => {
  const sev = props.severity
  const v = props.variant

  if (sev === 'error' || sev === 'danger' || v === 'destructive') {
    return 'border-admin-danger/30 bg-admin-danger-soft text-admin-danger font-medium'
  }
  if (sev === 'warn' || sev === 'warning' || v === 'warning') {
    return 'border-admin-warning/30 bg-admin-warning-soft text-admin-warning font-medium'
  }
  if (sev === 'success' || v === 'success') {
    return 'border-admin-success/30 bg-admin-success-soft text-admin-success font-medium'
  }
  if (sev === 'secondary') {
    return 'border-admin-border bg-admin-surface-muted text-admin-text-2'
  }
  return 'border-admin-info/30 bg-admin-info-soft text-admin-info'
})
</script>
