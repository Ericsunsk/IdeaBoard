<template>
  <Badge :variant="badgeVariant" rounded :class="pillClass">
    <span v-if="props.dot" class="relative flex size-1.5" aria-hidden="true">
      <span
        v-if="shouldPulse"
        class="absolute inline-flex size-full animate-ping rounded-pill opacity-75"
        :class="dotColorClass"
      />
      <span class="relative inline-flex size-1.5 rounded-pill" :class="dotColorClass" />
    </span>
    <span
      ><slot>{{ props.label }}</slot></span
    >
  </Badge>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { Badge, type BadgeVariants } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

// Project-level status base over shadcn Badge. Five visual semantics:
// neutral / info (covers processing + information states) / success / warning / danger.
// Processing is expressed as info (or success/warning) + pulse, not a sixth tone,
// so AdminStatusPill can wrap this 1:1 on `variant`.
export type AppStatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'
export type AppStatusBadgeSize = 'small' | 'medium'

const props = withDefaults(
  defineProps<{
    tone?: AppStatusTone
    label?: string | undefined
    pulse?: boolean | undefined
    dot?: boolean | undefined
    size?: AppStatusBadgeSize
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    tone: 'neutral',
    label: undefined,
    pulse: undefined,
    dot: true,
    size: 'medium',
    class: undefined,
  },
)

// Pulse default mirrors the operational convention: only success/warning
// breathe by default; explicit `pulse` always wins.
const shouldPulse = computed(() => {
  if (props.pulse !== undefined) return props.pulse
  return props.tone === 'success' || props.tone === 'warning'
})

// Neutral lands on the Badge secondary carrier; the tone class then wins via cn().
const badgeVariant = computed<BadgeVariants['variant']>(() => {
  switch (props.tone) {
    case 'success':
      return 'success'
    case 'warning':
      return 'warning'
    case 'danger':
      return 'danger'
    case 'info':
      return 'info'
    case 'neutral':
    default:
      return 'secondary'
  }
})

// Tone colors are canonical semantic tokens (success/warning/info/destructive/muted),
// never product-prefixed aliases, so both surfaces share them.
const toneClass = computed(() => {
  switch (props.tone) {
    case 'success':
      return 'bg-success-soft text-success border-success/20 shadow-xs'
    case 'danger':
      return 'bg-destructive-soft text-destructive border-destructive/20 shadow-xs'
    case 'warning':
      return 'bg-warning-soft text-warning border-warning/20 shadow-xs'
    case 'info':
      return 'bg-info-soft text-info border-info/20 shadow-xs'
    case 'neutral':
    default:
      // hover:bg-muted neutralizes the Badge secondary hover wash.
      return 'bg-muted text-muted-foreground border-border hover:bg-muted'
  }
})

const sizeClass = computed(() => {
  if (props.size === 'small') {
    return 'h-5 px-2 gap-1 text-admin-caption'
  }
  return 'h-6 px-2.5 gap-1.5 text-xs'
})

// py-0 settles the Badge base py-0.5: fixed-height pills need no vertical padding.
const pillClass = computed(() =>
  cn('shrink-0 select-none leading-none py-0', sizeClass.value, toneClass.value, props.class),
)

const dotColorClass = computed(() => {
  switch (props.tone) {
    case 'success':
      return 'bg-success'
    case 'danger':
      return 'bg-destructive'
    case 'warning':
      return 'bg-warning'
    case 'info':
      return 'bg-info'
    case 'neutral':
    default:
      return 'bg-muted-foreground'
  }
})
</script>
