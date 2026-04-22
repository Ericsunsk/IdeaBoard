<template>
  <div class="admin-card-header">
    <div class="admin-card-copy">
      <div class="admin-card-title-row">
        <AdminChip :label="provider" tone="strong" />
        <AdminChip :label="status" :variant="statusVariant" />
      </div>
      <h3 class="admin-card-title">{{ title }}</h3>
      <p class="admin-card-subtitle">{{ subtitle }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import AdminChip from './AdminChip.vue'

withDefaults(
  defineProps<{
    provider: string
    status: string
    statusVariant?: 'neutral' | 'current' | 'enabled' | 'empty'
    title: string
    subtitle: string
  }>(),
  {
    statusVariant: 'neutral',
  }
)
</script>

<style scoped>
.admin-card-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.admin-card-copy {
  min-width: 0;
}

.admin-card-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.admin-card-title {
  margin: 8px 0 0;
  font-size: 20px;
  color: var(--admin-text-1);
}

.admin-card-subtitle {
  margin: 6px 0 0;
  color: var(--admin-text-2);
  font-size: 13px;
}
</style>
