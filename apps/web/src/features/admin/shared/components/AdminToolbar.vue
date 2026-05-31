<template>
  <div class="admin-toolbar">
    <div class="admin-toolbar-main">
      <n-input
        v-if="showSearch"
        :value="search"
        :placeholder="searchPlaceholder"
        clearable
        class="admin-toolbar-search"
        @update:value="handleSearchUpdate"
        @keyup.enter="emit('search-submit')"
      >
        <template #prefix>
          <n-icon><Search /></n-icon>
        </template>
      </n-input>

      <slot name="filters" />

      <n-button
        v-if="showRefresh"
        secondary
        class="admin-icon-btn"
        :aria-label="refreshLabel"
        @click="emit('refresh')"
      >
        <template #icon>
          <n-icon><Refresh /></n-icon>
        </template>
      </n-button>
    </div>

    <div v-if="$slots.actions" class="admin-toolbar-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { NButton, NIcon, NInput } from 'naive-ui'
import { Refresh, Search } from '@iconoir/vue'

withDefaults(
  defineProps<{
    search?: string
    searchPlaceholder?: string
    showSearch?: boolean
    showRefresh?: boolean
    refreshLabel?: string
  }>(),
  {
    search: '',
    searchPlaceholder: '搜索...',
    showSearch: true,
    showRefresh: true,
    refreshLabel: '刷新',
  },
)

const emit = defineEmits<{
  'update:search': [value: string]
  'search-submit': []
  refresh: []
}>()

function handleSearchUpdate(value: string) {
  emit('update:search', value)
}
</script>

<style scoped>
.admin-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--admin-border-soft);
  border-radius: 14px;
  padding: 12px;
  background: var(--admin-card);
}

.admin-toolbar-main,
.admin-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  min-width: 0;
}

.admin-toolbar-main {
  flex: 1 1 320px;
}

.admin-toolbar-actions {
  justify-content: flex-end;
}

.admin-toolbar-search {
  width: 260px;
  max-width: 100%;
}

.admin-toolbar-search :deep(.n-input) {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.9);
}

@media (max-width: 1024px) {
  .admin-toolbar {
    padding: 10px;
  }

  .admin-toolbar-search {
    width: 100%;
    flex: 1;
  }
}

@media (max-width: 720px) {
  .admin-toolbar {
    gap: 12px;
  }

  .admin-toolbar-main,
  .admin-toolbar-actions {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
