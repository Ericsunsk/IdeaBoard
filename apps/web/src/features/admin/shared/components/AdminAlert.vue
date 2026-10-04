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
    return 'border-destructive/20 bg-destructive/15 text-destructive font-medium'
  }
  if (sev === 'warn' || sev === 'warning' || v === 'warning') {
    return 'border-amber-500/20 bg-amber-500/15 text-admin-warning font-medium'
  }
  if (sev === 'success' || v === 'success') {
    return 'border-emerald-500/20 bg-emerald-500/15 text-admin-success font-medium'
  }
  if (sev === 'secondary') {
    return 'border-border bg-muted text-muted-foreground'
  }
  return 'border-sky-500/20 bg-sky-500/15 text-admin-info'
})
</script>
