<template>
  <span
    class="inline-flex items-center select-none font-semibold leading-none border shrink-0 rounded-full transition-colors"
    :class="[pillClasses, sizeClasses]"
  >
    <span class="relative flex size-1.5">
      <span
        v-if="shouldPulse"
        class="absolute inline-flex size-full animate-ping rounded-full opacity-75"
        :class="dotColorClass"
      />
      <span class="relative inline-flex size-1.5 rounded-full" :class="dotColorClass" />
    </span>
    <span>{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

export type AdminStatusPillVariant = 'success' | 'danger' | 'warning' | 'neutral' | 'info'

const props = withDefaults(
  defineProps<{
    variant?: AdminStatusPillVariant
    label: string
    pulse?: boolean | undefined
    size?: 'small' | 'medium'
  }>(),
  {
    variant: 'neutral',
    pulse: undefined,
    size: 'medium',
  },
)

const shouldPulse = computed(() => {
  if (props.pulse !== undefined) return props.pulse
  return props.variant === 'success' || props.variant === 'warning'
})

const sizeClasses = computed(() => {
  if (props.size === 'small') {
    return 'h-5 px-2 gap-1 text-[11px]'
  }
  return 'h-6 px-2.5 gap-1.5 text-xs'
})

const pillClasses = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'bg-admin-success-soft text-admin-success border-admin-success/30 shadow-xs'
    case 'danger':
      return 'bg-admin-danger-soft text-admin-danger border-admin-danger/30 shadow-xs'
    case 'warning':
      return 'bg-admin-warning-soft text-admin-warning border-admin-warning/30 shadow-xs'
    case 'info':
      return 'bg-admin-info-soft text-admin-info border-admin-info/30 shadow-xs'
    case 'neutral':
    default:
      return 'bg-admin-surface-muted text-admin-text-3 border-admin-border-soft'
  }
})

const dotColorClass = computed(() => {
  switch (props.variant) {
    case 'success':
      return 'bg-admin-success'
    case 'danger':
      return 'bg-admin-danger'
    case 'warning':
      return 'bg-admin-warning'
    case 'info':
      return 'bg-admin-info'
    case 'neutral':
    default:
      return 'bg-admin-text-3'
  }
})
</script>
