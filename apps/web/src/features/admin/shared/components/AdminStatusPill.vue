<template>
  <StatusBadge :tone="props.variant" :label="props.label" :pulse="props.pulse" :size="props.size" />
</template>

<script setup lang="ts">
import StatusBadge from '@/shared/components/StatusBadge.vue'

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
</script>
