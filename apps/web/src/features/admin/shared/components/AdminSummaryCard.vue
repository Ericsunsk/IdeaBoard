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
withDefaults(defineProps<{
  label: string
  value?: string | number
  foot?: string
}>(), {
  value: '',
  foot: '',
})
</script>

<style scoped>
.summary-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 18px;
  border-radius: 18px;
  background: var(--admin-panel);
  border: 1px solid var(--admin-border);
}

.summary-label {
  font-size: 12px;
  color: var(--admin-text-3);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.summary-value {
  font-size: clamp(24px, 3vw, 34px);
  font-weight: 700;
  color: var(--admin-text-1);
  line-height: 1.05;
}

.summary-foot {
  font-size: 12px;
  color: var(--admin-text-2);
}
</style>
