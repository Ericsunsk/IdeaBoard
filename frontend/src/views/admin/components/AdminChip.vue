<template>
  <span class="admin-chip" :class="chipClasses">{{ label }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

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
  }
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
  background: #17181a;
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  font-size: 11px;
}

.admin-chip.is-strong {
  color: var(--admin-text-1);
}

.admin-chip.is-current {
  color: #dbeafe;
  background: rgba(59, 130, 246, 0.12);
  border-color: rgba(59, 130, 246, 0.24);
}

.admin-chip.is-enabled {
  color: #d1fae5;
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.24);
}

.admin-chip.is-empty {
  color: #fef3c7;
  background: rgba(245, 158, 11, 0.12);
  border-color: rgba(245, 158, 11, 0.24);
}
</style>
