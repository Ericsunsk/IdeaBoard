<template>
  <span
    class="inline-flex min-h-7 items-center rounded-[999px] border px-[10px] text-[11px]"
    :class="chipClasses"
  >
    {{ label }}
  </span>
</template>

<script setup lang="ts">
type ChipVariant = 'neutral' | 'current' | 'enabled' | 'empty'
type ChipTone = 'default' | 'strong'

const props = withDefaults(
  defineProps<{
    label: string
    variant?: ChipVariant
    tone?: ChipTone
  }>(),
  {
    variant: 'neutral',
    tone: 'default',
  },
)

const chipClasses = computed(() => {
  const neutralText =
    props.tone === 'strong' ? 'text-[var(--admin-text-1)]' : 'text-[var(--admin-text-2)]'
  const variantClasses: Record<ChipVariant, string> = {
    neutral: `bg-[var(--admin-surface-muted,#151618)] border-[var(--admin-border)] ${neutralText}`,
    current:
      'text-[var(--admin-success)] bg-[rgba(74,222,128,0.12)] border-[rgba(74,222,128,0.24)]',
    enabled:
      'text-[var(--admin-text-2)] bg-[rgba(255,255,255,0.06)] border-[rgba(255,255,255,0.14)]',
    empty: 'text-[var(--admin-warning)] bg-[rgba(251,191,36,0.12)] border-[rgba(251,191,36,0.24)]',
  }

  return variantClasses[props.variant]
})
</script>
