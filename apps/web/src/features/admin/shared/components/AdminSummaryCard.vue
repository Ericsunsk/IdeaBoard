<template>
  <div
    class="flex min-h-24 flex-col justify-between rounded-[14px] border border-[var(--admin-border-soft,var(--admin-border))] bg-[var(--admin-card-strong,var(--admin-card))] px-[18px] py-4"
  >
    <span class="text-xs tracking-[0.02em] text-[var(--admin-text-3)]">{{ label }}</span>
    <span class="text-[28px] font-bold tracking-[0.01em] text-[var(--admin-text-1)]">
      <slot>{{ value }}</slot>
    </span>
    <span v-if="foot || $slots.foot" class="text-xs text-[var(--admin-text-2)]">
      <slot name="foot">{{ foot }}</slot>
    </span>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    value?: string | number
    foot?: string
  }>(),
  {
    value: '',
    foot: '',
  },
)
</script>
