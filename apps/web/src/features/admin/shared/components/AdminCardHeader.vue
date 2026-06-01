<template>
  <div
    class="admin-card-heading"
    :class="{
      'is-small': size === 'small',
      'is-table': variant === 'table',
    }"
  >
    <div class="admin-card-heading-main">
      <slot name="prefix" />
      <div class="admin-card-heading-copy">
        <div v-if="status || provider" class="admin-card-heading-title-row">
          <AdminChip v-if="provider" :label="provider" tone="strong" />
          <AdminChip v-if="status" :label="status" :variant="statusVariant" />
        </div>
        <h3 class="admin-card-heading-title">{{ title }}</h3>
        <p v-if="trimmedSubtitle" class="admin-card-heading-subtitle" :title="trimmedSubtitle">
          {{ trimmedSubtitle }}
        </p>
      </div>
    </div>
    <div v-if="$slots.extra" class="admin-card-heading-side">
      <slot name="extra" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    provider?: string
    status?: string
    statusVariant?: 'neutral' | 'current' | 'enabled' | 'empty'
    title: string
    subtitle?: string
    size?: 'default' | 'small'
    variant?: 'card' | 'table'
  }>(),
  {
    provider: '',
    status: '',
    statusVariant: 'neutral',
    subtitle: '',
    size: 'default',
    variant: 'card',
  },
)

const trimmedSubtitle = computed(() =>
  typeof props.subtitle === 'string' ? props.subtitle.trim() : '',
)
</script>

<style scoped>
.admin-card-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.admin-card-heading.is-table {
  padding: 14px 24px;
  border-bottom: 1px solid var(--admin-border-soft);
}

.admin-card-heading-main {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

.admin-card-heading-copy {
  min-width: 0;
  flex: 1;
}

.admin-card-heading-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.admin-card-heading-title {
  margin: 0;
  font-size: var(--admin-font-size-heading);
  font-weight: 700;
  color: var(--admin-title-color, var(--admin-text-1));
  line-height: 1.2;
}

.admin-card-heading.is-small .admin-card-heading-title {
  font-size: var(--admin-font-size-title-sm);
  margin-top: 2px;
}

.admin-card-heading-subtitle {
  margin: 4px 0 0;
  color: var(--admin-subtitle-color, var(--admin-text-3));
  font-size: var(--admin-font-size-meta);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@media (max-width: 720px) {
  .admin-card-heading.is-table {
    padding: 12px 14px;
  }
}
</style>
