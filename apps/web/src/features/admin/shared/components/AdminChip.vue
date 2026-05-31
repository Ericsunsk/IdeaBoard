<template>
  <span class="admin-chip" :class="chipClasses">{{ label }}</span>
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

const chipClasses = computed(() => ({
  'is-strong': props.tone === 'strong',
  'is-current': props.variant === 'current',
  'is-enabled': props.variant === 'enabled',
  'is-empty': props.variant === 'empty',
}))
</script>

<style scoped>
.admin-chip {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  background: var(--admin-surface-muted, #f4f6fb);
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  font-size: 11px;
}

.admin-chip.is-strong {
  color: var(--admin-text-1);
}

.admin-chip.is-current {
  color: var(--admin-success);
  background: rgba(22, 163, 74, 0.1);
  border-color: rgba(22, 163, 74, 0.22);
}

.admin-chip.is-enabled {
  color: var(--admin-text-2);
  background: rgba(122, 107, 213, 0.09);
  border-color: rgba(122, 107, 213, 0.22);
}

.admin-chip.is-empty {
  color: var(--admin-warning);
  background: rgba(217, 119, 6, 0.11);
  border-color: rgba(217, 119, 6, 0.22);
}
</style>
