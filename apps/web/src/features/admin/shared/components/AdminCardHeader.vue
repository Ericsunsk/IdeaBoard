<template>
  <div class="admin-card-header" :class="{ 'is-small': size === 'small' }">
    <div class="admin-card-header-main">
      <slot name="prefix" />
      <div class="admin-card-copy">
        <div v-if="status || provider" class="admin-card-title-row">
          <AdminChip v-if="provider" :label="provider" tone="strong" />
          <AdminChip v-if="status" :label="status" :variant="statusVariant" />
        </div>
        <h3 class="admin-card-title">{{ title }}</h3>
        <p class="admin-card-subtitle" :title="subtitle">{{ subtitle }}</p>
      </div>
    </div>
    <div v-if="$slots.extra" class="admin-card-header-side">
      <slot name="extra" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    provider?: string
    status?: string
    statusVariant?: 'neutral' | 'current' | 'enabled' | 'empty'
    title: string
    subtitle: string
    size?: 'default' | 'small'
  }>(),
  {
    provider: '',
    status: '',
    statusVariant: 'neutral',
    size: 'default',
  },
)
</script>

<style scoped>
.admin-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.admin-card-header-main {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

.admin-card-copy {
  min-width: 0;
  flex: 1;
}

.admin-card-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.admin-card-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--admin-text-1);
  line-height: 1.2;
}

.admin-card-header.is-small .admin-card-title {
  font-size: 15px;
  margin-top: 2px;
}

.admin-card-subtitle {
  margin: 4px 0 0;
  color: var(--admin-text-2);
  font-size: 12px;
  opacity: 0.8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
