<template>
  <div class="summary-card">
    <span class="summary-label">{{ label }}</span>
    <span class="summary-value">
      <slot>{{ value }}</slot>
    </span>
    <span v-if="foot || $slots.foot" class="summary-foot">
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

<style scoped>
.summary-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 96px;
  padding: 16px 18px;
  border-radius: 14px;
  background: var(--admin-card-strong, var(--admin-card));
  border: 1px solid var(--admin-border-soft, var(--admin-border));
}

.summary-label {
  font-size: var(--admin-font-size-meta);
  color: var(--admin-title-color, var(--admin-text-1));
  font-weight: 600;
  letter-spacing: 0.02em;
}

.summary-value {
  font-size: var(--admin-font-size-summary-value);
  font-weight: 700;
  color: var(--admin-title-color, var(--admin-text-1));
}

.summary-foot {
  font-size: var(--admin-font-size-meta);
  color: var(--admin-subtitle-color, var(--admin-text-3));
}
</style>
