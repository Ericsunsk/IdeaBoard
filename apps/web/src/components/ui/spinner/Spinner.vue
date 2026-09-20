<template>
  <svg
    :class="cn('animate-spin text-current', props.class)"
    :style="spinnerStyle"
    viewBox="0 0 24 24"
    fill="none"
    :role="props.label ? 'status' : undefined"
    :aria-label="props.label"
    :aria-hidden="props.label ? undefined : true"
  >
    <circle
      class="opacity-20"
      cx="12"
      cy="12"
      r="10"
      stroke="currentColor"
      :stroke-width="strokeWidth"
    />
    <path
      d="M22 12a10 10 0 0 0-10-10"
      stroke="currentColor"
      :stroke-width="strokeWidth"
      stroke-linecap="round"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed, type CSSProperties, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    strokeWidth?: number | string
    label?: string | undefined
    class?: HTMLAttributes['class']
    size?: number | string | undefined
  }>(),
  {
    strokeWidth: 4,
    label: undefined,
    class: undefined,
    size: undefined,
  },
)

const spinnerStyle = computed<CSSProperties | undefined>(() => {
  if (props.size == null) return undefined
  const size = typeof props.size === 'number' ? `${props.size}px` : props.size
  return { width: size, height: size }
})
</script>
