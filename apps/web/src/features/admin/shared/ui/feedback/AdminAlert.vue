<template>
  <div
    role="alert"
    :class="
      cn(
        'relative w-full rounded-panel border px-4 py-3 text-admin-body transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] flex items-start gap-3',
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
import { cn } from '@/lib/utils'

export type AdminAlertSeverity =
  'info' | 'success' | 'warn' | 'warning' | 'error' | 'danger' | 'secondary'

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
    return 'border-destructive/20 bg-destructive-soft text-destructive font-medium'
  }
  if (sev === 'warn' || sev === 'warning' || v === 'warning') {
    return 'border-warning/20 bg-warning-soft text-warning font-medium'
  }
  if (sev === 'success' || v === 'success') {
    return 'border-success/20 bg-success-soft text-success font-medium'
  }
  if (sev === 'secondary') {
    return 'border-border bg-muted text-muted-foreground'
  }
  return 'border-info/20 bg-info-soft text-info'
})
</script>
